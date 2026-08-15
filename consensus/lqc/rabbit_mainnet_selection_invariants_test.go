package lqc

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestRabbitMainnetDynamicCommitteeScale(t *testing.T) {
	tests := []struct {
		active uint64
		want   uint64
	}{
		{1, 32},
		{32, 32},
		{100, 32},
		{320, 32},
		{500, 50},
		{1000, 100},
		{1280, 128},
		{10000, 128},
		{100000, 128},
		{1000000, 128},
	}
	for _, tt := range tests {
		if got := ComputeCommitteeSizeWithBounds(tt.active, 32, 128); got != tt.want {
			t.Fatalf("active=%d committee=%d want=%d", tt.active, got, tt.want)
		}
	}
}

func TestRabbitMainnetWholeQueueCanFailOver(t *testing.T) {
	ordered := make([]HybridParticipant, 200)
	for i := range ordered {
		ordered[i] = HybridParticipant{
			Address: common.BigToAddress(big.NewInt(int64(i + 1))),
		}
	}
	sel := HybridSelection{
		Producer:  &ordered[0],
		Fallbacks: append([]HybridParticipant(nil), ordered[1:6]...),
		Committee: append([]HybridParticipant(nil), ordered[6:134]...),
		Ordered:   ordered,
	}

	wantPos := 199
	ok, gotPos := IsAuthorAllowed(sel, ordered[wantPos].Address)
	if !ok || gotPos != wantPos {
		t.Fatalf("whole-queue failover rejected: ok=%v pos=%d want=%d", ok, gotPos, wantPos)
	}
}
