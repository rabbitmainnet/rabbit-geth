package lqc

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/misc"
	"github.com/ethereum/go-ethereum/consensus/misc/eip1559"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/types/bal"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/trie"
	"github.com/holiman/uint256"
)

var errUnknownAncestor = errors.New("unknown ancestor")
var errInvalidBlockNumber = errors.New("invalid block number")
var errUnclesNotAllowed = errors.New("uncles are not allowed in lqc")
var errInvalidExtra = errors.New("invalid lqc extra data")
var errBlockTimeOverflow = errors.New("lqc block time overflow")

// allowedFutureBlockTimeSeconds is deliberately small: LQC producers already
// have deterministic time slots, so accepting a header far in the future can
// stall the canonical chain until wall-clock time catches up.
const allowedFutureBlockTimeSeconds = uint64(30)

type LQC struct {
	config        *params.LQCConfig
	chainID       *big.Int
	db            ethdb.Database
	registryCache *registrySnapshotCache
	registryPool  *RegistryOperationPool
}

// verifiedHeaderChain exposes headers which have already passed verification in
// the current batch. Header-chain sync validates a batch before inserting it,
// so stateful LQC checks must be able to walk those verified ancestors without
// treating unverified headers as canonical.
type verifiedHeaderChain struct {
	consensus.ChainHeaderReader
	byHash   map[common.Hash]*types.Header
	byNumber map[uint64]*types.Header
}

func newVerifiedHeaderChain(chain consensus.ChainHeaderReader, capacity int) *verifiedHeaderChain {
	return &verifiedHeaderChain{
		ChainHeaderReader: chain,
		byHash:            make(map[common.Hash]*types.Header, capacity),
		byNumber:          make(map[uint64]*types.Header, capacity),
	}
}

func (c *verifiedHeaderChain) remember(header *types.Header) {
	if header == nil || header.Number == nil || !header.Number.IsUint64() {
		return
	}
	c.byHash[header.Hash()] = header
	c.byNumber[header.Number.Uint64()] = header
}

func (c *verifiedHeaderChain) GetHeader(hash common.Hash, number uint64) *types.Header {
	if header := c.byHash[hash]; header != nil && header.Number.Uint64() == number {
		return header
	}
	return c.ChainHeaderReader.GetHeader(hash, number)
}

func (c *verifiedHeaderChain) GetHeaderByHash(hash common.Hash) *types.Header {
	if header := c.byHash[hash]; header != nil {
		return header
	}
	return c.ChainHeaderReader.GetHeaderByHash(hash)
}

func (c *verifiedHeaderChain) GetHeaderByNumber(number uint64) *types.Header {
	if header := c.byNumber[number]; header != nil {
		return header
	}
	return c.ChainHeaderReader.GetHeaderByNumber(number)
}

// SetChainID binds the generic Engine.SealHash method to the configured chain.
// Header verification always uses the ChainHeaderReader configuration instead.
func (l *LQC) SetChainID(chainID *big.Int) {
	if l == nil || chainID == nil {
		return
	}
	l.chainID = new(big.Int).Set(chainID)
}

func New(config *params.LQCConfig, db ethdb.Database) *LQC {
	return &LQC{
		config:        config,
		db:            db,
		registryCache: newRegistrySnapshotCache(),
		registryPool:  NewRegistryOperationPool(),
	}
}

func (l *LQC) Author(header *types.Header) (common.Address, error) {
	return header.Coinbase, nil
}

