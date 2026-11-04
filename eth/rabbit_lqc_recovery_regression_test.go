package eth

import (
    "math/big"
    "testing"

    "github.com/ethereum/go-ethereum/core/types"
)

// Regression: an incompatible higher fork must remain available
// for later recovery without being confused with a descendant.
func TestLQCCompetingForkTargetPreservedV1(t *testing.T) {
    current := &lqcSyncTarget{
        header: &types.Header{Number: big.NewInt(368014)},
    }
    competing := &lqcSyncTarget{
        header: &types.Header{Number: big.NewInt(368036)},
    }

    h := &ethHandler{
        lqcSyncRunning: true,
        lqcSyncCurrent: current,
        lqcSyncPending: competing,
    }

    next := h.takeBetterPendingLQCSyncTargetV1(current)
    if next != competing {
        t.Fatal("competing fork target was lost")
    }

    h.restorePendingLQCSyncTargetV1(next)

    if h.lqcSyncPending != competing {
        t.Fatal("competing fork was not retained for recovery")
    }
    if !h.lqcSyncRunning || h.lqcSyncCurrent != current {
        t.Fatal("active recovery state changed unexpectedly")
    }
}
