package eth

import (
	"bytes"
	"math/big"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/core/types"
)

// These cases exercise production fork-choice and recovery helpers without
// creating fake P2P peers or asserting that a real chain has converged.
func lqcLabHeader(number uint64, tag string) *types.Header {
	return &types.Header{Number: new(big.Int).SetUint64(number), Extra: []byte(tag)}
}

func TestLQCLabEqualHeightDeterministicConvergence(t *testing.T) {
	a := lqcLabHeader(408069, "fork-A")
	b := lqcLabHeader(408069, "fork-B")
	if a.Hash() == b.Hash() {
		t.Fatal("test blocks unexpectedly identical")
	}
	lower, higher := a, b
	ah, bh := a.Hash(), b.Hash()
	if bytes.Compare(ah[:], bh[:]) > 0 {
		lower, higher = b, a
	}
	if !shouldStartLQCSync(higher, lower.Number.Uint64(), lower.Hash()) {
		t.Fatal("node with higher hash did not select lower-hash competing tip")
	}
	if shouldStartLQCSync(lower, higher.Number.Uint64(), higher.Hash()) {
		t.Fatal("node with lower hash tried to replace its tip with higher hash")
	}
	if shouldStartLQCSync(lower, lower.Number.Uint64(), lower.Hash()) {
		t.Fatal("node attempts recovery for its own canonical head")
	}
	if !betterLQCSyncTarget(&lqcSyncTarget{header: lower}, &lqcSyncTarget{header: higher}) {
		t.Fatal("recovery queue prefers inferior same-height target")
	}
	if betterLQCSyncTarget(&lqcSyncTarget{header: higher}, &lqcSyncTarget{header: lower}) {
		t.Fatal("recovery queue replaced preferred target with inferior hash")
	}
	for i := 0; i < 100; i++ {
		if shouldStartLQCSync(lower, higher.Number.Uint64(), higher.Hash()) {
			t.Fatal("repeated inferior announcement triggered reorg")
		}
	}
}

func TestLQCLabHigherChainAndStaleAnnouncements(t *testing.T) {
	head := lqcLabHeader(408070, "local")
	newer := lqcLabHeader(408071, "newer")
	old := lqcLabHeader(408069, "old")
	if !shouldStartLQCSync(head, newer.Number.Uint64(), newer.Hash()) {
		t.Fatal("higher chain should trigger recovery even if its hash is large")
	}
	if shouldStartLQCSync(head, old.Number.Uint64(), old.Hash()) {
		t.Fatal("older announcement must not roll back current head")
	}
	if !betterLQCSyncTarget(&lqcSyncTarget{header: newer}, &lqcSyncTarget{header: head}) {
		t.Fatal("higher recovery target should supersede lower one")
	}
	if betterLQCSyncTarget(&lqcSyncTarget{header: old}, &lqcSyncTarget{header: head}) {
		t.Fatal("stale recovery target superseded newer one")
	}
}

func TestLQCLabLiveImportCompletesBeforeGrace(t *testing.T) {
	head := lqcLabHeader(408070, "local")
	target := lqcLabHeader(408071, "received")
	target.ParentHash = head.Hash()
	var current atomic.Pointer[types.Header]
	current.Store(head)
	quit := make(chan struct{})
	go func() {
		time.Sleep(25 * time.Millisecond)
		current.Store(target)
	}()
	if waitForLQCLiveImport(current.Load, target, quit, 300*time.Millisecond) {
		t.Fatal("recovery was scheduled even though live import reached announced tip")
	}
}

func TestLQCLabLiveImportRetiresRecovery(t *testing.T) {
	head := lqcLabHeader(408070, "local")
	target := lqcLabHeader(408071, "received")
	target.ParentHash = head.Hash()
	quit := make(chan struct{})
	go func() {
		time.Sleep(20 * time.Millisecond)
		close(quit)
	}()
	// The quit channel models the node stopping. Recovery must exit immediately.
	if waitForLQCLiveImport(func() *types.Header { return head }, target, quit, 250*time.Millisecond) {
		t.Fatal("recovery continued after cancellation")
	}
}

func TestLQCLabNearTipStallBudgets(t *testing.T) {
	cases := []struct {
		local, remote uint64
		want          time.Duration
	}{
		{408070, 408070, 30 * time.Second},
		{408070, 408071, 30 * time.Second},
		{408070, 408072, 30 * time.Second},
		{408070, 408073, 2 * time.Minute},
	}
	for _, tc := range cases {
		got := lqcRecoveryStallBudget(lqcLabHeader(tc.local, "local"), lqcLabHeader(tc.remote, "remote"))
		if got != tc.want {
			t.Fatalf("%d -> %d: budget %s, want %s", tc.local, tc.remote, got, tc.want)
		}
	}
}
