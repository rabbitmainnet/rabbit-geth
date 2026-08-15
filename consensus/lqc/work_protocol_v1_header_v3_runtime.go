package lqc

import (
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

var (
	ErrInvalidLQCHeaderRuntimeV3 = errors.New(
		"invalid lqc header v3 canonical work runtime",
	)
	ErrLQCHeaderWorkStateRootMismatchV3 = errors.New(
		"lqc header v3 work state root mismatch",
	)
	ErrLQCHeaderRegistryRootMismatchV3 = errors.New(
		"lqc header v3 registry root mismatch",
	)
)

var workHeaderTransitionPlaceholderDomainV1 = []byte(
	"RABBIT-LQC-WORK-HEADER-TRANSITION-PLACEHOLDER-V1",
)

// Difficulty is deliberately absent. It comes only from Parent.Difficulty.
type LQCHeaderWorkRuntimeContextV1 struct {
	ChainID         *big.Int
	Parent          *CanonicalWorkRuntimeStateV1
	BlockNumber     uint64
	RegistryRoot    common.Hash
	DatasetAnchor   common.Hash
	ChallengeAnchor common.Hash
	Eligibility     WorkRelayEligibilityCheckV1
	Hasher          WorkRelayHasherV1
}

func validateLQCHeaderWorkRuntimeContextV1(
	ctx LQCHeaderWorkRuntimeContextV1,
) error {
	if ctx.ChainID == nil ||
		ctx.ChainID.Sign() <= 0 ||
		ctx.Parent == nil ||
		ctx.BlockNumber == 0 ||
		ctx.RegistryRoot == (common.Hash{}) ||
		ctx.Eligibility == nil ||
		ctx.Hasher == nil {
		return ErrInvalidLQCHeaderRuntimeV3
	}
	if err := ctx.Parent.Validate(ctx.ChainID); err != nil {
		return err
	}
	if ctx.Parent.Work.Number == ^uint64(0) ||
		ctx.BlockNumber != ctx.Parent.Work.Number+1 {
		return ErrInvalidLQCHeaderRuntimeV3
	}

	_, hasCommit, err := WorkCommitTargetEpochV1(
		ctx.BlockNumber,
		ctx.Parent.Work.EpochLength,
	)
	if err != nil {
		return err
	}
	if hasCommit {
		if ctx.DatasetAnchor == (common.Hash{}) ||
			ctx.ChallengeAnchor == (common.Hash{}) {
			return ErrInvalidLQCHeaderRuntimeV3
		}
	} else if ctx.DatasetAnchor != (common.Hash{}) ||
		ctx.ChallengeAnchor != (common.Hash{}) {
		return ErrInvalidLQCHeaderRuntimeV3
	}
	return nil
}

// Header V3 carries signed tickets but never trusts a claimed ProofHash.
// Every validator recomputes RandomX from canonical anchors and ticket fields.
func verifyLQCHeaderWorkTicketsV1(
	ctx LQCHeaderWorkRuntimeContextV1,
	workTickets []SignedRandomXWorkTicketV1,
) ([]VerifiedRandomXWorkTicketV1, error) {
	if err := validateLQCHeaderWorkRuntimeContextV1(ctx); err != nil {
		return nil, err
	}

	canonical, err := CanonicalWorkTicketsV3(
		workTickets,
		MaxWorkTicketsPerBlockV1,
	)
	if err != nil {
		return nil, err
	}
	if !signedWorkTicketsEqualV3(canonical, workTickets) {
		return nil, ErrNonCanonicalWorkTicketsV3
	}

	epoch, hasCommit, err := WorkCommitTargetEpochV1(
		ctx.BlockNumber,
		ctx.Parent.Work.EpochLength,
	)
	if err != nil {
		return nil, err
	}
	if !hasCommit {
		if len(canonical) != 0 {
			return nil, ErrUnexpectedWorkCommitV1
		}
		return nil, nil
	}

	difficulty, ok, err := ctx.Parent.CommitDifficultyV1(
		ctx.ChainID,
		ctx.BlockNumber,
	)
	if err != nil {
		return nil, err
	}
	if !ok || difficulty == nil || difficulty.Sign() <= 0 {
		return nil, ErrInvalidLQCHeaderRuntimeV3
	}

	epochKey, err := RandomXWorkDatasetKeyV1(
		ctx.ChainID,
		epoch,
		ctx.DatasetAnchor,
	)
	if err != nil {
		return nil, err
	}

	verified := make(
		[]VerifiedRandomXWorkTicketV1,
		0,
		len(canonical),
	)

	for _, signed := range canonical {
		if signed.Ticket.Epoch != epoch {
			return nil, ErrWorkCommitEpochMismatchV1
		}
		if err := ctx.Eligibility(
			signed.Ticket.Participant,
		); err != nil {
			return nil, err
		}

		input, err := RandomXWorkChallengeInputV1(
			ctx.ChainID,
			ctx.ChallengeAnchor,
			signed.Ticket,
		)
		if err != nil {
			return nil, err
		}
		proofHash, err := ctx.Hasher(epochKey, input)
		if err != nil {
			return nil, err
		}

		item, err := ValidateRecomputedRandomXWorkV1(
			ctx.ChainID,
			ctx.ChallengeAnchor,
			difficulty,
			signed,
			proofHash,
		)
		if err != nil {
			return nil, err
		}
		verified = append(verified, item)
	}
	return verified, nil
}

func workHeaderTransitionPlaceholderHashV1(
	parentHash common.Hash,
	blockNumber uint64,
) common.Hash {
	return crypto.Keccak256Hash(
		workHeaderTransitionPlaceholderDomainV1,
		parentHash.Bytes(),
		new(big.Int).SetUint64(blockNumber).Bytes(),
	)
}

func computeLQCHeaderPostWorkStateV1(
	ctx LQCHeaderWorkRuntimeContextV1,
	workTickets []SignedRandomXWorkTicketV1,
	childHash common.Hash,
) (*CanonicalWorkRuntimeStateV1, error) {
	verified, err := verifyLQCHeaderWorkTicketsV1(
		ctx,
		workTickets,
	)
	if err != nil {
		return nil, err
	}

	_, hasCommit, err := WorkCommitTargetEpochV1(
		ctx.BlockNumber,
		ctx.Parent.Work.EpochLength,
	)
	if err != nil {
		return nil, err
	}

	challengeAnchor := common.Hash{}
	if hasCommit {
		challengeAnchor = ctx.ChallengeAnchor
	}
	if childHash == (common.Hash{}) {
		childHash = workHeaderTransitionPlaceholderHashV1(
			ctx.Parent.Work.Hash,
			ctx.BlockNumber,
		)
	}

	return ctx.Parent.ApplyVerifiedBlockV1(
		ctx.ChainID,
		ctx.BlockNumber,
		childHash,
		ctx.Parent.Work.Hash,
		challengeAnchor,
		verified,
	)
}

// Builds Header V3 with a canonical POST-state root. The protocol cap is fixed
// to MaxWorkTicketsPerBlockV1; callers cannot enlarge it.
func BuildLQCHeaderExtraV3WithCanonicalWorkV1(
	ctx LQCHeaderWorkRuntimeContextV1,
	registryOperations []RegistryOperation,
	workTickets []SignedRandomXWorkTicketV1,
) ([]byte, common.Hash, error) {
	if err := validateLQCHeaderWorkRuntimeContextV1(ctx); err != nil {
		return nil, common.Hash{}, err
	}

	canonicalTickets, err := CanonicalWorkTicketsV3(
		workTickets,
		MaxWorkTicketsPerBlockV1,
	)
	if err != nil {
		return nil, common.Hash{}, err
	}

	next, err := computeLQCHeaderPostWorkStateV1(
		ctx,
		canonicalTickets,
		common.Hash{},
	)
	if err != nil {
		return nil, common.Hash{}, err
	}

	extra, err := EncodeLQCHeaderExtraV3(
		ctx.BlockNumber,
		ctx.RegistryRoot,
		next.StateRoot,
		registryOperations,
		canonicalTickets,
		MaxWorkTicketsPerBlockV1,
	)
	if err != nil {
		return nil, common.Hash{}, err
	}
	return extra, next.StateRoot, nil
}

// Validates Header V3, recomputes RandomX, recomputes the post-work root and
// returns the persistent child runtime linked to the actual child block hash.
func ValidateAndApplyLQCHeaderExtraV3WithCanonicalWorkV1(
	ctx LQCHeaderWorkRuntimeContextV1,
	childHash common.Hash,
	extra []byte,
) (
	LQCHeaderEnvelopeV3,
	*CanonicalWorkRuntimeStateV1,
	error,
) {
	if err := validateLQCHeaderWorkRuntimeContextV1(ctx); err != nil {
		return LQCHeaderEnvelopeV3{}, nil, err
	}
	if childHash == (common.Hash{}) {
		return LQCHeaderEnvelopeV3{}, nil,
			ErrInvalidLQCHeaderRuntimeV3
	}

	envelope, err := ValidateLQCHeaderExtraV3(
		ctx.BlockNumber,
		MaxWorkTicketsPerBlockV1,
		extra,
	)
	if err != nil {
		return LQCHeaderEnvelopeV3{}, nil, err
	}
	if envelope.RegistryRoot != ctx.RegistryRoot {
		return LQCHeaderEnvelopeV3{}, nil,
			ErrLQCHeaderRegistryRootMismatchV3
	}

	expected, err := computeLQCHeaderPostWorkStateV1(
		ctx,
		envelope.WorkTickets,
		common.Hash{},
	)
	if err != nil {
		return LQCHeaderEnvelopeV3{}, nil, err
	}
	if envelope.WorkStateRoot != expected.StateRoot {
		return LQCHeaderEnvelopeV3{}, nil,
			ErrLQCHeaderWorkStateRootMismatchV3
	}

	next, err := computeLQCHeaderPostWorkStateV1(
		ctx,
		envelope.WorkTickets,
		childHash,
	)
	if err != nil {
		return LQCHeaderEnvelopeV3{}, nil, err
	}
	if next.StateRoot != expected.StateRoot {
		return LQCHeaderEnvelopeV3{}, nil,
			ErrLQCHeaderWorkStateRootMismatchV3
	}
	return envelope, next, nil
}
