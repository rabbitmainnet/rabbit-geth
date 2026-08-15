// Copyright 2014 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

// Package miner implements Ethereum block creation and mining.
package miner

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"os"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/txpool"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
)

// Backend wraps all methods required for mining. Only full node is capable
// to offer all the functions here.
type Backend interface {
	BlockChain() *core.BlockChain
	TxPool() *txpool.TxPool
	AccountManager() *accounts.Manager
}

// Config is the configuration parameters of mining.
type Config struct {
	Enabled             bool           `toml:"-"`          // Enable local LQC block production.
	Etherbase           common.Address `toml:"-"`          // Deprecated
	PendingFeeRecipient common.Address `toml:"-"`          // Address for pending block rewards.
	ExtraData           hexutil.Bytes  `toml:",omitempty"` // Block extra data set by the miner
	GasCeil             uint64         // Target gas ceiling for mined blocks.
	GasPrice            *big.Int       // Minimum gas price for mining a transaction
	Recommit            time.Duration  // The time interval for miner to re-create mining work.
	MaxBlobsPerBlock    int            // Maximum number of blobs per block (0 for unset uses protocol default)
}

// DefaultConfig contains default settings for miner.
var DefaultConfig = Config{
	GasCeil:  60_000_000,
	GasPrice: big.NewInt(params.GWei / 1000),

	// The default recommit time is chosen as two seconds since
	// consensus-layer usually will wait a half slot of time(6s)
	// for payload generation. It should be enough for Geth to
	// run 3 rounds.
	Recommit: 2 * time.Second,
}

// Miner is the main object which takes care of submitting new work to consensus
// engine and gathering the sealing result.
type Miner struct {
	backend     Backend
	confMu      sync.RWMutex // The lock used to protect the config fields: GasCeil, GasTip and Extradata
	config      *Config
	chainConfig *params.ChainConfig
	engine      consensus.Engine
	txpool      *txpool.TxPool
	prio        []common.Address // A list of senders to prioritize
	chain       *core.BlockChain
	pending     *pending
	pendingMu   sync.Mutex // Lock protects the pending block

	lqcMu      sync.Mutex
	lqcRunning bool
	lqcStop    chan struct{}
}

// New creates a new miner with provided config.
func New(eth Backend, config Config, engine consensus.Engine) *Miner {
	return &Miner{
		backend:     eth,
		config:      &config,
		chainConfig: eth.BlockChain().Config(),
		engine:      engine,
		txpool:      eth.TxPool(),
		chain:       eth.BlockChain(),
		pending:     &pending{},
	}
}

func (miner *Miner) configuredLQCCoinbase() common.Address {
	miner.confMu.RLock()
	defer miner.confMu.RUnlock()

	if miner.config.PendingFeeRecipient != (common.Address{}) {
		return miner.config.PendingFeeRecipient
	}
	if miner.config.Etherbase != (common.Address{}) {
		return miner.config.Etherbase
	}
	if env := os.Getenv("RABBIT_LQC_COINBASE"); env != "" {
		addr := common.HexToAddress(env)
		if addr != (common.Address{}) {
			return addr
		}
	}
	return common.Address{}
}

func (miner *Miner) lqcCoinbase() common.Address {
	if addr := miner.configuredLQCCoinbase(); addr != (common.Address{}) {
		return addr
	}

	local := miner.localParticipant()
	if local.Allowed {
		return local.Address
	}
	return common.Address{}
}

