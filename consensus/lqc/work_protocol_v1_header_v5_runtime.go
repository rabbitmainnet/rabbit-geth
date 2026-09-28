package lqc

import "github.com/ethereum/go-ethereum/common"

// BuildLQCHeaderExtraV5WithCanonicalRuntimeV1 extends the deterministic V4
// runtime transition with canonical Rabbit VRF finalizations. Work and
// committee-claim state transitions remain identical to V4.
func BuildLQCHeaderExtraV5WithCanonicalRuntimeV1(
	ctx LQCHeaderV4RuntimeContextV1,
	registryOperations []RegistryOperation,
	workTickets []SignedRandomXWorkTicketV1,
	committeeClaims []CommitteeParticipationClaimGroupV1,
	vrfFinalizations []RabbitVRFFinalizationV1,
) ([]byte, common.Hash, common.Hash, error) {
	return buildLQCHeaderExtraV5WithCanonicalRuntimeV1(ctx, registryOperations, workTickets, committeeClaims, nil, vrfFinalizations)
}

func BuildLQCHeaderExtraV5WithCanonicalRuntimeAndKeysetCertificateV1(
	ctx LQCHeaderV4RuntimeContextV1,
	registryOperations []RegistryOperation,
	workTickets []SignedRandomXWorkTicketV1,
	committeeClaims []CommitteeParticipationClaimGroupV1,
	certificate RabbitVRFKeysetCertificateV1,
	vrfFinalizations []RabbitVRFFinalizationV1,
) ([]byte, common.Hash, common.Hash, error) {
	return buildLQCHeaderExtraV5WithCanonicalRuntimeV1(ctx, registryOperations, workTickets, committeeClaims, []RabbitVRFKeysetCertificateV1{certificate}, vrfFinalizations)
}

func buildLQCHeaderExtraV5WithCanonicalRuntimeV1(
	ctx LQCHeaderV4RuntimeContextV1,
	registryOperations []RegistryOperation,
	workTickets []SignedRandomXWorkTicketV1,
	committeeClaims []CommitteeParticipationClaimGroupV1,
	vrfKeysetCertificates []RabbitVRFKeysetCertificateV1,
	vrfFinalizations []RabbitVRFFinalizationV1,
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
	canonicalFinalizations, err := CanonicalRabbitVRFFinalizationsV1(
		vrfFinalizations,
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
	extra, err := encodeLQCHeaderExtraV5(
		ctx.Work.BlockNumber,
		ctx.Work.RegistryRoot,
		nextWork.StateRoot,
		claimRoot,
		registryOperations,
		canonicalTickets,
		canonicalClaims,
		vrfKeysetCertificates,
		canonicalFinalizations,
		MaxWorkTicketsPerBlockV1,
	)
	if err != nil {
		return nil, common.Hash{}, common.Hash{}, err
	}
	return extra, nextWork.StateRoot, claimRoot, nil
}