// ResolveLocalParticipant implements consensus.LocalParticipantResolver. The
// miner passes the current head, so resolve the deterministic queue for the
// next block using the current head hash as the parent seed.
func (l *LQC) ResolveLocalParticipant(
	chain consensus.ChainHeaderReader,
	header *types.Header,
	accounts []common.Address,
) consensus.LocalParticipant {

	if header == nil || header.Number == nil || len(accounts) == 0 {
		return consensus.LocalParticipant{QueuePos: -1}
	}
	if header.Number.Uint64() == ^uint64(0) {
		return consensus.LocalParticipant{QueuePos: -1}
	}

	blockNumber := header.Number.Uint64() + 1

	// Block 1 is the permissionless activation block.
	// Do NOT require the local account to already exist in the
	// deterministic LCQ queue. Any local wallet may activate the chain.
	if blockNumber == 1 {
		for _, addr := range accounts {
			if addr == (common.Address{}) {
				continue
			}
			return consensus.LocalParticipant{
				Address:  addr,
				QueuePos: 0,
				Allowed:  true,
			}
		}
		return consensus.LocalParticipant{QueuePos: -1}
	}
	now := time.Now().Unix()
	if now >= 0 && l.recoveryOpenAt(header, uint64(now)) {
		for _, addr := range accounts {
			if addr != (common.Address{}) {
				return consensus.LocalParticipant{Address: addr, QueuePos: 0, Allowed: true}
			}
		}
		return consensus.LocalParticipant{QueuePos: -1}
	}

	next := &types.Header{
		ParentHash: header.Hash(),
		Number:     new(big.Int).SetUint64(blockNumber),
	}

	if !l.registryProtocolEnabled(next.Number) {
		for _, addr := range accounts {
			if addr != (common.Address{}) {
				RegisterParticipant(nil, addr, blockNumber)
				UpdateParticipantActivity(nil, addr, blockNumber)
			}
		}
	}

	sel := l.selectionForHeaderMaybeWorkV1Lab(chain, next)
	for queuePos, participant := range sel.Ordered {
		for _, local := range accounts {
			if participant.Address == local {
				return consensus.LocalParticipant{
					Address:  local,
					QueuePos: queuePos,
					Allowed:  true,
				}
			}
		}
	}

	return consensus.LocalParticipant{QueuePos: -1}
}

func (l *LQC) recoveryTimeoutSeconds() uint64 {
	if l == nil || l.config == nil || l.config.RecoveryTimeoutMs == 0 {
		return 0
	}
	seconds := l.config.RecoveryTimeoutMs / 1000
	if l.config.RecoveryTimeoutMs%1000 != 0 {
		seconds++
	}
	return seconds
}

// recoveryOpenAt reopens producer activation after a prolonged chain halt.
// It changes only the canonical producer registry; execution state and block
// history continue from the existing parent.
func (l *LQC) recoveryOpenAt(parent *types.Header, candidateTime uint64) bool {
	timeout := l.recoveryTimeoutSeconds()
	if timeout == 0 || parent == nil || parent.Number == nil || parent.Number.Sign() == 0 {
		return false
	}
	recoveryTime, ok := checkedRegistryBlockAdd(parent.Time, timeout)
	return ok && candidateTime >= recoveryTime
}

func (l *LQC) openActivationForHeader(chain consensus.ChainHeaderReader, header *types.Header) bool {
	if header == nil || header.Number == nil || header.Number.Sign() <= 0 {
		return false
	}
	if header.Number.Uint64() == 1 {
		return l == nil || l.config == nil || len(l.config.BootstrapParticipants) == 0
	}
	if chain == nil {
		return false
	}
	parent := chain.GetHeader(header.ParentHash, header.Number.Uint64()-1)
	return l.recoveryOpenAt(parent, header.Time)
}

func (l *LQC) makeExtra(number uint64) []byte {
	return appendEmptyProducerSeal([]byte(fmt.Sprintf("LQC:1:%d", number)))
}

func (l *LQC) verifyExtra(header *types.Header) error {
	if header == nil || header.Number == nil {
		return errInvalidExtra
	}
	payload, _, err := splitProducerSeal(header.Extra)
	if err != nil {
		return fmt.Errorf("%w: %v", errInvalidExtra, err)
	}
	expected, _, err := splitProducerSeal(l.makeExtra(header.Number.Uint64()))
	if err != nil {
		return fmt.Errorf("%w: %v", errInvalidExtra, err)
	}
	if !bytes.Equal(payload, expected) {
		return fmt.Errorf("%w: expected %q got %q", errInvalidExtra, string(expected), string(payload))
	}
	return nil
}