func (miner *Miner) localParticipant() consensus.LocalParticipant {
	var addrs []common.Address

	// An explicitly configured LQC coinbase must be backed by a local or external
	// wallet. Registry ownership alone is not enough: every produced block is
	// independently signed after execution finalizes its roots.
	if addr := miner.configuredLQCCoinbase(); addr != (common.Address{}) {
		am := miner.backend.AccountManager()
		if am == nil {
			return consensus.LocalParticipant{QueuePos: -1}
		}
		if _, err := am.Find(accounts.Account{Address: addr}); err != nil {
			return consensus.LocalParticipant{QueuePos: -1}
		}
		addrs = append(addrs, addr)
	} else {
		am := miner.backend.AccountManager()
		if am != nil {
			for _, wallet := range am.Wallets() {
				for _, account := range wallet.Accounts() {
					addrs = append(addrs, account.Address)
				}
			}
		}
	}

	if len(addrs) == 0 {
		return consensus.LocalParticipant{
			QueuePos: -1,
		}
	}

	if resolver, ok := miner.engine.(consensus.LocalParticipantResolver); ok {
		header := miner.chain.CurrentHeader()
		return resolver.ResolveLocalParticipant(
			miner.chain,
			header,
			addrs,
		)
	}

	return consensus.LocalParticipant{
		Address:  addrs[0],
		QueuePos: 0,
		Allowed:  true,
	}
}

func (miner *Miner) signConsensusHeader(address common.Address, payload []byte) ([]byte, error) {
	if address == (common.Address{}) {
		return nil, errors.New("missing consensus signer address")
	}
	am := miner.backend.AccountManager()
	if am == nil {
		return nil, errors.New("account manager unavailable for consensus signing")
	}
	account := accounts.Account{Address: address}
	wallet, err := am.Find(account)
	if err != nil {
		return nil, fmt.Errorf("find consensus signer %s: %w", address, err)
	}
	signature, err := wallet.SignData(account, accounts.MimetypeClique, payload)
	if err != nil {
		return nil, fmt.Errorf("sign consensus header with %s: %w", address, err)
	}
	return signature, nil
}

func (miner *Miner) StartLQCDevnet() {
	if miner.chainConfig == nil || miner.chainConfig.LQC == nil {
		return
	}
	coinbase := miner.lqcCoinbase()

	if coinbase == (common.Address{}) {
		log.Warn("LQC devnet producer not started: no coinbase configured")
		return
	}

	miner.lqcMu.Lock()
	defer miner.lqcMu.Unlock()

	if miner.lqcRunning {
		return
	}
	miner.lqcStop = make(chan struct{})
	miner.lqcRunning = true

	go miner.lqcDevnetLoop()
	log.Info("LQC devnet producer loop started", "coinbase", coinbase)
}

func (miner *Miner) StopLQCDevnet() {
	miner.lqcMu.Lock()
	defer miner.lqcMu.Unlock()

	if !miner.lqcRunning {
		return
	}
	close(miner.lqcStop)
	miner.lqcRunning = false
	miner.lqcStop = nil
	log.Info("LQC devnet producer loop stopped")
}

func (miner *Miner) lqcTargetBlockSeconds() uint64 {
	sec := uint64(15)
	if miner.chainConfig != nil && miner.chainConfig.LQC != nil && miner.chainConfig.LQC.TargetBlockTimeMs > 0 {
		sec = miner.chainConfig.LQC.TargetBlockTimeMs / 1000
		if sec == 0 {
			sec = 1
		}
	}
	return sec
}

func (miner *Miner) lqcFallbackWindowSeconds() uint64 {
	sec := uint64(5)
	if miner.chainConfig != nil && miner.chainConfig.LQC != nil && miner.chainConfig.LQC.FallbackWindowMs > 0 {
		sec = miner.chainConfig.LQC.FallbackWindowMs / 1000
		if sec == 0 {
			sec = 1
		}
	}
	return sec
}

func (miner *Miner) lqcMinAllowedTime(parentTime uint64, queuePos int) uint64 {
	base := miner.lqcTargetBlockSeconds()
	if queuePos <= 0 {
		return parentTime + base
	}
	return parentTime + base + uint64(queuePos)*miner.lqcFallbackWindowSeconds()
}

type lqcBlockInserter interface {
	CurrentBlock() *types.Header
	InsertChain(types.Blocks) (int, error)
}

