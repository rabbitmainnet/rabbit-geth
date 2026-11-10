package eth

import (
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

func TestLQCResumeStabilityGuard(t *testing.T) {
	var g lqcResumeStabilityGuard
	a := common.HexToHash("0x01")
	b := common.HexToHash("0x02")
	start := time.Unix(1000, 0)
	if g.allow(a, start) || g.allow(a, start.Add(900*time.Millisecond)) {
		t.Fatal("early same-head restart")
	}
	if !g.allow(a, start.Add(time.Second)) {
		t.Fatal("stable head did not resume")
	}
	if g.allow(b, start.Add(2*time.Second)) {
		t.Fatal("different head reused old confirmation")
	}
	g.reset()
	if g.allow(b, start.Add(3*time.Second)) {
		t.Fatal("restart after reset without stability")
	}
	if !g.allow(b, start.Add(4*time.Second)) {
		t.Fatal("head did not recover after fresh interval")
	}
	if g.allow(common.Hash{}, start.Add(5*time.Second)) {
		t.Fatal("zero head allowed")
	}
}
