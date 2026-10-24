//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

func TestRabbitVRFDKGEvaluationDispatchRetryAndSessionIsolationV1(t *testing.T) {
	sessionA := common.HexToHash("0xa1")
	sessionB := common.HexToHash("0xb2")
	runtime := &rabbitVRFDKGRuntime{}
	runtime.current.SessionID = sessionA
	if !runtime.shouldDispatchEvaluationV1(sessionA, 1, 2) {
		t.Fatal("first send blocked")
	}
	runtime.markDispatchedEvaluationV1(sessionA, 1, 2)
	if runtime.shouldDispatchEvaluationV1(sessionA, 1, 2) {
		t.Fatal("duplicate send not throttled")
	}
	if !runtime.shouldDispatchEvaluationV1(sessionA, 2, 1) {
		t.Fatal("other pair blocked")
	}
	runtime.mu.Lock()
	runtime.evalLastSent[[2]uint64{1, 2}] = time.Now().Add(-11 * time.Second)
	runtime.mu.Unlock()
	if !runtime.shouldDispatchEvaluationV1(sessionA, 1, 2) {
		t.Fatal("retry after deadline blocked")
	}
	if !runtime.markDKGFinalizedV1(sessionA) {
		t.Fatal("first finalize blocked")
	}
	if runtime.markDKGFinalizedV1(sessionA) {
		t.Fatal("duplicate finalize allowed")
	}
	runtime.mu.Lock()
	runtime.current.SessionID = sessionB
	runtime.mu.Unlock()
	if !runtime.shouldDispatchEvaluationV1(sessionB, 1, 2) {
		t.Fatal("new session inherited old throttle")
	}
	if !runtime.markDKGFinalizedV1(sessionB) {
		t.Fatal("new session finalization blocked")
	}
	if runtime.shouldDispatchEvaluationV1(sessionA, 1, 2) {
		t.Fatal("stale session accepted")
	}
}
