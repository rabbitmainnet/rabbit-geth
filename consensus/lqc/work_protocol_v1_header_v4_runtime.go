package lqc

import (
	"errors"

	"github.com/ethereum/go-ethereum/common"
)

var (
	ErrInvalidLQCHeaderRuntimeV4 = errors.New(
		"invalid lqc header v4 committee claim runtime",
	)
	ErrLQCHeaderCommitteeClaimRootMismatchV4 = errors.New(
		"lqc header v4 committee claim root mismatch",
	)
)

// CommitteeClaimVerificationContextResolverV1 reconstructs the immutable
// committee context of a claim's target block from canonical history.
type CommitteeClaimVerificationContextResolverV1 func(
	targetBlock uint64,
) (CommitteeParticipationVerificationContextV1, error)

// LQCHeaderV4RuntimeContextV1 extends the existing V3 Work transition without
// changing its state root. Committee claims have an independent parent state
// and root so the first V4 block and every reorg branch are deterministic.
type LQCHeaderV4RuntimeContextV1 struct {
	Work          LQCHeaderWorkRuntimeContextV1
	ParentClaims  *CommitteeClaimLedgerV1
	ResolveClaims CommitteeClaimVerificationContextResolverV1
}

func validateLQCHeaderV4RuntimeContextV1(
	ctx LQCHeaderV4RuntimeContextV1,
) error {
	if err := validateLQCHeaderWorkRuntimeContextV1(ctx.Work); err != nil {
		return err
	}
	if ctx.ParentClaims == nil {
		return ErrInvalidLQCHeaderRuntimeV4
	}
	return ctx.ParentClaims.Validate()
}

func verifyAndApplyCommitteeClaimsV1(
	ctx LQCHeaderV4RuntimeContextV1,
	groups []CommitteeParticipationClaimGroupV1,
) (*CommitteeClaimLedgerV1, []VerifiedCommitteeParticipationV1, error) {
	if err := validateLQCHeaderV4RuntimeContextV1(ctx); err != nil {
		return nil, nil, err
	}
	canonical, err := CanonicalCommitteeParticipationClaimGroupsV1(
		ctx.Work.BlockNumber,
		groups,
	)
	if err != nil {
		return nil, nil, err
	}
	if !committeeParticipationClaimGroupsEqualV1(canonical, groups) {
		return nil, nil, ErrNonCanonicalCommitteeClaimsV1
	}
	if len(canonical) > 0 && ctx.ResolveClaims == nil {
		return nil, nil, ErrInvalidLQCHeaderRuntimeV4
	}
	next, err := ctx.ParentClaims.Apply(ctx.Work.BlockNumber, canonical)
	if err != nil {
		return nil, nil, err
	}

	verified := make([]VerifiedCommitteeParticipationV1, 0)
	for _, group := range canonical {
		verification, err := ctx.ResolveClaims(group.TargetBlock)
		if err != nil {
			return nil, nil, err
		}
		if verification.ChainID == nil ||
			verification.ChainID.Cmp(ctx.Work.ChainID) != 0 {
			return nil, nil, ErrInvalidLQCHeaderRuntimeV4
		}
		items, err := VerifyCommitteeParticipationClaimGroupV1(
			verification,
			group,
		)
		if err != nil {
			return nil, nil, err
		}
		verified = append(verified, items...)
	}

	return next, verified, nil
}