func (l *LQC) VerifyHeader(chain consensus.ChainHeaderReader, header *types.Header) error {
	return l.verifyHeader(chain, header, nil)
}

func (l *LQC) verifyHeader(chain consensus.ChainHeaderReader, header, batchParent *types.Header) error {
	now := time.Now().Unix()
	if now < 0 {
		now = 0
	}
	return l.verifyHeaderAt(chain, header, batchParent, uint64(now))
}

// verifyHeaderAt contains the deterministic header checks. unixNow is passed
// explicitly so boundary tests do not depend on the machine clock.
func (l *LQC) verifyHeaderAt(chain consensus.ChainHeaderReader, header, batchParent *types.Header, unixNow uint64) error {
	if header == nil {
		return errors.New("nil header")
	}
	if header.Number == nil {
		return errors.New("missing block number")
	}
	if !header.Number.IsUint64() {
		return errInvalidBlockNumber
	}
	if header.Number.Sign() == 0 {
		// The genesis is committed by its configured hash and is the validation
		// dead-end. Its frozen extraData is not an ordinary LQC block envelope.
		return nil
	}
	parent := batchParent
	if parent == nil && chain != nil {
		parent = chain.GetHeader(header.ParentHash, header.Number.Uint64()-1)
	}
	if parent == nil {
		return errUnknownAncestor
	}
	if parent.Number == nil || !parent.Number.IsUint64() {
		return errInvalidBlockNumber
	}
	if header.Number.Uint64() != parent.Number.Uint64()+1 {
		return errInvalidBlockNumber
	}
	latestTime, ok := checkedRegistryBlockAdd(unixNow, allowedFutureBlockTimeSeconds)
	if !ok {
		latestTime = ^uint64(0)
	}
	if header.Time > latestTime {
		return consensus.ErrFutureBlock
	}
	if header.Time <= parent.Time {
		return errors.New("non-increasing block time")
	}
	if header.Difficulty == nil || header.Difficulty.Sign() != 0 {
		return errors.New("lqc requires zero difficulty")
	}
	if header.Nonce != (types.BlockNonce{}) {
		return errors.New("lqc requires zero nonce")
	}
	if header.MixDigest != (common.Hash{}) {
		return errors.New("lqc requires zero mix digest")
	}
	if err := verifyExecutionHeaderRules(chain, parent, header); err != nil {
		return err
	}
	if err := VerifyProducerSeal(chain.Config().ChainID, header); err != nil {
		return err
	}
	var (
		sel              HybridSelection
		registrySnapshot *RegistrySnapshot
	)

	// Block 1 is the open activation block.
	// If canonical registry is already active, validate the canonical
	// registry envelope. Otherwise use the legacy LQC envelope.
	if header.Number.Uint64() == 1 {
		if header.Coinbase == (common.Address{}) {
			return errors.New("lqc block 1 requires producer coinbase")
		}

		if l.registryProtocolEnabled(header.Number) {
			var err error
			sel, registrySnapshot, err = l.verifyCanonicalRegistryHeaderMaybeWorkV1Lab(chain, header)
			if err != nil {
				return err
			}

			// Block 1 is permissionless: the actual signer becomes
			// the activation producer regardless of the genesis registry.
			sel.Ordered = []HybridParticipant{{
				Address:       header.Coinbase,
				Payout:        header.Coinbase,
				Bond:          big.NewInt(25),
				RegisteredAt:  1,
				LastHeartbeat: 1,
				Status:        ParticipantActiveCandidate,
			}}
			sel.Producer = &sel.Ordered[0]
		} else {
			if err := l.verifyExtra(header); err != nil {
				return err
			}

			RegisterParticipant(nil, header.Coinbase, 1)
			UpdateParticipantActivity(nil, header.Coinbase, 1)

			sel.Ordered = []HybridParticipant{{
				Address:       header.Coinbase,
				Payout:        header.Coinbase,
				Bond:          big.NewInt(25),
				RegisteredAt:  1,
				LastHeartbeat: 1,
				Status:        ParticipantActiveCandidate,
			}}
			sel.Producer = &sel.Ordered[0]
		}
	} else if l.registryProtocolEnabled(header.Number) {
		var err error
		sel, registrySnapshot, err = l.verifyCanonicalRegistryHeaderMaybeWorkV1Lab(chain, header)
		if err != nil {
			return err
		}
	} else {
		if err := l.verifyExtra(header); err != nil {
			return err
		}
		sel = l.selectionForHeader(nil, header)
	}

	if len(sel.Ordered) > 0 {
		ok, queuePos := IsAuthorAllowed(sel, header.Coinbase)
		if !ok {
			return fmt.Errorf("lqc unauthorized producer %s at block %d", header.Coinbase.Hex(), header.Number.Uint64())
		}
		minTime, err := l.minAllowedTimeChecked(parent.Time, queuePos)
		if err != nil {
			return err
		}
		if header.Time < minTime {
			return fmt.Errorf("lqc producer %s published too early at block %d: have %d want >= %d", header.Coinbase.Hex(), header.Number.Uint64(), header.Time, minTime)
		}
	}
	if registrySnapshot != nil {
		l.rememberRegistrySnapshot(registrySnapshot)
	}

	if !l.registryProtocolEnabled(header.Number) && header.Coinbase != (common.Address{}) {
		RegisterParticipant(nil, header.Coinbase, header.Number.Uint64())
		UpdateParticipantActivity(nil, header.Coinbase, header.Number.Uint64())
	}

	return nil
}

