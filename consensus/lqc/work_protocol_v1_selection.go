package lqc

import (
	"bytes"
	"errors"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

var ErrInvalidWorkSelectionV1 = errors.New("invalid lqc work selection v1")

// WorkSelectionV1 is an inactive consensus-model structure.
//
// Every element is a unique participant's WORK SEAT. Repeated Participant
// addresses are invalid consensus input.
type WorkSelectionV1 struct {
	Ordered   []WorkSeatV1
	Producer  *WorkSeatV1
	Fallbacks []WorkSeatV1
	Committee []WorkSeatV1
}

type scoredWorkSeatV1 struct {
	Seat  WorkSeatV1
	Score common.Hash
}

// DeterministicallyOrderWorkSeatsV1 orders seats only by an already-derived
// canonical selection seed and ticket hash. parentHash is deliberately absent:
// a producer must not get free seed choices from mutable header variants.
// Participant address is deliberately absent from the score:
// identity count must not itself create selection weight.
//
// TicketHash is already unique in a verified canonical work set.
func DeterministicallyOrderWorkSeatsV1(
	input []WorkSeatV1,
	selectionSeed common.Hash,
) ([]WorkSeatV1, error) {
	if selectionSeed == (common.Hash{}) {
		return nil, ErrInvalidWorkSelectionV1
	}
	if len(input) == 0 {
		return nil, nil
	}

	scored := make([]scoredWorkSeatV1, len(input))
	seen := make(map[common.Hash]struct{}, len(input))
	seenParticipants := make(map[common.Address]struct{}, len(input))

	for index, seat := range input {
		if seat.TicketHash == (common.Hash{}) ||
			seat.Participant == (common.Address{}) {
			return nil, ErrInvalidWorkSeat
		}
		if _, exists := seen[seat.TicketHash]; exists {
			return nil, ErrDuplicateRandomXWorkHash
		}
		seen[seat.TicketHash] = struct{}{}
		if _, exists := seenParticipants[seat.Participant]; exists {
			return nil, ErrDuplicateWorkParticipantV1
		}
		seenParticipants[seat.Participant] = struct{}{}

		scored[index] = scoredWorkSeatV1{
			Seat: seat,
			Score: crypto.Keccak256Hash(
				selectionSeed.Bytes(),
				seat.TicketHash.Bytes(),
			),
		}
	}

	sort.Slice(scored, func(i, j int) bool {
		if order := bytes.Compare(
			scored[i].Score.Bytes(),
			scored[j].Score.Bytes(),
		); order != 0 {
			return order < 0
		}
		if order := bytes.Compare(
			scored[i].Seat.TicketHash.Bytes(),
			scored[j].Seat.TicketHash.Bytes(),
		); order != 0 {
			return order < 0
		}
		return scored[i].Seat.Participant.Cmp(
			scored[j].Seat.Participant,
		) < 0
	})

	ordered := make([]WorkSeatV1, len(scored))
	for index := range scored {
		ordered[index] = scored[index].Seat
	}
	return ordered, nil
}

// BuildWorkSelectionV1 assigns roles by WORK SEAT.
//
// committeeSize is supplied by the caller. This foundation intentionally does
// not redefine Rabbit's existing committee-size policy; future engine wiring
// can continue deriving that size from canonical active registry membership.
func BuildWorkSelectionV1(
	seats []WorkSeatV1,
	selectionSeed common.Hash,
	fallbackCount uint64,
	committeeSize uint64,
) (WorkSelectionV1, error) {
	ordered, err := DeterministicallyOrderWorkSeatsV1(
		seats,
		selectionSeed,
	)
	if err != nil {
		return WorkSelectionV1{}, err
	}

	selection := WorkSelectionV1{Ordered: ordered}
	if len(ordered) == 0 {
		return selection, nil
	}

	selection.Producer = &selection.Ordered[0]

	fallbackEnd := len(ordered)
	availableAfterProducer := uint64(len(ordered) - 1)
	if fallbackCount < availableAfterProducer {
		fallbackEnd = 1 + int(fallbackCount)
	}
	if fallbackEnd > 1 {
		selection.Fallbacks = append(
			selection.Fallbacks,
			ordered[1:fallbackEnd]...,
		)
	}

	committeeAvailable := len(ordered) - fallbackEnd
	committeeTake := committeeSize
	if committeeTake > uint64(committeeAvailable) {
		committeeTake = uint64(committeeAvailable)
	}
	committeeEnd := fallbackEnd + int(committeeTake)
	if committeeEnd > fallbackEnd {
		selection.Committee = append(
			selection.Committee,
			ordered[fallbackEnd:committeeEnd]...,
		)
	}

	return selection, nil
}
