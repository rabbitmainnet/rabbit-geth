package core

import (
	"context"
	"errors"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// InsertLQCBlockIfParentCurrent atomically checks the producer's captured parent
// and imports its child. Remote blocks continue using the normal fork-choice
// path; only stale locally built payloads are discarded here.
func (bc *BlockChain) InsertLQCBlockIfParentCurrent(parent common.Hash, block *types.Block) (bool, error) {
	if block == nil {
		return false, nil
	}
	if !bc.chainmu.TryLock() {
		return false, errChainStopped
	}
	defer bc.chainmu.Unlock()
	if bc.chainConfig == nil || bc.chainConfig.LQC == nil {
		return false, errors.New("local LQC insertion requires LQC chain")
	}
	head := bc.CurrentBlock()
	if head == nil || head.Number == nil || head.Hash() != parent || block.ParentHash() != parent ||
		head.Number.BitLen() > 64 || head.Number.Uint64() == ^uint64(0) || block.NumberU64() != head.Number.Uint64()+1 {
		return false, nil
	}
	_, _, err := bc.insertChain(context.Background(), types.Blocks{block}, true, false)
	return err == nil, err
}
