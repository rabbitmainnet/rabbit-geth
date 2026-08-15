package lqc

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestCanonicalMissedTurnPenaltyKeepsEmergencyFallback(t *testing.T) {
	a := common.HexToAddress("0x1000000000000000000000000000000000000001")
	b := common.HexToAddress("0x2000000000000000000000000000000000000002")

	r := NewCanonicalRegistry()
	r.entries[a] = CanonicalParticipant{Address: a, Active: true}
	r.entries[b] = CanonicalParticipant{Address: b, Active: true}

	for block := uint64(1); block <= 3; block++ {
		if err := r.ApplyMissedTurn(a, block, 3, 256); err != nil {
			t.Fatal(err)
		}
	}

	pa, ok := r.Participant(a)
	if !ok {
		t.Fatal("participant A missing")
	}
	if pa.JailedUntil != 259 {
		t.Fatalf("JailedUntil=%d want=259", pa.JailedUntil)
	}
	if pa.MissedTurns != 0 {
		t.Fatalf("MissedTurns=%d want=0 after penalty", pa.MissedTurns)
	}

	ordered := r.OrderedParticipantsForBlock(
		common.HexToHash("0x1234"),
		4,
		0,
		64,
		16,
	)

	if len(ordered) != 2 {
		t.Fatalf("queue size=%d want=2", len(ordered))
	}
	if ordered[len(ordered)-1].Address != a {
		t.Fatalf("penalized participant is not emergency fallback: queue=%v", ordered)
	}

	if err := r.MarkProducerHeartbeat(a, 4); err != nil {
		t.Fatal(err)
	}

	pa, _ = r.Participant(a)
	if pa.JailedUntil != 0 || pa.MissedTurns != 0 || pa.LastHeartbeat != 4 {
		t.Fatalf("successful production did not recover participant: %+v", pa)
	}
}