// BuildLQCHeaderExtraV4WithCanonicalRuntimeV1 computes both independent
// post-state roots. It does not mutate either parent state.
func BuildLQCHeaderExtraV4WithCanonicalRuntimeV1(
	ctx LQCHeaderV4RuntimeContextV1,
	registryOperations []RegistryOperation,
	workTickets []SignedRandomXWorkTicketV1,
	committeeClaims []CommitteeParticipationClaimGroupV1,
) ([]byte, common.Hash, common.Hash, error) {
	if err := validateLQCHeaderV4RuntimeContextV1(ctx); err != nil {
		return nil, common.Hash{}, common.Hash{}, err
	}
	canonicalTickets, err := CanonicalWorkTicketsV3(
		workTickets,
		MaxWorkTicketsPerBlockV1,
	)
	if err != nil {
		return nil, common.Hash{}, common.Hash{}, err
	}
	canonicalClaims, err := CanonicalCommitteeParticipationClaimGroupsV1(
		ctx.Work.BlockNumber,
		committeeClaims,
	)
	if err != nil {
		return nil, common.Hash{}, common.Hash{}, err
	}
	nextWork, err := computeLQCHeaderPostWorkStateV1(
		ctx.Work,
		canonicalTickets,
		common.Hash{},
	)
	if err != nil {
		return nil, common.Hash{}, common.Hash{}, err
	}
	nextClaims, _, err := verifyAndApplyCommitteeClaimsV1(ctx, canonicalClaims)
	if err != nil {
		return nil, common.Hash{}, common.Hash{}, err
	}
	claimRoot, err := nextClaims.Root()
	if err != nil {
		return nil, common.Hash{}, common.Hash{}, err
	}
	extra, err := EncodeLQCHeaderExtraV4(
		ctx.Work.BlockNumber,
		ctx.Work.RegistryRoot,
		nextWork.StateRoot,
		claimRoot,
		registryOperations,
		canonicalTickets,
		canonicalClaims,
		MaxWorkTicketsPerBlockV1,
	)
	if err != nil {
		return nil, common.Hash{}, common.Hash{}, err
	}
	return extra, nextWork.StateRoot, claimRoot, nil
}

// ValidateAndApplyLQCHeaderExtraV4WithCanonicalRuntimeV1 recomputes Work and
// committee claim state, then links the Work state to the actual child hash.
func ValidateAndApplyLQCHeaderExtraV4WithCanonicalRuntimeV1(
	ctx LQCHeaderV4RuntimeContextV1,
	childHash common.Hash,
	extra []byte,
) (
	LQCHeaderEnvelopeV4,
	*CanonicalWorkRuntimeStateV1,
	*CommitteeClaimLedgerV1,
	[]VerifiedCommitteeParticipationV1,
	error,
) {
	if err := validateLQCHeaderV4RuntimeContextV1(ctx); err != nil {
		return LQCHeaderEnvelopeV4{}, nil, nil, nil, err
	}
	if childHash == (common.Hash{}) {
		return LQCHeaderEnvelopeV4{}, nil, nil, nil,
			ErrInvalidLQCHeaderRuntimeV4
	}
	envelope, err := ValidateLQCHeaderExtraV4(
		ctx.Work.BlockNumber,
		MaxWorkTicketsPerBlockV1,
		extra,
	)
	if err != nil {
		return LQCHeaderEnvelopeV4{}, nil, nil, nil, err
	}
	if envelope.RegistryRoot != ctx.Work.RegistryRoot {
		return LQCHeaderEnvelopeV4{}, nil, nil, nil,
			ErrLQCHeaderRegistryRootMismatchV3
	}
	expectedWork, err := computeLQCHeaderPostWorkStateV1(
		ctx.Work,
		envelope.WorkTickets,
		common.Hash{},
	)
	if err != nil {
		return LQCHeaderEnvelopeV4{}, nil, nil, nil, err
	}
	if envelope.WorkStateRoot != expectedWork.StateRoot {
		return LQCHeaderEnvelopeV4{}, nil, nil, nil,
			ErrLQCHeaderWorkStateRootMismatchV3
	}
	nextClaims, verified, err := verifyAndApplyCommitteeClaimsV1(
		ctx,
		envelope.CommitteeParticipationClaims,
	)
	if err != nil {
		return LQCHeaderEnvelopeV4{}, nil, nil, nil, err
	}
	expectedClaimRoot, err := nextClaims.Root()
	if err != nil {
		return LQCHeaderEnvelopeV4{}, nil, nil, nil, err
	}
	if envelope.CommitteeClaimRoot != expectedClaimRoot {
		return LQCHeaderEnvelopeV4{}, nil, nil, nil,
			ErrLQCHeaderCommitteeClaimRootMismatchV4
	}
	nextWork, err := computeLQCHeaderPostWorkStateV1(
		ctx.Work,
		envelope.WorkTickets,
		childHash,
	)
	if err != nil {
		return LQCHeaderEnvelopeV4{}, nil, nil, nil, err
	}
	if nextWork.StateRoot != expectedWork.StateRoot {
		return LQCHeaderEnvelopeV4{}, nil, nil, nil,
			ErrLQCHeaderWorkStateRootMismatchV3
	}
	return envelope, nextWork, nextClaims, verified, nil
}
