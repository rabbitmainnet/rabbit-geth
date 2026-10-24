package eth

import (
	"github.com/ethereum/go-ethereum/core/types"
	"math/big"
	"testing"
)

func TestLQCRecoveryQueuedTargetAdvancesWithoutDeadlineV1(t *testing.T) {
	current := &lqcSyncTarget{header: &types.Header{Number: big.NewInt(55481)}}
	newer := &lqcSyncTarget{header: &types.Header{Number: big.NewInt(55524)}}
	newest := &lqcSyncTarget{header: &types.Header{Number: big.NewInt(55529)}}
	h := &ethHandler{lqcSyncRunning: true, lqcSyncCurrent: current, lqcSyncPending: newer}
	next := h.takeBetterPendingLQCSyncTargetV1(current)
	if next != newer || !h.lqcSyncRunning || h.lqcSyncCurrent != current {
		t.Fatal("queued improvement lost or worker stopped")
	}
	// While BeaconExtend is executing, a later announce can queue a newer head.
	h.lqcSyncPending = newest
	h.restorePendingLQCSyncTargetV1(next)
	if h.lqcSyncPending != newest {
		t.Fatal("failed extension overwrote a newer queued head")
	}
	if got := h.takePendingLQCSyncTarget(); got != newest || !h.lqcSyncRunning {
		t.Fatal("forced recovery did not take newest target")
	}
	h.lqcSyncPending = current
	if h.takeBetterPendingLQCSyncTargetV1(newest) != nil {
		t.Fatal("older target preempted newer recovery")
	}
	h.lqcSyncPending = nil
	h.restorePendingLQCSyncTargetV1(newer)
	if h.takePendingLQCSyncTarget() != newer {
		t.Fatal("failed extension dropped its target")
	}
}
