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
	Index int
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

	var scoreInput [64]byte
	copy(scoreInput[:32], selectionSeed[:])
	hasher := crypto.NewKeccakState()

	for index := range input {
		if input[index].TicketHash == (common.Hash{}) ||
			input[index].Participant == (common.Address{}) {
			return nil, ErrInvalidWorkSeat
		}
		if _, exists := seen[input[index].TicketHash]; exists {
			return nil, ErrDuplicateRandomXWorkHash
		}
		seen[input[index].TicketHash] = struct{}{}
		if _, exists := seenParticipants[input[index].Participant]; exists {
			return nil, ErrDuplicateWorkParticipantV1
		}
		seenParticipants[input[index].Participant] = struct{}{}

		copy(scoreInput[32:], input[index].TicketHash[:])

		var score common.Hash
		hasher.Reset()
		_, _ = hasher.Write(scoreInput[:])
		_, _ = hasher.Read(score[:])

		scored[index] = scoredWorkSeatV1{
			Index: index,
			Score: score,
		}
	}

	sort.Slice(scored, func(i, j int) bool {
		if order := bytes.Compare(
			scored[i].Score[:],
			scored[j].Score[:],
		); order != 0 {
			return order < 0
		}
		left := input[scored[i].Index]
		right := input[scored[j].Index]
		if order := bytes.Compare(
			left.TicketHash[:],
			right.TicketHash[:],
		); order != 0 {
			return order < 0
		}
		return left.Participant.Cmp(right.Participant) < 0
	})

	ordered := make([]WorkSeatV1, len(scored))
	for index := range scored {
		ordered[index] = input[scored[index].Index]
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
	return buildWorkSelectionFromOrderedSeatsV1(
		ordered,
		fallbackCount,
		committeeSize,
	), nil
}

// buildWorkSelectionFromOrderedSeatsV1 assigns every role from one canonical
// final queue. The caller is responsible for deterministic seat ordering.
func buildWorkSelectionFromOrderedSeatsV1(
	ordered []WorkSeatV1,
	fallbackCount uint64,
	committeeSize uint64,
) WorkSelectionV1 {
	selection := WorkSelectionV1{
		Ordered: append([]WorkSeatV1(nil), ordered...),
	}
	if len(selection.Ordered) == 0 {
		return selection
	}

	selection.Producer = &selection.Ordered[0]

	fallbackEnd := len(selection.Ordered)
	availableAfterProducer := uint64(len(selection.Ordered) - 1)
	if fallbackCount < availableAfterProducer {
		fallbackEnd = 1 + int(fallbackCount)
	}
	if fallbackEnd > 1 {
		selection.Fallbacks = append(
			selection.Fallbacks,
			selection.Ordered[1:fallbackEnd]...,
		)
	}

	committeeAvailable := len(selection.Ordered) - fallbackEnd
	committeeTake := committeeSize
	if committeeTake > uint64(committeeAvailable) {
		committeeTake = uint64(committeeAvailable)
	}
	committeeEnd := fallbackEnd + int(committeeTake)
	if committeeEnd > fallbackEnd {
		selection.Committee = append(
			selection.Committee,
			selection.Ordered[fallbackEnd:committeeEnd]...,
		)
	}
	return selection
}