// insertLQCBlockIfParentCurrent prevents a block built by a fallback slot from
// being inserted after the canonical head has already advanced. The captured
// parent and the built block are checked together immediately before insertion.
func insertLQCBlockIfParentCurrent(chain lqcBlockInserter, parent *types.Header, block *types.Block) (bool, error) {
	if chain == nil || parent == nil || parent.Number == nil || block == nil {
		return false, nil
	}
	current := chain.CurrentBlock()
	if current == nil || current.Number == nil ||
		current.Number.Cmp(parent.Number) != 0 ||
		current.Hash() != parent.Hash() ||
		block.NumberU64() != parent.Number.Uint64()+1 ||
		block.ParentHash() != parent.Hash() {
		return false, nil
	}
	_, err := chain.InsertChain(types.Blocks{block})
	return true, err
}

func (miner *Miner) lqcDevnetLoop() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-miner.lqcStop:
			return
		case <-ticker.C:
			coinbase := miner.lqcCoinbase()
			if coinbase == (common.Address{}) {
				log.Warn("LQC devnet skipped block: missing producer address")
				continue
			}

			head := miner.chain.CurrentBlock()
			if head == nil {
				log.Warn("LQC devnet skipped block: missing current head")
				continue
			}
			local := miner.localParticipant()

			if !local.Allowed ||
				local.Address == (common.Address{}) ||
				local.Address != coinbase ||
				local.QueuePos < 0 {

				log.Info(
					"LCQ local account outside producer queue",
					"coinbase", coinbase,
					"next", head.Number.Uint64()+1,
				)
				continue
			}

			queuePos := local.QueuePos
			topProducer := local.Address
			now := uint64(time.Now().Unix())
			if head.Number.Uint64() == 0 && queuePos > 0 {
				// A fallback at genesis must wait for its deterministic slot,
				// but it must not be blocked forever when the primary is offline.
				log.Info("LCQ genesis fallback armed for deterministic slot",
					"coinbase", coinbase,
					"next", head.Number.Uint64()+1,
					"queuePos", queuePos,
				)
			}

			minTime := miner.lqcMinAllowedTime(head.Time, queuePos)
			if now < minTime {
				log.Info("LCQ waiting slot", "coinbase", coinbase, "next", head.Number.Uint64()+1, "queuePos", queuePos, "now", now, "minTime", minTime)
				continue
			}

			log.Info("LCQ selected to build", "coinbase", coinbase, "next", head.Number.Uint64()+1, "queuePos", queuePos, "producer", topProducer, "now", now, "minTime", minTime)

			var withdrawals types.Withdrawals
			if miner.chainConfig.IsShanghai(new(big.Int).Add(head.Number, big.NewInt(1)), now) {
				withdrawals = []*types.Withdrawal{}
			}

			ret := miner.generateWork(context.Background(), &generateParams{
				timestamp:   now,
				forceTime:   false,
				parentHash:  head.Hash(),
				coinbase:    coinbase,
				random:      common.Hash{},
				withdrawals: withdrawals,
				beaconRoot:  nil,
				noTxs:       false,
			}, false)
			if ret.err != nil {
				log.Warn("LQC devnet block build failed", "err", ret.err)
				continue
			}
			if ret.block == nil {
				log.Warn("LQC devnet block build returned nil block")
				continue
			}

			inserted, err := insertLQCBlockIfParentCurrent(miner.chain, head, ret.block)
			if err != nil {
				log.Warn("LQC devnet block insert failed", "number", ret.block.NumberU64(), "err", err)
				continue
			}
			if !inserted {
				current := miner.chain.CurrentBlock()
				var currentNumber uint64
				var currentHash common.Hash
				if current != nil {
					currentHash = current.Hash()
					if current.Number != nil {
						currentNumber = current.Number.Uint64()
					}
				}
				log.Info("LQC devnet discarded stale built block",
					"number", ret.block.NumberU64(),
					"parent", ret.block.ParentHash(),
					"capturedParent", head.Hash(),
					"currentNumber", currentNumber,
					"currentHash", currentHash,
				)
				continue
			}
			log.Info("LQC devnet block inserted", "number", ret.block.NumberU64(), "hash", ret.block.Hash(), "txs", len(ret.block.Transactions()), "queuePos", queuePos)
		}
	}
}

