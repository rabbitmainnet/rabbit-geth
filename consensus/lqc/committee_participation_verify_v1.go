package lqc

import (
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

var ErrInvalidCommitteeParticipationVerificationV1 = errors.New(
	"invalid lqc committee participation verification context v1",
)

type CommitteeParticipationHasherV1 func(
	datasetKey common.Hash,
	input []byte,
) (common.Hash, error)

type CommitteeParticipationVerificationContextV1 struct {
	ChainID       *big.Int
	DatasetKey    common.Hash
	ParentHash    common.Hash
	SelectionRoot common.Hash
	Committee     []WorkSeatV1
	Hasher        CommitteeParticipationHasherV1
}

type VerifiedCommitteeParticipationV1 struct {
	TargetBlock uint64
	Position    uint8
	Seat        WorkSeatV1
	ProofHash   common.Hash
}

// VerifyCommitteeParticipationClaimGroupV1 reconstructs every semantic proof,
// performs exactly one RandomX hash through the shared epoch-keyed hasher, and
// verifies that the selected WorkSeat owner signed the recomputed result.
func VerifyCommitteeParticipationClaimGroupV1(
	ctx CommitteeParticipationVerificationContextV1,
	group CommitteeParticipationClaimGroupV1,
) ([]VerifiedCommitteeParticipationV1, error) {
	if ctx.ChainID == nil || ctx.ChainID.Sign() <= 0 ||
		ctx.DatasetKey == (common.Hash{}) ||
		ctx.ParentHash == (common.Hash{}) ||
		ctx.SelectionRoot == (common.Hash{}) ||
		len(ctx.Committee) > MaxCommitteeParticipationsV1 ||
		ctx.Hasher == nil {
		return nil, ErrInvalidCommitteeParticipationVerificationV1
	}
	proofs, err := ExpandCommitteeParticipationClaimGroupV1(
		ctx.ParentHash,
		ctx.SelectionRoot,
		ctx.Committee,
		group,
	)
	if err != nil {
		return nil, err
	}
	verified := make([]VerifiedCommitteeParticipationV1, len(proofs))
	for index, proof := range proofs {
		input, err := CommitteeParticipationInputV1(ctx.ChainID, proof)
		if err != nil {
			return nil, err
		}
		proofHash, err := ctx.Hasher(ctx.DatasetKey, input)
		if err != nil {
			return nil, err
		}
		position := group.Participations[index].Position
		seat := ctx.Committee[position]
		if err := VerifyCommitteeParticipationV1(
			ctx.ChainID,
			seat,
			proof,
			proofHash,
		); err != nil {
			return nil, err
		}
		verified[index] = VerifiedCommitteeParticipationV1{
			TargetBlock: group.TargetBlock,
			Position:    position,
			Seat:        seat,
			ProofHash:   proofHash,
		}
	}
	return verified, nil
}
