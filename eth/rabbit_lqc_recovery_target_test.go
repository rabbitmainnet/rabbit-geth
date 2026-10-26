package eth

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

func TestLQCForkRecoveryRetiresSupersededTargetV1(t *testing.T) {
	base := &types.Header{Number: big.NewInt(52304), Extra: []byte("stalled-target")}
	preferred, losing := headersAroundHash(base)
	newer := &types.Header{Number: big.NewInt(52358), Extra: []byte("newer-pending")}
	behind := &types.Header{Number: big.NewInt(52303)}
	if !lqcForkRecoveryTargetNeededV1(behind, losing, common.Hash{}) {
		t.Fatal("behind node must still recover")
	}
	if lqcForkRecoveryTargetNeededV1(preferred, losing, preferred.Hash()) {
		t.Fatal("losing same-height target blocks newer recovery until timeout")
	}
	if !lqcForkRecoveryTargetNeededV1(losing, preferred, losing.Hash()) {
		t.Fatal("preferred same-height fork must remain eligible")
	}
	if lqcForkRecoveryTargetNeededV1(newer, losing, common.Hash{}) {
		t.Fatal("obsolete lower target blocks newer local head")
	}
	if lqcForkRecoveryTargetNeededV1(behind, losing, losing.Hash()) {
		t.Fatal("canonical target must complete")
	}
	if !lqcForkRecoveryTargetNeededV1(preferred, newer, common.Hash{}) {
		t.Fatal("newer target lost during retirement")
	}
	// Retiring the current target must preserve the existing pending selection.
	h := &ethHandler{lqcSyncRunning: true, lqcSyncCurrent: &lqcSyncTarget{header: losing}, lqcSyncPending: &lqcSyncTarget{header: newer}}
	if lqcForkRecoveryTargetNeededV1(preferred, h.lqcSyncCurrent.header, preferred.Hash()) {
		t.Fatal("current target not retired")
	}
	next := h.takePendingLQCSyncTarget()
	if next == nil || next.header.Hash() != newer.Hash() || !h.lqcSyncRunning {
		t.Fatal("pending newer target not selected")
	}
	if h.takePendingLQCSyncTarget() != nil || h.lqcSyncRunning {
		t.Fatal("empty recovery queue did not stop")
	}
}