// verifyExecutionHeaderRules validates the execution-layer header fields that
// LQC must enforce itself. The Rabbit mainnet is currently London-only; later
// forks must be implemented and audited before their activation is configured.
func verifyExecutionHeaderRules(chain consensus.ChainHeaderReader, parent, header *types.Header) error {
	if chain == nil || chain.Config() == nil {
		return errors.New("missing chain configuration")
	}
	config := chain.Config()

	if header.GasLimit > params.MaxGasLimit {
		return fmt.Errorf("invalid gasLimit: have %d, max %d", header.GasLimit, params.MaxGasLimit)
	}
	if header.GasUsed > header.GasLimit {
		return fmt.Errorf("invalid gasUsed: have %d, gasLimit %d", header.GasUsed, header.GasLimit)
	}
	if !config.IsLondon(header.Number) {
		if header.BaseFee != nil {
			return fmt.Errorf("invalid baseFee before fork: have %d, expected nil", header.BaseFee)
		}
		if err := misc.VerifyGaslimit(parent.GasLimit, header.GasLimit); err != nil {
			return err
		}
	} else {
		if config.IsLondon(parent.Number) && parent.BaseFee == nil {
			return errors.New("london parent is missing baseFee")
		}
		if err := eip1559.VerifyEIP1559Header(config, parent, header); err != nil {
			return err
		}
	}

	if config.IsShanghai(header.Number, header.Time) {
		return errors.New("lqc does not support shanghai fork")
	}
	if header.WithdrawalsHash != nil {
		return fmt.Errorf("invalid withdrawalsHash: have %x, expected nil", header.WithdrawalsHash)
	}
	if config.IsCancun(header.Number, header.Time) {
		return errors.New("lqc does not support cancun fork")
	}
	switch {
	case header.ExcessBlobGas != nil:
		return fmt.Errorf("invalid excessBlobGas: have %d, expected nil", *header.ExcessBlobGas)
	case header.BlobGasUsed != nil:
		return fmt.Errorf("invalid blobGasUsed: have %d, expected nil", *header.BlobGasUsed)
	case header.ParentBeaconRoot != nil:
		return fmt.Errorf("invalid parentBeaconRoot: have %x, expected nil", *header.ParentBeaconRoot)
	}
	if config.IsPrague(header.Number, header.Time) {
		return errors.New("lqc does not support prague fork")
	}
	if header.RequestsHash != nil {
		return fmt.Errorf("invalid requestsHash: have %x, expected nil", *header.RequestsHash)
	}
	return nil
}

