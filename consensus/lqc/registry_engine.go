package lqc

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/lru"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
)

const registrySnapshotMemoryLimit = 256

var errRegistryProtocolUnavailable = errors.New("lqc canonical registry protocol unavailable")

// RegistryProtocolStatus is a read-only view of the canonical registry rules
// and head. It is exposed to the RPC layer so external clients can construct
// operations without duplicating chain configuration.
type RegistryProtocolStatus struct {
	ActivationBlock      uint64
	CurrentBlock         uint64
	NextBlock            uint64
	ProofDifficulty      uint64
	ActivationDelay      uint64
	HeartbeatWindow      uint64
	HeartbeatGrace       uint64
	RecoveryTimeoutMs    uint64
	MaxOperationLifetime uint64
	PoolCapacity         uint64
	RegistryRoot         common.Hash
	ParticipantCount     uint64
	ActiveForNextBlock   bool
}

// RegistryParticipantStatus is the canonical state of one address at the
// current head. EligibleNext is computed for the next block, which is the
// block against which a newly submitted operation is validated.
type RegistryParticipantStatus struct {
	Participant    CanonicalParticipant
	Exists         bool
	EligibleNext   bool
	CanonicalBlock uint64
	RegistryRoot   common.Hash
}

type registrySnapshotCache struct {
	entries *lru.Cache[common.Hash, *RegistrySnapshot]
}

func newRegistrySnapshotCache() *registrySnapshotCache {
	return &registrySnapshotCache{entries: lru.NewCache[common.Hash, *RegistrySnapshot](registrySnapshotMemoryLimit)}
}

func (l *LQC) registryProtocolEnabled(number *big.Int) bool {
	return l != nil && l.config != nil && l.config.RegistryProtocolBlock > 0 && number != nil && number.Sign() >= 0 && number.Uint64() >= l.config.RegistryProtocolBlock
}

func (l *LQC) registryRules() RegistrySnapshotRules {
	rules := RegistrySnapshotRules{}
	if l != nil && l.config != nil {
		rules.ProofDifficulty = l.config.ProofDifficulty
		rules.ActivationDelay = l.config.ActivationDelay
		rules.HeartbeatWindow = l.config.HeartbeatWindow
		rules.HeartbeatGrace = l.config.HeartbeatGrace
		rules.JailBlocks = l.config.JailBlocks
		rules.MaxMissedTurns = l.config.MaxMissedTurns
	}
	if rules.HeartbeatWindow == 0 {
		rules.HeartbeatWindow = 64
	}
	if rules.HeartbeatGrace == 0 {
		rules.HeartbeatGrace = 16
	}
	if rules.JailBlocks == 0 {
		rules.JailBlocks = 256
	}
	if rules.MaxMissedTurns == 0 {
		rules.MaxMissedTurns = 3
	}
	return rules
}

func (l *LQC) registryCheckpointInterval() uint64 {
	if l != nil && l.config != nil && l.config.EpochLength > 0 {
		return l.config.EpochLength
	}
	return 128
}

func registryChainID(chain consensus.ChainHeaderReader) (*big.Int, error) {
	if chain == nil || chain.Config() == nil || chain.Config().ChainID == nil {
		return nil, errRegistryProtocolUnavailable
	}
	return new(big.Int).Set(chain.Config().ChainID), nil
}

func (l *LQC) rememberRegistrySnapshot(snapshot *RegistrySnapshot) {
	if l == nil || snapshot == nil {
		return
	}
	if l.registryCache != nil && l.registryCache.entries != nil {
		l.registryCache.entries.Add(snapshot.Hash, snapshot)
	}
	if l.db != nil && snapshot.Number%l.registryCheckpointInterval() == 0 {
		if err := StoreRegistrySnapshot(l.db, snapshot); err != nil {
			log.Warn("Failed to store LQC registry checkpoint", "number", snapshot.Number, "hash", snapshot.Hash, "err", err)
		}
	}
}

