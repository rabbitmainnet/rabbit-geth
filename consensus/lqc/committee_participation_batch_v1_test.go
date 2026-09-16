package lqc

import (
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

func committeeParticipationBatchFixtureV1(count int) ([]WorkSeatV1, []CommitteeParticipationV1) {
	selected := make([]WorkSeatV1, count)
	proofs := make([]CommitteeParticipationV1, count)
	parentHash := crypto.Keccak256Hash([]byte("batch-parent"))
	selectionRoot := crypto.Keccak256Hash([]byte("batch-selection"))
	for index := 0; index < count; index++ {
		participant := common.BigToAddress(big.NewInt(int64(index + 1)))
		ticket := common.BigToHash(big.NewInt(int64(index + 1)))
		selected[index] = WorkSeatV1{Participant: participant, TicketHash: ticket}
		proofs[index] = CommitteeParticipationV1{
			Version:       CommitteeParticipationVersionV1,
			BlockNumber:   100,
			ParentHash:    parentHash,
			SelectionRoot: selectionRoot,
			TicketHash:    ticket,
			Participant:   participant,
			Signature:     make([]byte, crypto.SignatureLength),
		}
	}
	return selected, proofs
}

func TestCanonicalizeCommitteeParticipationsV1(t *testing.T) {
	selected, proofs := committeeParticipationBatchFixtureV1(3)
	unordered := []CommitteeParticipationV1{proofs[2], proofs[0], proofs[1]}
	canonical, err := CanonicalizeCommitteeParticipationsV1(selected, unordered)
	if err != nil {
		t.Fatal(err)
	}
	for index := range canonical {
		if canonical[index].Participant != selected[index].Participant {
			t.Fatalf("canonical position %d has %s, want %s", index, canonical[index].Participant, selected[index].Participant)
		}
	}

	tests := []struct {
		name     string
		selected []WorkSeatV1
		proofs   []CommitteeParticipationV1
		want     error
	}{
		{name: "duplicate participant", selected: selected, proofs: []CommitteeParticipationV1{proofs[0], proofs[0]}, want: ErrDuplicateCommitteeParticipationV1},
		{name: "wrong ticket", selected: selected, proofs: []CommitteeParticipationV1{func() CommitteeParticipationV1 {
			changed := proofs[0]
			changed.TicketHash = proofs[1].TicketHash
			return changed
		}()}, want: ErrCommitteeParticipantNotSelectedV1},
		{name: "mixed block", selected: selected, proofs: []CommitteeParticipationV1{proofs[0], func() CommitteeParticipationV1 { changed := proofs[1]; changed.BlockNumber++; return changed }()}, want: ErrMixedCommitteeParticipationV1},
		{name: "too many selected", selected: func() []WorkSeatV1 {
			seats, _ := committeeParticipationBatchFixtureV1(MaxCommitteeParticipationsV1 + 1)
			return seats
		}(), proofs: nil, want: ErrTooManyCommitteeParticipationsV1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := CanonicalizeCommitteeParticipationsV1(test.selected, test.proofs)
			if !errors.Is(err, test.want) {
				t.Fatalf("error=%v want=%v", err, test.want)
			}
		})
	}
}
