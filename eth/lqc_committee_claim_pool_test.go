package eth

import (
	"bytes"
	"testing"

	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/crypto"
)

func TestLQCCommitteeClaimPoolBoundsDeduplicatesAndPrunes(t *testing.T) {
	pool := newLQCCommitteeClaimPool()
	signature := bytes.Repeat([]byte{1}, crypto.SignatureLength)
	group := lqc.CommitteeParticipationClaimGroupV1{
		TargetBlock: 10,
		Participations: []lqc.CompactCommitteeParticipationV1{{
			Position: 3, Signature: signature,
		}},
	}
	changed, err := pool.add(group)
	if err != nil || !changed {
		t.Fatalf("first add: changed=%v err=%v", changed, err)
	}
	changed, err = pool.add(group)
	if err != nil || changed {
		t.Fatalf("duplicate add: changed=%v err=%v", changed, err)
	}
	if got := pool.pending(11, nil); len(got) != 1 || len(got[0].Participations) != 1 {
		t.Fatalf("pending=%+v", got)
	}
	if got := pool.pending(19, nil); len(got) != 0 {
		t.Fatalf("expired claims retained: %+v", got)
	}
}