func (l *LQC) cachedRegistrySnapshot(number uint64, hash common.Hash) (*RegistrySnapshot, bool) {
	if l == nil || hash == (common.Hash{}) {
		return nil, false
	}
	if l.registryCache != nil && l.registryCache.entries != nil {
		if snapshot, ok := l.registryCache.entries.Get(hash); ok && snapshot != nil && snapshot.Number == number {
			return snapshot, true
		}
	}
	if l.db != nil && number%l.registryCheckpointInterval() == 0 {
		snapshot, err := LoadRegistrySnapshot(l.db, hash)
		if err == nil && snapshot.Number == number {
			if l.registryCache != nil && l.registryCache.entries != nil {
				l.registryCache.entries.Add(hash, snapshot)
			}
			return snapshot, true
		}
		if err != nil {
			log.Debug("LQC registry checkpoint unavailable", "number", number, "hash", hash, "err", err)
		}
	}
	return nil, false
}

func (l *LQC) applyRegistrySnapshotHeaderMaybeV3(
	chain consensus.ChainHeaderReader,
	parent *RegistrySnapshot,
	header *types.Header,
) (*RegistrySnapshot, error) {
	if parent == nil ||
		header == nil ||
		header.Number == nil ||
		parent.Number == ^uint64(0) ||
		header.Number.Uint64() != parent.Number+1 ||
		header.ParentHash != parent.Hash {
		return nil, ErrRegistrySnapshotChainMismatch
	}
	if envelope, err := DecodeLQCHeaderExtraV3(
		header.Extra,
		MaxWorkTicketsPerBlockV1,
	); err == nil {
		blockNumber := header.Number.Uint64()
		if envelope.BlockNumber != blockNumber {
			return nil, ErrLQCHeaderBlockMismatchV3
		}
		chainID, err := registryChainID(chain)
		if err != nil {
			return nil, err
		}
		rules := l.registryRules()
		v2Extra, err := EncodeRegistryHeaderExtra(
			envelope.BlockNumber,
			envelope.RegistryRoot,
			envelope.RegistryOperations,
		)
		if err != nil {
			return nil, err
		}
		validated, err := ValidateRegistryHeaderExtra(
			chainID,
			blockNumber,
			rules.ProofDifficulty,
			v2Extra,
		)
		if err != nil {
			return nil, err
		}

		// Header V3 has two canonical registry-state modes. When Work V2
		// seats are active, registry state changes only through committed
		// operations. During zero-work/open-registry fallback, the legacy
		// heartbeat/missed-turn transition is used. The committed registry
		// root lets historical replay select the correct transition without
		// requiring transient Work runtime caches.
		registry, err := parent.Registry()
		if err != nil {
			return nil, err
		}
		for _, operation := range validated.Operations {
			if err := registry.ApplyOperation(
				chainID,
				blockNumber,
				rules.ProofDifficulty,
				operation,
			); err != nil {
				return nil, err
			}
		}
		if registry.Root() == envelope.RegistryRoot {
			return newRegistrySnapshot(blockNumber, header.Hash(), registry), nil
		}

		synthetic := types.CopyHeader(header)
		synthetic.Extra = v2Extra
		legacy, err := parent.ApplyHeaderWithOpenActivation(
			chainID,
			rules,
			synthetic,
			l.openActivationForHeader(chain, header),
		)
		if err != nil {
			return nil, err
		}
		legacyRegistry, err := legacy.Registry()
		if err != nil {
			return nil, err
		}
		if legacyRegistry.Root() != envelope.RegistryRoot {
			return nil, ErrRegistryRootMismatch
		}
		return newRegistrySnapshot(blockNumber, header.Hash(), legacyRegistry), nil
	}
	chainID, err := registryChainID(chain)
	if err != nil {
		return nil, err
	}
	return parent.ApplyHeaderWithOpenActivation(
		chainID,
		l.registryRules(),
		header,
		l.openActivationForHeader(chain, header),
	)
}

