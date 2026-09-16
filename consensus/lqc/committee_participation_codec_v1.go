package lqc

import (
	"errors"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

var (
	ErrInvalidCompactCommitteeParticipationV1 = errors.New("invalid compact lqc committee participation v1")
	ErrNonCanonicalCommitteeParticipationV1   = errors.New("non-canonical lqc committee participation order v1")
)

// CompactCommitteeParticipationV1 is the wire representation of one proof.
// The selected committee position deterministically supplies TicketHash and
// Participant; the common block context is stored once by the parent envelope.
type CompactCommitteeParticipationV1 struct {
	Position  uint8
	Signature []byte
}

func cloneCompactCommitteeParticipationV1(
	input CompactCommitteeParticipationV1,
) CompactCommitteeParticipationV1 {
	return CompactCommitteeParticipationV1{
		Position:  input.Position,
		Signature: append([]byte(nil), input.Signature...),
	}
}

// CompactCommitteeParticipationsV1 canonicalizes proofs by selected committee
// position and removes fields that can be reconstructed deterministically.
func CompactCommitteeParticipationsV1(
	selected []WorkSeatV1,
	proofs []CommitteeParticipationV1,
) ([]CompactCommitteeParticipationV1, error) {
	canonical, err := CanonicalizeCommitteeParticipationsV1(selected, proofs)
	if err != nil {
		return nil, err
	}
	positions := make(map[common.Address]uint8, len(selected))
	for index, seat := range selected {
		positions[seat.Participant] = uint8(index)
	}
	out := make([]CompactCommitteeParticipationV1, len(canonical))
	for index, proof := range canonical {
		if len(proof.Signature) != crypto.SignatureLength {
			return nil, ErrInvalidCompactCommitteeParticipationV1
		}
		out[index] = CompactCommitteeParticipationV1{
			Position:  positions[proof.Participant],
			Signature: append([]byte(nil), proof.Signature...),
		}
	}
	return out, nil
}

// ExpandCommitteeParticipationsV1 reconstructs the signed semantic proofs and
// rejects any non-canonical or out-of-range wire position before RandomX work.
func ExpandCommitteeParticipationsV1(
	blockNumber uint64,
	parentHash common.Hash,
	selectionRoot common.Hash,
	selected []WorkSeatV1,
	compact []CompactCommitteeParticipationV1,
) ([]CommitteeParticipationV1, error) {
	if blockNumber == 0 ||
		parentHash == (common.Hash{}) ||
		selectionRoot == (common.Hash{}) ||
		len(selected) > MaxCommitteeParticipationsV1 ||
		len(compact) > MaxCommitteeParticipationsV1 ||
		len(compact) > len(selected) {
		return nil, ErrInvalidCompactCommitteeParticipationV1
	}
	out := make([]CommitteeParticipationV1, len(compact))
	previous := -1
	for index, item := range compact {
		position := int(item.Position)
		if position >= len(selected) {
			return nil, ErrCommitteeParticipantNotSelectedV1
		}
		if position <= previous {
			return nil, ErrNonCanonicalCommitteeParticipationV1
		}
		if len(item.Signature) != crypto.SignatureLength {
			return nil, ErrInvalidCompactCommitteeParticipationV1
		}
		seat := selected[position]
		if seat.Participant == (common.Address{}) ||
			seat.TicketHash == (common.Hash{}) {
			return nil, ErrCommitteeParticipantNotSelectedV1
		}
		out[index] = CommitteeParticipationV1{
			Version:       CommitteeParticipationVersionV1,
			BlockNumber:   blockNumber,
			ParentHash:    parentHash,
			SelectionRoot: selectionRoot,
			TicketHash:    seat.TicketHash,
			Participant:   seat.Participant,
			Signature:     append([]byte(nil), item.Signature...),
		}
		previous = position
	}
	return out, nil
}
