package lqc

import "github.com/ethereum/go-ethereum/common"

// ValidateAndApplyLQCHeaderExtraV5WithCanonicalRuntimeV1 extends the V4
// deterministic Work and committee-claim transition with canonical Rabbit VRF
// finalizations committed by the V5 header envelope.
func ValidateAndApplyLQCHeaderExtraV5WithCanonicalRuntimeV1(
	ctx LQCHeaderV4RuntimeContextV1,
	childHash common.Hash,
	extra []byte,
) (
	LQCHeaderEnvelopeV5,
	*CanonicalWorkRuntimeStateV1,
	*CommitteeClaimLedgerV1,
	[]VerifiedCommitteeParticipationV1,
	error,
) {
	if err := validateLQCHeaderV4RuntimeContextV1(ctx); err != nil {
		return LQCHeaderEnvelopeV5{}, nil, nil, nil, err
	}
	if childHash == (common.Hash{}) {
		return LQCHeaderEnvelopeV5{}, nil, nil, nil, ErrInvalidLQCHeaderRuntimeV4
	}
	envelope, err := ValidateLQCHeaderExtraV5(
		ctx.Work.BlockNumber,
		MaxWorkTicketsPerBlockV1,
		extra,
	)
	if err != nil {
		return LQCHeaderEnvelopeV5{}, nil, nil, nil, err
	}
	if envelope.RegistryRoot != ctx.Work.RegistryRoot {
		return LQCHeaderEnvelopeV5{}, nil, nil, nil, ErrLQCHeaderRegistryRootMismatchV3
	}
	expectedWork, err := computeLQCHeaderPostWorkStateV1(
		ctx.Work,
		envelope.WorkTickets,
		common.Hash{},
	)
	if err != nil {
		return LQCHeaderEnvelopeV5{}, nil, nil, nil, err
	}
	if envelope.WorkStateRoot != expectedWork.StateRoot {
		return LQCHeaderEnvelopeV5{}, nil, nil, nil, ErrLQCHeaderWorkStateRootMismatchV3
	}
	nextClaims, verified, err := verifyAndApplyCommitteeClaimsV1(
		ctx,
		envelope.CommitteeParticipationClaims,
	)
	if err != nil {
		return LQCHeaderEnvelopeV5{}, nil, nil, nil, err
	}
	expectedClaimRoot, err := nextClaims.Root()
	if err != nil {
		return LQCHeaderEnvelopeV5{}, nil, nil, nil, err
	}
	if envelope.CommitteeClaimRoot != expectedClaimRoot {
		return LQCHeaderEnvelopeV5{}, nil, nil, nil, ErrLQCHeaderCommitteeClaimRootMismatchV4
	}
	nextWork, err := computeLQCHeaderPostWorkStateV1(
		ctx.Work,
		envelope.WorkTickets,
		childHash,
	)
	if err != nil {
		return LQCHeaderEnvelopeV5{}, nil, nil, nil, err
	}
	if nextWork.StateRoot != expectedWork.StateRoot {
		return LQCHeaderEnvelopeV5{}, nil, nil, nil, ErrLQCHeaderWorkStateRootMismatchV3
	}
	return envelope, nextWork, nextClaims, verified, nil
}