func (l *LQC) registrySnapshotAt(chain consensus.ChainHeaderReader, number uint64, hash common.Hash) (*RegistrySnapshot, error) {
	if l == nil || l.config == nil || l.config.RegistryProtocolBlock == 0 || hash == (common.Hash{}) {
		return nil, errRegistryProtocolUnavailable
	}
	activation := l.config.RegistryProtocolBlock
	if number < activation-1 {
		return nil, ErrRegistrySnapshotChainMismatch
	}
	if snapshot, ok := l.cachedRegistrySnapshot(number, hash); ok {
		return snapshot, nil
	}

	currentNumber, currentHash := number, hash
	pending := make([]*types.Header, 0, l.registryCheckpointInterval())
	var base *RegistrySnapshot
	for currentNumber >= activation {
		if snapshot, ok := l.cachedRegistrySnapshot(currentNumber, currentHash); ok {
			base = snapshot
			break
		}
		if chain == nil {
			return nil, errUnknownAncestor
		}
		header := chain.GetHeader(currentHash, currentNumber)
		if header == nil {
			return nil, errUnknownAncestor
		}
		pending = append(pending, header)
		currentHash = header.ParentHash
		currentNumber--
	}
	if base == nil {
		if currentNumber != activation-1 {
			return nil, ErrRegistrySnapshotChainMismatch
		}
		var err error
		base, err = NewBootstrapRegistrySnapshot(currentNumber, currentHash, l.config.BootstrapParticipants)
		if err != nil {
			return nil, err
		}
		l.rememberRegistrySnapshot(base)
	}
	var err error
	for index := len(pending) - 1; index >= 0; index-- {
		base, err = l.applyRegistrySnapshotHeaderMaybeV3(
			chain,
			base,
			pending[index],
		)
		if err != nil {
			return nil, err
		}
		l.rememberRegistrySnapshot(base)
	}
	if base.Number != number || base.Hash != hash {
		return nil, ErrRegistrySnapshotChainMismatch
	}
	return base, nil
}

func (l *LQC) registryParentSnapshot(chain consensus.ChainHeaderReader, header *types.Header) (*RegistrySnapshot, error) {
	if header == nil || header.Number == nil || header.Number.Sign() <= 0 || !l.registryProtocolEnabled(header.Number) {
		return nil, errRegistryProtocolUnavailable
	}
	return l.registrySnapshotAt(chain, header.Number.Uint64()-1, header.ParentHash)
}

func (l *LQC) canonicalSelection(snapshot *RegistrySnapshot, header *types.Header) (HybridSelection, error) {
	if snapshot == nil || header == nil || header.Number == nil {
		return HybridSelection{}, ErrInvalidRegistrySnapshot
	}
	registry, err := snapshot.Registry()
	if err != nil {
		return HybridSelection{}, err
	}
	rules := l.registryRules()
	ordered := registry.OrderedParticipantsForBlock(
		header.ParentHash,
		header.Number.Uint64(),
		rules.ActivationDelay,
		rules.HeartbeatWindow,
		rules.HeartbeatGrace,
	)

	selection := HybridSelection{Ordered: ordered}
	if len(ordered) == 0 {
		return selection, nil
	}
	selection.Producer = &selection.Ordered[0]

	cfg := l.hybridCfg()
	fallbackEnd := 1 + int(cfg.FallbackCount)
	if fallbackEnd > len(ordered) {
		fallbackEnd = len(ordered)
	}
	if fallbackEnd > 1 {
		selection.Fallbacks = append(selection.Fallbacks, ordered[1:fallbackEnd]...)
	}

	committeeSize := uint64(0)
	if l != nil && l.config != nil && l.config.CommitteeSize > 0 {
		// Explicit CommitteeSize remains supported for tests/devnets and
		// deliberately fixed configurations.
		committeeSize = l.config.CommitteeSize
	} else {
		// Rabbit mainnet omits CommitteeSize and therefore uses the
		// dynamic rule: ceil(active*10%), bounded by CommitteeMin/Max.
		committeeMin := uint64(32)
		committeeMax := uint64(128)
		if l != nil && l.config != nil {
			if l.config.CommitteeMin > 0 {
				committeeMin = l.config.CommitteeMin
			}
			if l.config.CommitteeMax > 0 {
				committeeMax = l.config.CommitteeMax
			}
		}
		committeeSize = ComputeCommitteeSizeWithBounds(
			uint64(len(ordered)),
			committeeMin,
			committeeMax,
		)
	}
	committeeEnd := fallbackEnd + int(committeeSize)
	if committeeEnd > len(ordered) {
		committeeEnd = len(ordered)
	}
	if committeeEnd > fallbackEnd {
		selection.Committee = append(selection.Committee, ordered[fallbackEnd:committeeEnd]...)
	}
	return selection, nil
}