func (l *LQC) VerifyHeaders(chain consensus.ChainHeaderReader, headers []*types.Header) (chan<- struct{}, <-chan error) {
	quit := make(chan struct{})
	results := make(chan error, len(headers))
	now := time.Now().Unix()
	if now < 0 {
		now = 0
	}
	unixNow := uint64(now)

	go func() {
		defer close(results)
		batchChain := newVerifiedHeaderChain(chain, len(headers))
		verified := make(map[common.Hash]*types.Header, len(headers))
		for _, header := range headers {
			var parent *types.Header
			if header != nil {
				parent = verified[header.ParentHash]
			}
			err := l.verifyHeaderAt(batchChain, header, parent, unixNow)
			select {
			case <-quit:
				return
			case results <- err:
			}
			if err == nil && header != nil {
				verified[header.Hash()] = header
				batchChain.remember(header)
			}
		}
	}()

	return quit, results
}

func (l *LQC) VerifyUncles(chain consensus.ChainReader, block *types.Block) error {
	if block == nil {
		return nil
	}
	if len(block.Uncles()) > 0 {
		return errUnclesNotAllowed
	}
	return nil
}

func (l *LQC) Prepare(chain consensus.ChainHeaderReader, header *types.Header) error {
	if header == nil {
		return errors.New("nil header")
	}
	if header.Number == nil {
		return errors.New("missing block number")
	}

	header.Difficulty = big.NewInt(0)
	header.Nonce = types.BlockNonce{}
	header.MixDigest = common.Hash{}
	if !l.registryProtocolEnabled(header.Number) {
		header.Extra = l.makeExtra(header.Number.Uint64())
	}

	if header.Number.Sign() > 0 {
		parent := chain.GetHeader(header.ParentHash, header.Number.Uint64()-1)
		if parent == nil {
			return errUnknownAncestor
		}

		queuePos := 0
		var sel HybridSelection

		// Block 1 is the open activation block. Do not ask the canonical
		// registry to select the genesis bootstrap identity. The actual
		// coinbase supplied by the miner is the activation producer.
		if header.Number.Uint64() == 1 {
			if header.Coinbase == (common.Address{}) {
				return errors.New("lqc block 1 requires producer coinbase")
			}

			if l.registryProtocolEnabled(header.Number) {
				var err error
				sel, err = l.prepareCanonicalRegistryExtraMaybeWorkV1Lab(
					chain,
					header,
				)
				if err != nil {
					return err
				}
			}

			sel.Ordered = []HybridParticipant{{
				Address:       header.Coinbase,
				Payout:        header.Coinbase,
				Bond:          big.NewInt(25),
				RegisteredAt:  1,
				LastHeartbeat: 1,
				Status:        ParticipantActiveCandidate,
			}}
			sel.Producer = &sel.Ordered[0]
		} else if l.registryProtocolEnabled(header.Number) {
			var err error
			sel, err = l.prepareCanonicalRegistryExtraMaybeWorkV1Lab(
				chain,
				header,
			)
			if err != nil {
				return err
			}
		} else {
			sel = l.selectionForHeader(nil, header)
		}
		if len(sel.Ordered) > 0 {
			ok, pos := IsAuthorAllowed(sel, header.Coinbase)
			if ok {
				queuePos = pos
			}
		}

		// Preserve the real timestamp proposed by the miner. Only move it
		// forward when it is earlier than the producer/fallback slot. Replacing
		// it unconditionally with parent+slot makes a timestamp-zero genesis
		// generate blocks in the distant past, causing every fallback window to
		// appear expired and multiple nodes to fork at once.
		minTime, err := l.minAllowedTimeChecked(parent.Time, queuePos)
		if err != nil {
			return err
		}
		if header.Time < minTime {
			header.Time = minTime
		}
	}

	if !l.registryProtocolEnabled(header.Number) && header.Coinbase != (common.Address{}) {
		RegisterParticipant(nil, header.Coinbase, header.Number.Uint64())
		UpdateParticipantActivity(nil, header.Coinbase, header.Number.Uint64())
	}

	return nil
}

func (l *LQC) rewardActivityWindow() uint64 {
	if l != nil && l.config != nil && l.config.ActivityWindow > 0 {
		return l.config.ActivityWindow
	}
	return 128
}

