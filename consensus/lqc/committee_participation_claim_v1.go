package lqc

import (
	"bytes"
	"errors"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

const CommitteeParticipationClaimWindowV1 uint64 = 8

var (
	ErrInvalidCommitteeParticipationClaimV1   = errors.New("invalid lqc committee participation claim v1")
	ErrExpiredCommitteeParticipationClaimV1   = errors.New("expired lqc committee participation claim v1")
	ErrDuplicateCommitteeParticipationClaimV1 = errors.New("duplicate lqc committee participation claim v1")
	ErrNonCanonicalCommitteeClaimsV1          = errors.New("non-canonical lqc committee participation claims v1")
)

// CommitteeParticipationClaimGroupV1 stores the target block once and keeps
// each participation compact. Claims may be included during a short window so
// one producer cannot permanently censor an online committee member.
type CommitteeParticipationClaimGroupV1 struct {
	TargetBlock    uint64
	Participations []CompactCommitteeParticipationV1
}

func cloneCommitteeParticipationClaimGroupV1(
	input CommitteeParticipationClaimGroupV1,
) CommitteeParticipationClaimGroupV1 {
	out := CommitteeParticipationClaimGroupV1{
		TargetBlock:    input.TargetBlock,
		Participations: make([]CompactCommitteeParticipationV1, len(input.Participations)),
	}
	for index := range input.Participations {
		out.Participations[index] = cloneCompactCommitteeParticipationV1(input.Participations[index])
	}
	return out
}

// CanonicalCommitteeParticipationClaimGroupsV1 validates the inclusion window,
// enforces the global 128-proof bound, rejects duplicate (block, position)
// claims, and returns target blocks and positions in deterministic order.
func CanonicalCommitteeParticipationClaimGroupsV1(
	inclusionBlock uint64,
	input []CommitteeParticipationClaimGroupV1,
) ([]CommitteeParticipationClaimGroupV1, error) {
	if inclusionBlock <= 1 {
		return nil, ErrInvalidCommitteeParticipationClaimV1
	}
	out := make([]CommitteeParticipationClaimGroupV1, len(input))
	total := 0
	seenTargets := make(map[uint64]struct{}, len(input))
	for groupIndex, group := range input {
		if group.TargetBlock == 0 ||
			group.TargetBlock >= inclusionBlock ||
			len(group.Participations) == 0 {
			return nil, ErrInvalidCommitteeParticipationClaimV1
		}
		if inclusionBlock-group.TargetBlock > CommitteeParticipationClaimWindowV1 {
			return nil, ErrExpiredCommitteeParticipationClaimV1
		}
		if _, exists := seenTargets[group.TargetBlock]; exists {
			return nil, ErrDuplicateCommitteeParticipationClaimV1
		}
		seenTargets[group.TargetBlock] = struct{}{}
		total += len(group.Participations)
		if total > MaxCommitteeParticipationsV1 {
			return nil, ErrTooManyCommitteeParticipationsV1
		}

		out[groupIndex] = cloneCommitteeParticipationClaimGroupV1(group)
		seenPositions := make(map[uint8]struct{}, len(group.Participations))
		for _, item := range group.Participations {
			if int(item.Position) >= MaxCommitteeParticipationsV1 ||
				len(item.Signature) != crypto.SignatureLength {
				return nil, ErrInvalidCommitteeParticipationClaimV1
			}
			if _, exists := seenPositions[item.Position]; exists {
				return nil, ErrDuplicateCommitteeParticipationClaimV1
			}
			seenPositions[item.Position] = struct{}{}
		}
		sort.Slice(out[groupIndex].Participations, func(i, j int) bool {
			return out[groupIndex].Participations[i].Position < out[groupIndex].Participations[j].Position
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].TargetBlock < out[j].TargetBlock
	})
	return out, nil
}

func committeeParticipationClaimGroupsEqualV1(
	left []CommitteeParticipationClaimGroupV1,
	right []CommitteeParticipationClaimGroupV1,
) bool {
	if len(left) != len(right) {
		return false
	}
	for groupIndex := range left {
		if left[groupIndex].TargetBlock != right[groupIndex].TargetBlock ||
			len(left[groupIndex].Participations) != len(right[groupIndex].Participations) {
			return false
		}
		for proofIndex := range left[groupIndex].Participations {
			leftProof := left[groupIndex].Participations[proofIndex]
			rightProof := right[groupIndex].Participations[proofIndex]
			if leftProof.Position != rightProof.Position ||
				!bytes.Equal(leftProof.Signature, rightProof.Signature) {
				return false
			}
		}
	}
	return true
}

func ValidateCanonicalCommitteeParticipationClaimGroupsV1(
	inclusionBlock uint64,
	input []CommitteeParticipationClaimGroupV1,
) error {
	canonical, err := CanonicalCommitteeParticipationClaimGroupsV1(inclusionBlock, input)
	if err != nil {
		return err
	}
	if !committeeParticipationClaimGroupsEqualV1(canonical, input) {
		return ErrNonCanonicalCommitteeClaimsV1
	}
	return nil
}

// ExpandCommitteeParticipationClaimGroupV1 reconstructs the semantic proof
// after the target block's committee and roots have been derived from history.
func ExpandCommitteeParticipationClaimGroupV1(
	parentHash common.Hash,
	selectionRoot common.Hash,
	selected []WorkSeatV1,
	group CommitteeParticipationClaimGroupV1,
) ([]CommitteeParticipationV1, error) {
	return ExpandCommitteeParticipationsV1(
		group.TargetBlock,
		parentHash,
		selectionRoot,
		selected,
		group.Participations,
	)
}