func (l *LQC) canonicalSelectionForHeader(chain consensus.ChainHeaderReader, header *types.Header) (HybridSelection, *RegistrySnapshot, error) {
	parent, err := l.registryParentSnapshot(chain, header)
	if err != nil {
		return HybridSelection{}, nil, err
	}
	selection, err := l.canonicalSelection(parent, header)
	return selection, parent, err
}

func (l *LQC) prepareCanonicalRegistryExtra(chain consensus.ChainHeaderReader, header *types.Header) (HybridSelection, error) {
	if chain == nil || header == nil || header.Number == nil {
		return HybridSelection{}, errInvalidBlockNumber
	}

	// Block 1 and post-timeout recovery are open activation blocks. The
	// producer is selected by the miner itself and seeds a fresh operational
	// registry without changing execution state or chain history.
	// We still build the canonical registry envelope so the subsequent
	// Work V1/V3 hook has a valid RegistryRoot/Operations payload to extend.
	if l.openActivationForHeader(chain, header) {
		if header.Coinbase == (common.Address{}) {
			return HybridSelection{}, errors.New("lqc block 1 requires producer coinbase")
		}

		registry := NewCanonicalRegistry()
		if err := registry.ActivatePermissionlessProducer(
			header.Coinbase,
			header.Number.Uint64(),
		); err != nil {
			return HybridSelection{}, err
		}

		operations := make([]RegistryOperation, 0, MaxRegistryOperationsPerBlock)
		if l.registryPool != nil {
			l.registryPool.PruneExpired(header.Number.Uint64())
			for _, operation := range l.registryPool.Pending(header.Number.Uint64()) {
				if len(operations) >= MaxRegistryOperationsPerBlock {
					break
				}

				candidate := registry.Clone()
				chainID, chainErr := registryChainID(chain)
				if chainErr != nil {
					return HybridSelection{}, chainErr
				}

				if applyErr := candidate.ApplyOperation(
					chainID,
					header.Number.Uint64(),
					l.registryRules().ProofDifficulty,
					operation,
				); applyErr != nil {
					continue
				}

				registry = candidate
				operations = append(operations, operation)
			}
		}

		extra, err := EncodeRegistryHeaderExtra(
			header.Number.Uint64(),
			registry.Root(),
			operations,
		)
		if err != nil {
			return HybridSelection{}, err
		}
		header.Extra = extra

		selection := HybridSelection{
			Ordered: []HybridParticipant{{
				Address:       header.Coinbase,
				Payout:        header.Coinbase,
				Bond:          big.NewInt(25),
				RegisteredAt:  header.Number.Uint64(),
				LastHeartbeat: header.Number.Uint64(),
				Status:        ParticipantActiveCandidate,
			}},
		}
		selection.Producer = &selection.Ordered[0]

		return selection, nil
	}

	selection, parent, err := l.canonicalSelectionForHeader(chain, header)
	if err != nil {
		return HybridSelection{}, err
	}

	allowed, queuePos := IsAuthorAllowed(selection, header.Coinbase)
	if !allowed {
		return HybridSelection{}, fmt.Errorf("%w: %s", ErrUnauthorizedRegistryProducer, header.Coinbase)
	}

	registry, err := parent.Registry()
	if err != nil {
		return HybridSelection{}, err
	}

	rules := l.registryRules()
	for index := 0; index < queuePos; index++ {
		if err := registry.ApplyMissedTurn(
			selection.Ordered[index].Address,
			header.Number.Uint64(),
			rules.MaxMissedTurns,
			rules.JailBlocks,
		); err != nil {
			return HybridSelection{}, err
		}
	}

	if err := registry.MarkProducerHeartbeat(header.Coinbase, header.Number.Uint64()); err != nil {
		return HybridSelection{}, err
	}

	operations := make([]RegistryOperation, 0, MaxRegistryOperationsPerBlock)
	if l.registryPool != nil {
		l.registryPool.PruneExpired(header.Number.Uint64())
		for _, operation := range l.registryPool.Pending(header.Number.Uint64()) {
			if len(operations) >= MaxRegistryOperationsPerBlock {
				break
			}

			candidate := registry.Clone()
			chainID, chainErr := registryChainID(chain)
			if chainErr != nil {
				return HybridSelection{}, chainErr
			}

			if applyErr := candidate.ApplyOperation(
				chainID,
				header.Number.Uint64(),
				l.registryRules().ProofDifficulty,
				operation,
			); applyErr != nil {
				continue
			}

			registry = candidate
			operations = append(operations, operation)
		}
	}

	header.Extra, err = EncodeRegistryHeaderExtra(
		header.Number.Uint64(),
		registry.Root(),
		operations,
	)
	if err != nil {
		return HybridSelection{}, err
	}

	return selection, nil
}