// Pending returns the currently pending block and associated receipts, logs
// and statedb. The returned values can be nil in case the pending block is
// not initialized.
func (miner *Miner) Pending() (*types.Block, types.Receipts, *state.StateDB) {
	pending := miner.getPending()
	if pending == nil {
		return nil, nil, nil
	}
	return pending.block, pending.receipts, pending.stateDB.Copy()
}

// SetExtra sets the content used to initialize the block extra field.
func (miner *Miner) SetExtra(extra []byte) error {
	if uint64(len(extra)) > params.MaximumExtraDataSize {
		return fmt.Errorf("extra exceeds max length. %d > %v", len(extra), params.MaximumExtraDataSize)
	}
	miner.confMu.Lock()
	miner.config.ExtraData = extra
	miner.confMu.Unlock()
	return nil
}

// SetPrioAddresses sets a list of addresses to prioritize for transaction inclusion.
func (miner *Miner) SetPrioAddresses(prio []common.Address) {
	miner.confMu.Lock()
	miner.prio = prio
	miner.confMu.Unlock()
}

// SetGasCeil sets the gaslimit to strive for when mining blocks post 1559.
// For pre-1559 blocks, it sets the ceiling.
func (miner *Miner) SetGasCeil(ceil uint64) {
	miner.confMu.Lock()
	miner.config.GasCeil = ceil
	miner.confMu.Unlock()
}

// SetGasTip sets the minimum gas tip for inclusion.
func (miner *Miner) SetGasTip(tip *big.Int) error {
	miner.confMu.Lock()
	miner.config.GasPrice = tip
	miner.confMu.Unlock()
	return nil
}

// BuildPayload builds the payload according to the provided parameters.
func (miner *Miner) BuildPayload(ctx context.Context, args *BuildPayloadArgs, witness bool) (*Payload, error) {
	return miner.buildPayload(ctx, args, witness)
}

// getPending retrieves the pending block based on the current head block.
// The result might be nil if pending generation is failed.
func (miner *Miner) getPending() *newPayloadResult {
	header := miner.chain.CurrentHeader()
	miner.pendingMu.Lock()
	defer miner.pendingMu.Unlock()

	if cached := miner.pending.resolve(header.Hash()); cached != nil {
		return cached
	}
	var (
		timestamp   = uint64(time.Now().Unix())
		childNumber = new(big.Int).Add(header.Number, big.NewInt(1))
		withdrawal  types.Withdrawals
		slotNum     *uint64
	)
	if miner.chainConfig.IsShanghai(childNumber, timestamp) {
		withdrawal = []*types.Withdrawal{}
	}
	// Post-Amsterdam, prepareWork requires a slot number (EIP-7843). The pending
	// block is synthetic and has no canonical slot, so derive one from the parent
	// when available and fall back to zero otherwise.
	if miner.chainConfig.IsAmsterdam(childNumber, timestamp) {
		var n uint64
		if header.SlotNumber != nil {
			n = *header.SlotNumber + 1
		}
		slotNum = &n
	}
	ret := miner.generateWork(context.Background(),
		&generateParams{
			timestamp:   timestamp,
			forceTime:   false,
			parentHash:  header.Hash(),
			coinbase:    miner.config.PendingFeeRecipient,
			random:      common.Hash{},
			withdrawals: withdrawal,
			beaconRoot:  nil,
			slotNum:     slotNum,
			noTxs:       false,
		}, false) // we will never make a witness for a pending block
	if ret.err != nil {
		return nil
	}
	miner.pending.update(header.Hash(), ret)
	return ret
}
