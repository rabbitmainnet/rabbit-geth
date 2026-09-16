package lqc

import (
	"errors"
	"sort"

	"github.com/ethereum/go-ethereum/common"
)

const MaxCommitteeParticipationsV1 = 128

var (
	ErrTooManyCommitteeParticipationsV1  = errors.New("too many lqc committee participations v1")
	ErrDuplicateCommitteeParticipationV1 = errors.New("duplicate lqc committee participation v1")
	ErrMixedCommitteeParticipationV1     = errors.New("mixed lqc committee participation context v1")
)

// CanonicalizeCommitteeParticipationsV1 validates the bounded committee batch
// and returns a copy ordered by the selected committee position. Cryptographic
// verification remains the responsibility of VerifyCommitteeParticipationV1.
func CanonicalizeCommitteeParticipationsV1(
	selected []WorkSeatV1,
	proofs []CommitteeParticipationV1,
) ([]CommitteeParticipationV1, error) {
	if len(selected) > MaxCommitteeParticipationsV1 ||
		len(proofs) > MaxCommitteeParticipationsV1 ||
		len(proofs) > len(selected) {
		return nil, ErrTooManyCommitteeParticipationsV1
	}
	if len(proofs) == 0 {
		return []CommitteeParticipationV1{}, nil
	}

	selectedByParticipant := make(map[common.Address]int, len(selected))
	selectedTickets := make(map[common.Hash]struct{}, len(selected))
	for index, seat := range selected {
		if seat.Participant == (common.Address{}) ||
			seat.TicketHash == (common.Hash{}) {
			return nil, ErrCommitteeParticipantNotSelectedV1
		}
		if _, exists := selectedByParticipant[seat.Participant]; exists {
			return nil, ErrDuplicateCommitteeParticipationV1
		}
		if _, exists := selectedTickets[seat.TicketHash]; exists {
			return nil, ErrDuplicateCommitteeParticipationV1
		}
		selectedByParticipant[seat.Participant] = index
		selectedTickets[seat.TicketHash] = struct{}{}
	}

	type indexedProof struct {
		position int
		proof    CommitteeParticipationV1
	}
	indexed := make([]indexedProof, 0, len(proofs))
	seenParticipants := make(map[common.Address]struct{}, len(proofs))
	seenTickets := make(map[common.Hash]struct{}, len(proofs))
	context := proofs[0]

	for _, proof := range proofs {
		if proof.Version != context.Version ||
			proof.BlockNumber != context.BlockNumber ||
			proof.ParentHash != context.ParentHash ||
			proof.SelectionRoot != context.SelectionRoot {
			return nil, ErrMixedCommitteeParticipationV1
		}
		position, exists := selectedByParticipant[proof.Participant]
		if !exists || selected[position].TicketHash != proof.TicketHash {
			return nil, ErrCommitteeParticipantNotSelectedV1
		}
		if _, exists := seenParticipants[proof.Participant]; exists {
			return nil, ErrDuplicateCommitteeParticipationV1
		}
		if _, exists := seenTickets[proof.TicketHash]; exists {
			return nil, ErrDuplicateCommitteeParticipationV1
		}
		seenParticipants[proof.Participant] = struct{}{}
		seenTickets[proof.TicketHash] = struct{}{}
		indexed = append(indexed, indexedProof{position: position, proof: proof})
	}

	sort.Slice(indexed, func(i, j int) bool {
		return indexed[i].position < indexed[j].position
	})
	canonical := make([]CommitteeParticipationV1, len(indexed))
	for index := range indexed {
		canonical[index] = indexed[index].proof
	}
	return canonical, nil
}