// SubmitRegistryOperation validates an operation against the current canonical
// snapshot before admitting it to the bounded relay pool.
func (l *LQC) SubmitRegistryOperation(chain consensus.ChainHeaderReader, operation RegistryOperation) (common.Hash, error) {
	if l == nil || l.registryPool == nil || chain == nil || chain.CurrentHeader() == nil {
		return common.Hash{}, ErrRegistryPoolDisabled
	}
	head := chain.CurrentHeader()
	if head.Number == nil || head.Number.Uint64() == ^uint64(0) {
		return common.Hash{}, errInvalidBlockNumber
	}
	nextNumber := head.Number.Uint64() + 1
	if !l.registryProtocolEnabled(new(big.Int).SetUint64(nextNumber)) {
		return common.Hash{}, ErrRegistryPoolDisabled
	}
	snapshot, err := l.registrySnapshotAt(chain, head.Number.Uint64(), head.Hash())
	if err != nil {
		return common.Hash{}, err
	}
	registry, err := snapshot.Registry()
	if err != nil {
		return common.Hash{}, err
	}
	chainID, err := registryChainID(chain)
	if err != nil {
		return common.Hash{}, err
	}
	if err := registry.ApplyOperation(chainID, nextNumber, l.registryRules().ProofDifficulty, operation); err != nil {
		return common.Hash{}, err
	}
	return l.registryPool.Add(chainID, operation)
}

func (l *LQC) PendingRegistryOperations(chain consensus.ChainHeaderReader) []RegistryOperation {
	if l == nil || l.registryPool == nil || chain == nil || chain.CurrentHeader() == nil || chain.CurrentHeader().Number == nil {
		return nil
	}
	nextNumber := chain.CurrentHeader().Number.Uint64() + 1
	l.registryPool.PruneExpired(nextNumber)
	return l.registryPool.Pending(nextNumber)
}

func (l *LQC) RegistryOperationPoolStatus() RegistryPoolStatus {
	if l == nil {
		return RegistryPoolStatus{Capacity: MaxRegistryPoolOperations}
	}
	return l.registryPool.Status()
}

