package lqc

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/holiman/uint256"
)

func committeeClaimRewardSeatsV1(count int) []WorkSeatV1 {
	seats := make([]WorkSeatV1, count)
	for index := range seats {
		seats[index] = WorkSeatV1{
			TicketHash:  common.BigToHash(big.NewInt(int64(index + 1))),
			Participant: common.BigToAddress(big.NewInt(int64(index + 1))),
		}
	}
	return seats
}

func TestCommitteeClaimRewardV1FixedSharesAndNonIssuance(t *testing.T) {
	committee := committeeClaimRewardSeatsV1(3)
	verified := []VerifiedCommitteeParticipationV1{
		{TargetBlock: 100, Position: 0, Seat: committee[0]},
		{TargetBlock: 100, Position: 2, Seat: committee[2]},
	}
	result, err := CommitteeClaimRewardCreditsV1(uint256.NewInt(1000), committee, verified)
	if err != nil {
		t.Fatal(err)
	}
	if result.CommitteePool.Uint64() != 300 ||
		result.CommitteeIssued.Uint64() != 200 ||
		result.CommitteeUnissued.Uint64() != 100 ||
		len(result.Credits) != 2 {
		t.Fatalf("unexpected partial committee reward: %+v", result)
	}
	for index, credit := range result.Credits {
		if credit.Address != verified[index].Seat.Participant || credit.Amount.Uint64() != 100 {
			t.Fatalf("credit %d=%+v", index, credit)
		}
	}

	none, err := CommitteeClaimRewardCreditsV1(uint256.NewInt(1000), committee, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(none.Credits) != 0 || none.CommitteeIssued.Uint64() != 0 ||
		none.CommitteeUnissued.Uint64() != 300 {
		t.Fatalf("offline committee reward was emitted: %+v", none)
	}

	all := []VerifiedCommitteeParticipationV1{
		{TargetBlock: 100, Position: 0, Seat: committee[0]},
		{TargetBlock: 100, Position: 1, Seat: committee[1]},
		{TargetBlock: 100, Position: 2, Seat: committee[2]},
	}
	rounded, err := CommitteeClaimRewardCreditsV1(uint256.NewInt(1001), committee, all)
	if err != nil {
		t.Fatal(err)
	}
	if rounded.CommitteePool.Uint64() != 301 ||
		rounded.CommitteeIssued.Uint64() != 300 ||
		rounded.CommitteeUnissued.Uint64() != 1 {
		t.Fatalf("integer remainder was redistributed: %+v", rounded)
	}
}

func TestCommitteeClaimRewardV1RejectsDuplicatePosition(t *testing.T) {
	committee := committeeClaimRewardSeatsV1(2)
	duplicate := []VerifiedCommitteeParticipationV1{
		{TargetBlock: 100, Position: 0, Seat: committee[0]},
		{TargetBlock: 100, Position: 0, Seat: committee[0]},
	}
	if _, err := CommitteeClaimRewardCreditsV1(
		uint256.NewInt(1000),
		committee,
		duplicate,
	); err != ErrInvalidCommitteeClaimRewardV1 {
		t.Fatalf("duplicate error=%v", err)
	}
}