func (l *LQC) hybridCfg() HybridLQCConfig {
	cfg := HybridLQCConfig{}

	if l != nil && l.config != nil {
		if l.config.MinBond != nil {
			cfg.MinBond = new(big.Int).Set(l.config.MinBond)
		}
		cfg.ActivationDelay = l.config.ActivationDelay
		cfg.HeartbeatWindow = l.config.HeartbeatWindow
		cfg.HeartbeatGrace = l.config.HeartbeatGrace
		cfg.JailBlocks = l.config.JailBlocks
		cfg.MaxMissedTurns = l.config.MaxMissedTurns
		cfg.MinorSlashBps = l.config.MinorSlashBps
		cfg.MajorSlashBps = l.config.MajorSlashBps

		if l.config.CommitteeSize > 0 {
			cfg.CommitteeSize = l.config.CommitteeSize
		} else if l.config.CommitteeMax > 0 {
			cfg.CommitteeSize = l.config.CommitteeMax
		} else if l.config.CommitteeMin > 0 {
			cfg.CommitteeSize = l.config.CommitteeMin
		}

		if l.config.FallbackCount > 0 {
			cfg.FallbackCount = l.config.FallbackCount
		} else if l.config.FallbackSlots > 0 {
			cfg.FallbackCount = l.config.FallbackSlots
		}

		cfg.BootstrapOnlyUntil = l.config.BootstrapOnlyUntil
	}

	return normalizeHybridConfig(cfg)
}

func (l *LQC) selectionForHeader(
	chain consensus.ChainHeaderReader,
	header *types.Header,
) HybridSelection {
	if header != nil && l.registryProtocolEnabled(header.Number) {
		selection, _, err := l.canonicalSelectionForHeader(chain, header)
		if err != nil {
			log.Warn("Failed to derive canonical LQC selection", "number", header.Number, "parent", header.ParentHash, "err", err)
			return HybridSelection{}
		}
		return selection
	}
	return l.legacySelectionForHeader(header)
}

func (l *LQC) legacySelectionForHeader(header *types.Header) HybridSelection {
	if header == nil || header.Number == nil {
		return HybridSelection{}
	}

	blockNumber := header.Number.Uint64()

	var bootstrap []common.Address
	mode := "open"
	if l != nil && l.config != nil {
		bootstrap = l.config.BootstrapParticipants
		if l.config.RegistryMode != "" {
			mode = l.config.RegistryMode
		}
	}
	bootstrap = NormalizeBootstrapParticipants(bootstrap)

	reg := RealRegistry(blockNumber, bootstrap, mode)
	if reg == nil {
		reg = &Registry{}
	}
	reg.ApplyActivityWindow(blockNumber, l.rewardActivityWindow())

	sel := BuildHybridSelection(
		bootstrap,
		reg.ToHybridParticipants(),
		header.ParentHash,
		blockNumber,
		l.hybridCfg(),
	)

	if len(sel.Ordered) == 0 && header.Coinbase != (common.Address{}) {
		p := HybridParticipant{
			Address:       header.Coinbase,
			Payout:        header.Coinbase,
			Bond:          big.NewInt(25),
			RegisteredAt:  blockNumber,
			LastHeartbeat: blockNumber,
			Status:        ParticipantActiveCandidate,
		}
		sel.Ordered = []HybridParticipant{p}
		sel.Producer = &sel.Ordered[0]
	}

	return sel
}

func hybridToParticipants(in []HybridParticipant) []Participant {
	out := make([]Participant, 0, len(in))
	for _, p := range in {
		out = append(out, Participant{
			Address:         p.Address,
			Active:          true,
			LastActiveBlock: p.LastHeartbeat,
			Weight:          1,
		})
	}
	return out
}

func (l *LQC) targetBlockSeconds() uint64 {
	sec := uint64(15)
	if l != nil && l.config != nil && l.config.TargetBlockTimeMs > 0 {
		sec = l.config.TargetBlockTimeMs / 1000
		if sec == 0 {
			sec = 1
		}
	}
	return sec
}