// RegistryStatus returns the protocol parameters and canonical state at the
// current head. Before the activation parent is reached, the protocol rules
// are still returned but the canonical root remains empty.
func (l *LQC) RegistryStatus(chain consensus.ChainHeaderReader) (RegistryProtocolStatus, error) {
	if l == nil || l.config == nil || l.config.RegistryProtocolBlock == 0 || chain == nil {
		return RegistryProtocolStatus{}, errRegistryProtocolUnavailable
	}
	head := chain.CurrentHeader()
	if head == nil || head.Number == nil || head.Number.Sign() < 0 || head.Number.Uint64() == ^uint64(0) {
		return RegistryProtocolStatus{}, errInvalidBlockNumber
	}
	rules := l.registryRules()
	status := RegistryProtocolStatus{
		ActivationBlock:      l.config.RegistryProtocolBlock,
		CurrentBlock:         head.Number.Uint64(),
		NextBlock:            head.Number.Uint64() + 1,
		ProofDifficulty:      rules.ProofDifficulty,
		ActivationDelay:      rules.ActivationDelay,
		HeartbeatWindow:      rules.HeartbeatWindow,
		HeartbeatGrace:       rules.HeartbeatGrace,
		RecoveryTimeoutMs:    l.config.RecoveryTimeoutMs,
		MaxOperationLifetime: MaxRegistryOperationLifetime,
		PoolCapacity:         MaxRegistryPoolOperations,
	}
	status.ActiveForNextBlock = l.registryProtocolEnabled(new(big.Int).SetUint64(status.NextBlock))
	if !status.ActiveForNextBlock {
		return status, nil
	}
	snapshot, err := l.registrySnapshotAt(chain, status.CurrentBlock, head.Hash())
	if err != nil {
		return RegistryProtocolStatus{}, err
	}
	if _, err := snapshot.Registry(); err != nil {
		return RegistryProtocolStatus{}, err
	}
	status.RegistryRoot = snapshot.RegistryRoot
	status.ParticipantCount = uint64(len(snapshot.Participants))
	return status, nil
}

// RegistryParticipant returns the canonical state for address at the current
// head. It never consults the relay pool, which is deliberately non-consensus.
func (l *LQC) RegistryParticipant(chain consensus.ChainHeaderReader, address common.Address) (RegistryParticipantStatus, error) {
	if address == (common.Address{}) {
		return RegistryParticipantStatus{}, ErrInvalidRegistryAddress
	}
	if l == nil || l.config == nil || l.config.RegistryProtocolBlock == 0 || chain == nil {
		return RegistryParticipantStatus{}, errRegistryProtocolUnavailable
	}
	head := chain.CurrentHeader()
	if head == nil || head.Number == nil || head.Number.Sign() < 0 || head.Number.Uint64() == ^uint64(0) {
		return RegistryParticipantStatus{}, errInvalidBlockNumber
	}
	currentBlock := head.Number.Uint64()
	nextBlock := currentBlock + 1
	if !l.registryProtocolEnabled(new(big.Int).SetUint64(nextBlock)) {
		return RegistryParticipantStatus{}, errRegistryProtocolUnavailable
	}
	snapshot, err := l.registrySnapshotAt(chain, currentBlock, head.Hash())
	if err != nil {
		return RegistryParticipantStatus{}, err
	}
	registry, err := snapshot.Registry()
	if err != nil {
		return RegistryParticipantStatus{}, err
	}
	participant, exists := registry.Participant(address)
	rules := l.registryRules()
	return RegistryParticipantStatus{
		Participant:    participant,
		Exists:         exists,
		CanonicalBlock: currentBlock,
		RegistryRoot:   snapshot.RegistryRoot,
		EligibleNext: exists && registry.IsEligibleParticipant(
			address,
			nextBlock,
			rules.ActivationDelay,
			rules.HeartbeatWindow,
			rules.HeartbeatGrace,
		),
	}, nil
}

func (l *LQC) verifyCanonicalRegistryHeader(chain consensus.ChainHeaderReader, header *types.Header) (HybridSelection, *RegistrySnapshot, error) {
	selection, parent, err := l.canonicalSelectionForHeader(chain, header)
	if err != nil {
		return HybridSelection{}, nil, err
	}
	chainID, err := registryChainID(chain)
	if err != nil {
		return HybridSelection{}, nil, err
	}
	child, err := parent.ApplyHeaderWithOpenActivation(
		chainID,
		l.registryRules(),
		header,
		l.openActivationForHeader(chain, header),
	)
	if err != nil {
		return HybridSelection{}, nil, err
	}
	if l.openActivationForHeader(chain, header) {
		selection = HybridSelection{Ordered: []HybridParticipant{{
			Address:       header.Coinbase,
			Payout:        header.Coinbase,
			Bond:          big.NewInt(25),
			RegisteredAt:  header.Number.Uint64(),
			LastHeartbeat: header.Number.Uint64(),
			Status:        ParticipantActiveCandidate,
		}}}
		selection.Producer = &selection.Ordered[0]
	}
	return selection, child, nil
}
