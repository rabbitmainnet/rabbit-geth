package core

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/log"
)

var lqcLocalRecoveryKeyV1 = []byte("rabbit-lqc-local-state-recovery-v1")

// ReadLQCLocalStateRecoveryV1 returns a pending target, without changing the DB.
func ReadLQCLocalStateRecoveryV1(db ethdb.Database) (common.Hash, error) {
	has, err := db.Has(lqcLocalRecoveryKeyV1)
	if err != nil || !has {
		return common.Hash{}, err
	}
	value, err := db.Get(lqcLocalRecoveryKeyV1)
	if err != nil {
		return common.Hash{}, err
	}
	if len(value) != common.HashLength || common.BytesToHash(value) == (common.Hash{}) {
		return common.Hash{}, fmt.Errorf("invalid LQC local recovery target")
	}
	return common.BytesToHash(value), nil
}

// PrepareLQCLocalStateRecoveryV1 must run before NewBlockChain can rewind an
// unavailable head state. Keep the original target across interrupted retries.
// It is only called for LQC, whose producer can otherwise extend the repaired
// lower head even when every peer has suffered the same unclean shutdown.
func PrepareLQCLocalStateRecoveryV1(db ethdb.Database) (common.Hash, error) {
	pending, err := ReadLQCLocalStateRecoveryV1(db)
	if err != nil {
		return common.Hash{}, err
	}
	if pending != (common.Hash{}) {
		return pending, nil
	}
	target := rawdb.ReadHeadBlockHash(db)
	if target == (common.Hash{}) {
		return target, nil
	}
	if number, ok := rawdb.ReadHeaderNumber(db, target); !ok || number == 0 {
		return common.Hash{}, nil
	}
	if err := db.Put(lqcLocalRecoveryKeyV1, target[:]); err != nil {
		return common.Hash{}, err
	}
	return target, nil
}

// RecoverLQCLocalStateV1 re-executes locally stored blocks through InsertChain.
// It never writes a head marker or accepts a root without executing/validating
// the corresponding blocks. Call before networking and block production start.
func (bc *BlockChain) RecoverLQCLocalStateV1(target common.Hash) error {
	if target == (common.Hash{}) {
		return nil
	}
	if bc.chainConfig.LQC == nil {
		return fmt.Errorf("local LQC state recovery requires LQC consensus")
	}
	tip := bc.GetBlockByHash(target)
	if tip == nil {
		return fmt.Errorf("%w: LQC recovery block unavailable locally: %s", ErrLQCLocalRecoveryNeedsSyncV2, target)
	}
	head := bc.CurrentBlock()
	if head.Number.Cmp(tip.Number()) >= 0 {
		if bc.GetCanonicalHash(tip.NumberU64()) != target || !bc.HasState(head.Root) {
			return fmt.Errorf("LQC recovery target conflicts with current stateful chain: %s", target)
		}
		return bc.db.Delete(lqcLocalRecoveryKeyV1)
	}
	// Collect hashes first, so a missing body or a disconnected branch is
	// detected before execution. Bound memory and reject unexpected deep gaps.
	const maxBlocks = 4096
	var reverse []common.Hash
	for block := tip; ; {
		if bc.GetCanonicalHash(block.NumberU64()) == block.Hash() && bc.HasState(block.Root()) {
			break
		}
		if block.NumberU64() == 0 {
			return fmt.Errorf("LQC recovery has no available local ancestor within %d blocks", maxBlocks)
		}
		if len(reverse) == maxBlocks {
			return fmt.Errorf("%w: LQC recovery exceeds local replay limit of %d blocks", ErrLQCLocalRecoveryNeedsSyncV2, maxBlocks)
		}
		reverse = append(reverse, block.Hash())
		parent := bc.GetBlock(block.ParentHash(), block.NumberU64()-1)
		if parent == nil {
			return fmt.Errorf("%w: LQC recovery parent unavailable: number=%d hash=%s", ErrLQCLocalRecoveryNeedsSyncV2, block.NumberU64()-1, block.ParentHash())
		}
		block = parent
	}
	log.Info("Reexecuting stored LQC blocks after state repair", "from", head.Number, "target", tip.Number(), "hash", target, "blocks", len(reverse))
	for end := len(reverse); end > 0; {
		start := end - 32
		if start < 0 {
			start = 0
		}
		batch := make(types.Blocks, 0, end-start)
		for i := end - 1; i >= start; i-- {
			block := bc.GetBlockByHash(reverse[i])
			if block == nil {
				return fmt.Errorf("LQC recovery block disappeared: %s", reverse[i])
			}
			batch = append(batch, block)
		}
		if _, err := bc.InsertChain(batch); err != nil {
			return fmt.Errorf("LQC local recovery validation failed: %w", err)
		}
		end = start
	}
	if bc.CurrentBlock().Hash() != target || !bc.HasState(tip.Root()) {
		return fmt.Errorf("LQC local recovery did not restore target state: %s", target)
	}
	if err := bc.db.Delete(lqcLocalRecoveryKeyV1); err != nil {
		return err
	}
	log.Info("LQC local state recovery completed", "number", tip.Number(), "hash", target, "root", tip.Root())
	return nil
}
