package core

import (
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
)

// ErrLQCLocalRecoveryNeedsSyncV2 identifies missing local recovery data only.
// Invalid blocks and database failures must not be converted into this error.
var ErrLQCLocalRecoveryNeedsSyncV2 = errors.New("LQC local recovery requires network synchronization")

// LQCLocalRecoveryPendingV2 never changes the persistent recovery target.
func (bc *BlockChain) LQCLocalRecoveryPendingV2() (bool, error) {
	if bc.chainConfig.LQC == nil {
		return false, nil
	}
	target, err := ReadLQCLocalStateRecoveryV1(bc.db)
	return target != (common.Hash{}), err
}

// LQCLocalRecoveryReadyV2 permits normal validated network import while keeping
// local production blocked. Only a canonical recovered target with a readable
// current head state may retire the persistent guard. No head marker is written.
func (bc *BlockChain) LQCLocalRecoveryReadyV2() (bool, error) {
	pending, err := bc.LQCLocalRecoveryPendingV2()
	if err != nil || !pending {
		return !pending && err == nil, err
	}
	if !bc.chainmu.TryLock() {
		return false, nil
	}
	defer bc.chainmu.Unlock()
	target, err := ReadLQCLocalStateRecoveryV1(bc.db)
	if err != nil {
		return false, err
	}
	if target == (common.Hash{}) {
		return true, nil
	}
	tip := bc.GetBlockByHash(target)
	head := bc.CurrentBlock()
	if tip == nil || head == nil || head.Number == nil || head.Number.Cmp(tip.Number()) < 0 {
		return false, nil
	}
	if bc.GetCanonicalHash(tip.NumberU64()) != target {
		return false, fmt.Errorf("LQC recovery target is not canonical: %s", target)
	}
	if !bc.HasState(head.Root) {
		return false, nil
	}
	if err := bc.db.Delete(lqcLocalRecoveryKeyV1); err != nil {
		return false, err
	}
	return true, nil
}
