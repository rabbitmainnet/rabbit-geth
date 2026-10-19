package lqc

import (
	"errors"

	"github.com/ethereum/go-ethereum/common"
	"github.com/holiman/uint256"
)

const (
	CommitteeClaimProducerBpsV1  uint64 = 7000
	CommitteeClaimCommitteeBpsV1 uint64 = 3000
)

var ErrInvalidCommitteeClaimRewardV1 = errors.New(
	"invalid lqc committee claim reward v1",
)

type CommitteeClaimRewardResultV1 struct {
	Credits           []WorkSeatRewardV1
	CommitteePool     *uint256.Int
	CommitteeIssued   *uint256.Int
	CommitteeUnissued *uint256.Int
}

// CommitteeClaimRewardCreditsV1 gives every originally selected committee
// position the same fixed share. Missing proofs and integer remainder are not
// emitted and are never reassigned to the producer or another committee seat.
func CommitteeClaimRewardCreditsV1(
	totalBlockReward *uint256.Int,
	committee []WorkSeatV1,
	verified []VerifiedCommitteeParticipationV1,
) (CommitteeClaimRewardResultV1, error) {
	zero := func() CommitteeClaimRewardResultV1 {
		return CommitteeClaimRewardResultV1{
			Credits:           []WorkSeatRewardV1{},
			CommitteePool:     uint256.NewInt(0),
			CommitteeIssued:   uint256.NewInt(0),
			CommitteeUnissued: uint256.NewInt(0),
		}
	}
	if totalBlockReward == nil || len(committee) > MaxCommitteeParticipationsV1 {
		return zero(), ErrInvalidCommitteeClaimRewardV1
	}
	result := zero()
	if len(committee) == 0 {
		if len(verified) != 0 {
			return result, ErrInvalidCommitteeClaimRewardV1
		}
		return result, nil
	}
	if totalBlockReward.IsZero() {
		return result, nil
	}

	seenParticipants := make(map[common.Address]struct{}, len(committee))
	seenTickets := make(map[common.Hash]struct{}, len(committee))
	for _, seat := range committee {
		if seat.Participant == (common.Address{}) || seat.TicketHash == (common.Hash{}) {
			return zero(), ErrInvalidCommitteeClaimRewardV1
		}
		if _, exists := seenParticipants[seat.Participant]; exists {
			return zero(), ErrInvalidCommitteeClaimRewardV1
		}
		if _, exists := seenTickets[seat.TicketHash]; exists {
			return zero(), ErrInvalidCommitteeClaimRewardV1
		}
		seenParticipants[seat.Participant] = struct{}{}
		seenTickets[seat.TicketHash] = struct{}{}
	}

	producerReward := new(uint256.Int).Set(totalBlockReward)
	producerReward.Mul(producerReward, uint256.NewInt(CommitteeClaimProducerBpsV1))
	producerReward.Div(producerReward, uint256.NewInt(10000))
	result.CommitteePool.Set(totalBlockReward)
	result.CommitteePool.Sub(result.CommitteePool, producerReward)
	perSeat := new(uint256.Int).Set(result.CommitteePool)
	perSeat.Div(perSeat, uint256.NewInt(uint64(len(committee))))

	previousPosition := -1
	var targetBlock uint64
	for _, item := range verified {
		position := int(item.Position)
		if item.TargetBlock == 0 ||
			(targetBlock != 0 && item.TargetBlock != targetBlock) ||
			position >= len(committee) || position <= previousPosition ||
			item.Seat != committee[position] {
			return zero(), ErrInvalidCommitteeClaimRewardV1
		}
		if !perSeat.IsZero() {
			amount := new(uint256.Int).Set(perSeat)
			result.Credits = append(result.Credits, WorkSeatRewardV1{
				Address: committee[position].Participant,
				Amount:  amount,
			})
			result.CommitteeIssued.Add(result.CommitteeIssued, amount)
		}
		targetBlock = item.TargetBlock
		previousPosition = position
	}
	result.CommitteeUnissued.Set(result.CommitteePool)
	result.CommitteeUnissued.Sub(result.CommitteeUnissued, result.CommitteeIssued)
	return result, nil
}