func (l *LQC) fallbackWindowSeconds() uint64 {
	sec := uint64(5)
	if l != nil && l.config != nil && l.config.FallbackWindowMs > 0 {
		sec = l.config.FallbackWindowMs / 1000
		if sec == 0 {
			sec = 1
		}
	}
	return sec
}

func (l *LQC) minAllowedTime(parentTime uint64, queuePos int) uint64 {
	minTime, err := l.minAllowedTimeChecked(parentTime, queuePos)
	if err != nil {
		return ^uint64(0)
	}
	return minTime
}

func (l *LQC) minAllowedTimeChecked(parentTime uint64, queuePos int) (uint64, error) {
	base := l.targetBlockSeconds()
	if queuePos <= 0 {
		minTime, ok := checkedRegistryBlockAdd(parentTime, base)
		if !ok {
			return 0, errBlockTimeOverflow
		}
		return minTime, nil
	}
	window := l.fallbackWindowSeconds()
	position := uint64(queuePos)
	if window != 0 && position > ^uint64(0)/window {
		return 0, errBlockTimeOverflow
	}
	minTime, ok := checkedRegistryBlockAdd(parentTime, base, position*window)
	if !ok {
		return 0, errBlockTimeOverflow
	}
	return minTime, nil
}

func (l *LQC) rewardCommittee(chain consensus.ChainHeaderReader, header *types.Header) []Participant {
	sel := l.selectionForHeader(chain, header)
	if len(sel.Committee) == 0 {
		return nil
	}
	return hybridToParticipants(sel.Committee)
}

func (l *LQC) rewardCommitteeAddresses(chain consensus.ChainHeaderReader, header *types.Header) []common.Address {
	committee := l.rewardCommittee(chain, header)
	seen := make(map[common.Address]struct{})
	out := make([]common.Address, 0, len(committee))
	for _, p := range committee {
		if p.Address == (common.Address{}) {
			continue
		}
		if p.Address == header.Coinbase {
			continue
		}
		if _, ok := seen[p.Address]; ok {
			continue
		}
		seen[p.Address] = struct{}{}
		out = append(out, p.Address)
	}
	return out
}

func (l *LQC) blockRewardFor(header *types.Header) *uint256.Int {
	if header == nil || header.Number == nil {
		return uint256.NewInt(0)
	}

	eraLength := uint64(8409600)
	if l != nil && l.config != nil && l.config.EraLength > 0 {
		eraLength = l.config.EraLength
	}
	if eraLength == 0 {
		eraLength = 8409600
	}

	era := header.Number.Uint64() / eraLength

	switch era {
	case 0:
		return uint256.NewInt(1200000000000000000) // 1.20 RAB
	case 1:
		return uint256.NewInt(600000000000000000) // 0.60 RAB
	case 2:
		return uint256.NewInt(300000000000000000) // 0.30 RAB
	case 3:
		return uint256.NewInt(150000000000000000) // 0.15 RAB
	default:
		return uint256.NewInt(150000000000000000) // 0.15 RAB
	}
}

