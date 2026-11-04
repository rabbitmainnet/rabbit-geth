package core

import (
	"encoding/binary"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/log"
)

var lqcLocalRecoveryKeyV1 = []byte("rabbit-lqc-local-state-recovery-v1")
var lqcLocalRecoveryMetadataKeyV2 = []byte("rabbit-lqc-local-state-recovery-metadata-v2")

// ReadLQCLocalStateRecoveryV1 returns a pending target, without changing the DB.
func ReadLQCLocalStateRecoveryV1(db ethdb.Database) (common.Hash, error) {
	hasV1, err := db.Has(lqcLocalRecoveryKeyV1)
	if err != nil {
		return common.Hash{}, err
	}
	hasV2, err := db.Has(lqcLocalRecoveryMetadataKeyV2)
	if err != nil {
		return common.Hash{}, err
	}
	if !hasV1 {
		if hasV2 {
			return common.Hash{}, fmt.Errorf("orphan LQC recovery V2 metadata")
		}
		return common.Hash{}, nil
	}

	value, err := db.Get(lqcLocalRecoveryKeyV1)
	if err != nil {
		return common.Hash{}, err
	}
	if len(value) != common.HashLength {
		return common.Hash{}, fmt.Errorf("invalid LQC recovery V1 length")
	}
	target := common.BytesToHash(value)
	if target == (common.Hash{}) {
		return common.Hash{}, fmt.Errorf("invalid LQC recovery V1 target")
	}

	// Legacy V1 contains only a hash. Never infer or invent its number.
	if !hasV2 {
		return target, nil
	}

	metadata, err := db.Get(lqcLocalRecoveryMetadataKeyV2)
	if err != nil {
		return common.Hash{}, err
	}
	if len(metadata) != common.HashLength+8 {
		return common.Hash{}, fmt.Errorf("invalid LQC recovery V2 metadata length")
	}
	if common.BytesToHash(metadata[:common.HashLength]) != target {
		return common.Hash{}, fmt.Errorf("LQC recovery V1/V2 target mismatch")
	}
	number := binary.BigEndian.Uint64(metadata[common.HashLength:])
	if number == 0 {
		return common.Hash{}, fmt.Errorf("invalid LQC recovery V2 block number")
	}
	if stored, ok := rawdb.ReadHeaderNumber(db, target); ok && stored != number {
		return common.Hash{}, fmt.Errorf("LQC recovery V2 height mismatch")
	}
	// The mapping may disappear during rewinds. Preserve the recovery marker
	// for the existing recovery procedure rather than clearing it here.
	return target, nil
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

	number, ok := rawdb.ReadHeaderNumber(db, target)
	if !ok || number == 0 {
		return common.Hash{}, nil
	}

	metadata := make([]byte, common.HashLength+8)
	copy(metadata[:common.HashLength], target[:])
	binary.BigEndian.PutUint64(metadata[common.HashLength:], number)

	batch := db.NewBatch()
	defer batch.Reset()

	if err := batch.Put(lqcLocalRecoveryKeyV1, target[:]); err != nil {
		return common.Hash{}, err
	}
	if err := batch.Put(lqcLocalRecoveryMetadataKeyV2, metadata); err != nil {
		return common.Hash{}, err
	}
	if err := batch.Write(); err != nil {
		return common.Hash{}, err
	}
	return target, nil

}

// clearLQCLocalRecoveryV2 removes both recovery markers in one batch.
// Call only after validating the recovered canonical state.
func (bc *BlockChain) clearLQCLocalRecoveryV2() error {
	batch := bc.db.NewBatch()
	defer batch.Reset()

	if err := batch.Delete(lqcLocalRecoveryKeyV1); err != nil {
		return err
	}
	if err := batch.Delete(lqcLocalRecoveryMetadataKeyV2); err != nil {
		return err
	}
	return batch.Write()
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
		// Do not discard an unknown or unfinished recovery target.
		targetNumber, known := rawdb.ReadHeaderNumber(bc.db, target)
		head := bc.CurrentBlock()
		if !known || head == nil || head.Number.Uint64() <= targetNumber ||
			bc.GetCanonicalHash(targetNumber) != target ||
			bc.GetCanonicalHash(head.Number.Uint64()) != head.Hash() ||
			rawdb.ReadHeadBlockHash(bc.db) != head.Hash() ||
			!bc.HasState(head.Root) {
			return fmt.Errorf("LQC recovery block unavailable locally: %s", target)
		}
		if _, err := bc.State(); err != nil {
			return fmt.Errorf("LQC recovery state unavailable: %w", err)
		}
		log.Warn("Retiring superseded LQC recovery target",
			"target", target, "targetNumber", targetNumber,
			"head", head.Number, "headHash", head.Hash())
		return bc.clearLQCLocalRecoveryV2()
	}
	head := bc.CurrentBlock()
	if head.Number.Cmp(tip.Number()) >= 0 {
		if bc.GetCanonicalHash(tip.NumberU64()) != target || !bc.HasState(head.Root) {
			return fmt.Errorf("LQC recovery target conflicts with current stateful chain: %s", target)
		}
		return bc.clearLQCLocalRecoveryV2()
	}
	// Collect hashes first, so a missing body or a disconnected branch is
	// detected before execution. Bound memory and reject unexpected deep gaps.
	const maxBlocks = 4096
	var reverse []common.Hash
	for block := tip; ; {
		if bc.GetCanonicalHash(block.NumberU64()) == block.Hash() && bc.HasState(block.Root()) {
			break
		}
		if block.NumberU64() == 0 || len(reverse) == maxBlocks {
			return fmt.Errorf("LQC recovery has no available local ancestor within %d blocks", maxBlocks)
		}
		reverse = append(reverse, block.Hash())
		parent := bc.GetBlock(block.ParentHash(), block.NumberU64()-1)
		if parent == nil {
			return fmt.Errorf("LQC recovery parent unavailable: number=%d hash=%s", block.NumberU64()-1, block.ParentHash())
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
	if err := bc.clearLQCLocalRecoveryV2(); err != nil {
		return err
	}
	log.Info("LQC local state recovery completed", "number", tip.Number(), "hash", target, "root", tip.Root())
	return nil
}
