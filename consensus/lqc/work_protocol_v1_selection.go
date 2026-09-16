package lqc

import (
	"bytes"
	"container/heap"
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

type scoredWorkSeatHeapV1 struct {
	input []WorkSeatV1
	items []scoredWorkSeatV1
}

func (h scoredWorkSeatHeapV1) Len() int { return len(h.items) }

// Less is reversed so the worst retained seat is at the heap root.
func (h scoredWorkSeatHeapV1) Less(i, j int) bool {
	return compareScoredWorkSeatsV1(h.items[i], h.items[j], h.input) > 0
}

func (h scoredWorkSeatHeapV1) Swap(i, j int) {
	h.items[i], h.items[j] = h.items[j], h.items[i]
}

func (h *scoredWorkSeatHeapV1) Push(value any) {
	h.items = append(h.items, value.(scoredWorkSeatV1))
}

func (h *scoredWorkSeatHeapV1) Pop() any {
	last := len(h.items) - 1
	value := h.items[last]
	h.items = h.items[:last]
	return value
}

func compareScoredWorkSeatsV1(left, right scoredWorkSeatV1, input []WorkSeatV1) int {
	if order := bytes.Compare(left.Score[:], right.Score[:]); order != 0 {
		return order
	}
	leftSeat := input[left.Index]
	rightSeat := input[right.Index]
	if order := bytes.Compare(leftSeat.TicketHash[:], rightSeat.TicketHash[:]); order != 0 {
		return order
	}
	return leftSeat.Participant.Cmp(rightSeat.Participant)
}

// DeterministicallySelectWorkSeatsV1 returns the same prefix as
// DeterministicallyOrderWorkSeatsV1 without allocating or sorting a full
// million-seat queue. The input must be a canonical WorkChainSnapshotV1 seat
// set, whose uniqueness and ordering have already been consensus-validated.
func DeterministicallySelectWorkSeatsV1(
	input []WorkSeatV1,
	selectionSeed common.Hash,
	limit uint64,
) ([]WorkSeatV1, error) {
	return deterministicallySelectWorkSeatsV1(input, selectionSeed, limit, nil)
}

// deterministicallySelectWorkSeatsV1 optionally places preferred seats before
// non-preferred seats while preserving the canonical score order inside each
// group. With preferred == nil it is exactly the prefix of the full order.
func deterministicallySelectWorkSeatsV1(
	input []WorkSeatV1,
	selectionSeed common.Hash,
	limit uint64,
	preferred func(WorkSeatV1) bool,
) ([]WorkSeatV1, error) {
	if selectionSeed == (common.Hash{}) {
		return nil, ErrInvalidWorkSelectionV1
	}
	if len(input) == 0 || limit == 0 {
		return nil, nil
	}
	if limit > uint64(len(input)) {
		limit = uint64(len(input))
	}

	primary := &scoredWorkSeatHeapV1{
		input: input,
		items: make([]scoredWorkSeatV1, 0, int(limit)),
	}
	secondary := &scoredWorkSeatHeapV1{
		input: input,
		items: make([]scoredWorkSeatV1, 0, int(limit)),
	}
	var scoreInput [64]byte
	copy(scoreInput[:32], selectionSeed[:])
	hasher := crypto.NewKeccakState()

	for index := range input {
		seat := input[index]
		if seat.TicketHash == (common.Hash{}) || seat.Participant == (common.Address{}) {
			return nil, ErrInvalidWorkSeat
		}
		copy(scoreInput[32:], seat.TicketHash[:])
		var score common.Hash
		hasher.Reset()
		_, _ = hasher.Write(scoreInput[:])
		_, _ = hasher.Read(score[:])
		candidate := scoredWorkSeatV1{Index: index, Score: score}
		target := primary
		if preferred != nil && !preferred(seat) {
			target = secondary
		}
		if uint64(target.Len()) < limit {
			heap.Push(target, candidate)
		} else if compareScoredWorkSeatsV1(candidate, target.items[0], input) < 0 {
			target.items[0] = candidate
			heap.Fix(target, 0)
		}
	}

	order := func(items []scoredWorkSeatV1) {
		sort.Slice(items, func(i, j int) bool {
			return compareScoredWorkSeatsV1(items[i], items[j], input) < 0
		})
	}
	order(primary.items)
	order(secondary.items)
	result := make([]WorkSeatV1, 0, int(limit))
	for _, selected := range primary.items {
		result = append(result, input[selected.Index])
	}
	for _, selected := range secondary.items {
		if uint64(len(result)) == limit {
			break
		}
		result = append(result, input[selected.Index])
	}
	return result, nil
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
		return compareScoredWorkSeatsV1(scored[i], scored[j], input) < 0
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