func (l *LQC) distributeRewards(chain consensus.ChainHeaderReader, header *types.Header, statedb vm.StateDB) {
	if header == nil || header.Number == nil || statedb == nil {
		return
	}

	totalReward := l.blockRewardFor(header)
	if totalReward == nil || totalReward.IsZero() {
		return
	}

	if l.distributeWorkV1RewardsMaybeLab(
		chain,
		header,
		statedb,
		totalReward,
	) {
		return
	}

	committeeBps := uint64(3000)
	if l != nil && l.config != nil && l.config.CommitteeRatioBps > 0 {
		committeeBps = l.config.CommitteeRatioBps
	}
	if committeeBps > 10000 {
		committeeBps = 10000
	}
	producerBps := uint64(10000 - committeeBps)

	producerReward := new(uint256.Int).Set(totalReward)
	producerReward.Mul(producerReward, uint256.NewInt(producerBps))
	producerReward.Div(producerReward, uint256.NewInt(10000))

	committeeReward := new(uint256.Int).Set(totalReward)
	committeeReward.Sub(committeeReward, producerReward)

	committeeAddrs := l.rewardCommitteeAddresses(chain, header)
	if len(committeeAddrs) == 0 || committeeReward.IsZero() {
		statedb.AddBalance(header.Coinbase, totalReward, tracing.BalanceIncreaseRewardMineBlock)
		return
	}

	statedb.AddBalance(header.Coinbase, producerReward, tracing.BalanceIncreaseRewardMineBlock)

	perMember := new(uint256.Int).Set(committeeReward)
	perMember.Div(perMember, uint256.NewInt(uint64(len(committeeAddrs))))

	allocated := new(uint256.Int).Set(perMember)
	allocated.Mul(allocated, uint256.NewInt(uint64(len(committeeAddrs))))

	remainder := new(uint256.Int).Set(committeeReward)
	remainder.Sub(remainder, allocated)

	for i, addr := range committeeAddrs {
		amount := new(uint256.Int).Set(perMember)
		if i == 0 {
			amount.Add(amount, remainder)
		}
		if !amount.IsZero() {
			statedb.AddBalance(addr, amount, tracing.BalanceIncreaseRewardMineBlock)
		}
	}
}

func (l *LQC) Finalize(chain consensus.ChainHeaderReader, header *types.Header, statedb vm.StateDB, body *types.Body, _ uint32, _ *bal.ConstructionBlockAccessList) {
	if header == nil || header.Number == nil || statedb == nil {
		return
	}
	l.distributeRewards(chain, header, statedb)
}

func (l *LQC) FinalizeAndAssemble(chain consensus.ChainHeaderReader, header *types.Header, statedb *state.StateDB, body *types.Body, receipts []*types.Receipt) (*types.Block, error) {
	if body == nil {
		body = &types.Body{}
	}
	if statedb == nil {
		return nil, errors.New("nil stateDB")
	}
	if header == nil {
		return nil, errors.New("nil header")
	}
	l.Finalize(chain, header, statedb, body, 0, nil)

	isEIP158 := false
	if chain != nil && chain.Config() != nil {
		isEIP158 = chain.Config().IsEIP158(header.Number)
	}
	header.Root = statedb.IntermediateRoot(isEIP158)

	return types.NewBlock(header, body, receipts, trie.NewStackTrie(nil)), nil
}

func (l *LQC) Seal(chain consensus.ChainHeaderReader, block *types.Block, results chan<- *types.Block, stop <-chan struct{}) error {
	if block == nil {
		return errors.New("nil block")
	}
	if chain == nil || chain.Config() == nil {
		return errors.New("missing chain configuration")
	}
	if err := VerifyProducerSeal(chain.Config().ChainID, block.Header()); err != nil {
		return err
	}

	var sel HybridSelection
	if l.registryProtocolEnabled(block.Number()) {
		var err error
		var registrySnapshot *RegistrySnapshot
		sel, registrySnapshot, err = l.verifyCanonicalRegistryHeaderMaybeWorkV1Lab(chain, block.Header())
		if err != nil {
			return err
		}
		l.rememberRegistrySnapshot(registrySnapshot)
	} else {
		sel = l.selectionForHeader(chain, block.Header())
	}
	if len(sel.Ordered) > 0 {
		ok, _ := IsAuthorAllowed(sel, block.Coinbase())
		if !ok {
			return fmt.Errorf("lqc local node is not selected for block %d", block.NumberU64())
		}
	}

	go func() {
		select {
		case <-stop:
			return
		case results <- block:
		}
	}()
	return nil
}

func (l *LQC) SealHash(header *types.Header) common.Hash {
	if header == nil || l == nil || l.chainID == nil {
		return common.Hash{}
	}
	hash, err := producerSealHash(l.chainID, header)
	if err != nil {
		return common.Hash{}
	}
	return hash
}

func (l *LQC) CalcDifficulty(chain consensus.ChainHeaderReader, time uint64, parent *types.Header) *big.Int {
	return big.NewInt(0)
}

func (l *LQC) Close() error {
	return nil
}
