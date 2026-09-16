package lqc

import (
	"errors"
	"fmt"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

type headerV4ClaimFixtureV1 struct {
	resolver CommitteeClaimVerificationContextResolverV1
	groups   []CommitteeParticipationClaimGroupV1
}

func headerV4ClaimFixtureForPositionsV1(
	t *testing.T,
	chainID *big.Int,
	targetBlock uint64,
	positions ...uint8,
) headerV4ClaimFixtureV1 {
	t.Helper()
	datasetKey := crypto.Keccak256Hash([]byte("v4-claim-dataset"))
	parentHash := crypto.Keccak256Hash([]byte("v4-claim-parent"))
	selectionRoot := crypto.Keccak256Hash([]byte("v4-claim-selection"))
	committee := make([]WorkSeatV1, 3)
	privateKeys := make([]*big.Int, len(committee))
	for index := range committee {
		privateKeys[index] = big.NewInt(int64(index + 101))
		key, err := crypto.HexToECDSA(fmt.Sprintf("%064x", privateKeys[index]))
		if err != nil {
			t.Fatal(err)
		}
		committee[index] = WorkSeatV1{
			TicketHash:  crypto.Keccak256Hash([]byte{byte(index + 1)}),
			Participant: crypto.PubkeyToAddress(key.PublicKey),
		}
	}
	hasher := func(key common.Hash, input []byte) (common.Hash, error) {
		return crypto.Keccak256Hash(key.Bytes(), input), nil
	}
	group := CommitteeParticipationClaimGroupV1{
		TargetBlock:    targetBlock,
		Participations: make([]CompactCommitteeParticipationV1, len(positions)),
	}
	for index, position := range positions {
		key, err := crypto.HexToECDSA(fmt.Sprintf("%064x", privateKeys[position]))
		if err != nil {
			t.Fatal(err)
		}
		seat := committee[position]
		proof := CommitteeParticipationV1{
			Version:       CommitteeParticipationVersionV1,
			BlockNumber:   targetBlock,
			ParentHash:    parentHash,
			SelectionRoot: selectionRoot,
			TicketHash:    seat.TicketHash,
			Participant:   seat.Participant,
		}
		input, err := CommitteeParticipationInputV1(chainID, proof)
		if err != nil {
			t.Fatal(err)
		}
		proofHash, err := hasher(datasetKey, input)
		if err != nil {
			t.Fatal(err)
		}
		signingHash, err := CommitteeParticipationSigningHashV1(
			chainID,
			proof,
			proofHash,
		)
		if err != nil {
			t.Fatal(err)
		}
		signature, err := crypto.Sign(signingHash[:], key)
		if err != nil {
			t.Fatal(err)
		}
		group.Participations[index] = CompactCommitteeParticipationV1{
			Position:  position,
			Signature: signature,
		}
	}
	resolver := func(block uint64) (CommitteeParticipationVerificationContextV1, error) {
		if block != targetBlock {
			return CommitteeParticipationVerificationContextV1{},
				ErrInvalidCommitteeParticipationVerificationV1
		}
		return CommitteeParticipationVerificationContextV1{
			ChainID:       chainID,
			DatasetKey:    datasetKey,
			ParentHash:    parentHash,
			SelectionRoot: selectionRoot,
			Committee:     committee,
			Hasher:        hasher,
		}, nil
	}
	return headerV4ClaimFixtureV1{
		resolver: resolver,
		groups:   []CommitteeParticipationClaimGroupV1{group},
	}
}

func headerV4RuntimeContextV1(
	t *testing.T,
	chainID *big.Int,
	parentClaims *CommitteeClaimLedgerV1,
	resolver CommitteeClaimVerificationContextResolverV1,
) LQCHeaderV4RuntimeContextV1 {
	t.Helper()
	parent, datasetAnchor, challengeAnchor := headerRuntimeAdvanceTo128V1(
		t,
		chainID,
		big.NewInt(1),
	)
	return LQCHeaderV4RuntimeContextV1{
		Work: LQCHeaderWorkRuntimeContextV1{
			ChainID:         chainID,
			Parent:          parent,
			BlockNumber:     129,
			RegistryRoot:    crypto.Keccak256Hash([]byte("v4-runtime-registry")),
			DatasetAnchor:   datasetAnchor,
			ChallengeAnchor: challengeAnchor,
			Eligibility:     headerRuntimeOpenEligibilityV1,
			Hasher:          headerRuntimeHasherV1,
		},
		ParentClaims:  parentClaims,
		ResolveClaims: resolver,
	}
}

func TestLQCHeaderV4RuntimeRoundTripAndRejectsTamperedClaimRoot(t *testing.T) {
	chainID := big.NewInt(928)
	fixture := headerV4ClaimFixtureForPositionsV1(t, chainID, 128, 0, 2)
	ctx := headerV4RuntimeContextV1(
		t,
		chainID,
		NewCommitteeClaimLedgerV1(),
		fixture.resolver,
	)
	extra, workRoot, claimRoot, err := BuildLQCHeaderExtraV4WithCanonicalRuntimeV1(
		ctx,
		nil,
		nil,
		fixture.groups,
	)
	if err != nil {
		t.Fatal(err)
	}
	childHash := crypto.Keccak256Hash([]byte("v4-runtime-child"))
	envelope, nextWork, nextClaims, verified, err :=
		ValidateAndApplyLQCHeaderExtraV4WithCanonicalRuntimeV1(
			ctx,
			childHash,
			extra,
		)
	if err != nil {
		t.Fatal(err)
	}
	if envelope.WorkStateRoot != workRoot || nextWork.StateRoot != workRoot ||
		envelope.CommitteeClaimRoot != claimRoot || len(verified) != 2 ||
		!nextClaims.IsClaimed(128, 0) || !nextClaims.IsClaimed(128, 2) ||
		nextWork.Work.Hash != childHash {
		t.Fatal("v4 runtime roots, claims, or child link diverged")
	}

	tampered, err := EncodeLQCHeaderExtraV4(
		ctx.Work.BlockNumber,
		ctx.Work.RegistryRoot,
		workRoot,
		crypto.Keccak256Hash([]byte("tampered-claim-root")),
		nil,
		nil,
		fixture.groups,
		MaxWorkTicketsPerBlockV1,
	)
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, _, err = ValidateAndApplyLQCHeaderExtraV4WithCanonicalRuntimeV1(
		ctx,
		childHash,
		tampered,
	)
	if !errors.Is(err, ErrLQCHeaderCommitteeClaimRootMismatchV4) {
		t.Fatalf("tampered claim root error=%v", err)
	}
}

func TestLQCHeaderV4RuntimeRejectsDuplicateAndKeepsReorgBranchesIndependent(t *testing.T) {
	chainID := big.NewInt(928)
	parentClaims := NewCommitteeClaimLedgerV1()
	leftFixture := headerV4ClaimFixtureForPositionsV1(t, chainID, 128, 0)
	rightFixture := headerV4ClaimFixtureForPositionsV1(t, chainID, 128, 1)
	leftCtx := headerV4RuntimeContextV1(t, chainID, parentClaims, leftFixture.resolver)
	rightCtx := headerV4RuntimeContextV1(t, chainID, parentClaims, rightFixture.resolver)

	_, _, leftRoot, err := BuildLQCHeaderExtraV4WithCanonicalRuntimeV1(
		leftCtx, nil, nil, leftFixture.groups,
	)
	if err != nil {
		t.Fatal(err)
	}
	_, _, rightRoot, err := BuildLQCHeaderExtraV4WithCanonicalRuntimeV1(
		rightCtx, nil, nil, rightFixture.groups,
	)
	if err != nil {
		t.Fatal(err)
	}
	if leftRoot == rightRoot || parentClaims.IsClaimed(128, 0) ||
		parentClaims.IsClaimed(128, 1) {
		t.Fatal("reorg branches shared mutable claim state")
	}

	claimed, err := parentClaims.Apply(129, leftFixture.groups)
	if err != nil {
		t.Fatal(err)
	}
	duplicateCtx := leftCtx
	duplicateCtx.ParentClaims = claimed
	duplicateCtx.Work.BlockNumber = 130
	duplicateCtx.Work.Parent, err = computeLQCHeaderPostWorkStateV1(
		leftCtx.Work,
		nil,
		crypto.Keccak256Hash([]byte("v4-duplicate-parent")),
	)
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, err = BuildLQCHeaderExtraV4WithCanonicalRuntimeV1(
		duplicateCtx,
		nil,
		nil,
		leftFixture.groups,
	)
	if !errors.Is(err, ErrCommitteeParticipationClaimedV1) {
		t.Fatalf("duplicate claim error=%v", err)
	}
}
