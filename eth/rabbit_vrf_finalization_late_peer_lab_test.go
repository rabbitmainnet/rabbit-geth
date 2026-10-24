//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	gethcrypto "github.com/ethereum/go-ethereum/crypto"
	rabbitvrf "github.com/ethereum/go-ethereum/crypto/rabbitvrf"
	"github.com/ethereum/go-ethereum/internal/rabbitvrfstate"
)

func TestRabbitVRFLateFinalizationVerifiedReplayKeepsPeerV2(t *testing.T) {
	ctx, err := lqc.NewRabbitVRFDKGSessionContextV1(
		big.NewInt(9280), 11, common.HexToHash("0x1111"), 2,
	)
	if err != nil || ctx.Threshold != 2 {
		t.Fatalf("session: threshold=%d err=%v", ctx.Threshold, err)
	}

	master := rabbitVRFTestSecretShareV1(t, 99, 7)
	s1 := rabbitVRFTestSecretShareV1(t, 1, 10)
	s2 := rabbitVRFTestSecretShareV1(t, 2, 13)

	pub, err := master.PublicKey()
	if err != nil {
		t.Fatal(err)
	}
	p1, _ := s1.PublicKey()
	p2, _ := s2.PublicKey()

	verificationShares := []lqc.RabbitVRFVerificationShareV1{
		{ShareID: 1, PublicKey: p1},
		{ShareID: 2, PublicKey: p2},
	}
	transcriptRoot := common.HexToHash("0x4444")
	root, canonicalShares, err := lqc.RabbitVRFKeysetRootV1(
		ctx.ChainID,
		ctx.TargetVRFEpoch,
		ctx.CommitteeRoot,
		ctx.CommitteeSize,
		ctx.Threshold,
		pub,
		transcriptRoot,
		verificationShares,
	)
	if err != nil {
		t.Fatal(err)
	}
	keyset := rabbitvrfstate.DKGFinalKeysetV1{
		KeysetRoot:         root,
		ThresholdPublicKey: pub,
		TranscriptRoot:     transcriptRoot,
		VerificationShares: canonicalShares,
	}

	members := []lqc.RabbitVRFCommitteeMemberV1{
		{
			ShareID:     1,
			TicketHash:  gethcrypto.Keccak256Hash([]byte("rabbit-vrf-finalization-ticket-1")),
			Participant: common.HexToAddress("0x0000000000000000000000000000000000000011"),
		},
		{
			ShareID:     2,
			TicketHash:  gethcrypto.Keccak256Hash([]byte("rabbit-vrf-finalization-ticket-2")),
			Participant: common.HexToAddress("0x0000000000000000000000000000000000000022"),
		},
	}

	request := common.HexToHash("0x3333")
	message, messageHash, err := lqc.RabbitVRFThresholdMessageV1(ctx, root, request)
	if err != nil {
		t.Fatal(err)
	}

	partials := make([]rabbitvrf.PartialSignature, 0, 2)
	for i, share := range []*rabbitvrf.SecretShare{s1, s2} {
		partial, _, err := rabbitVRFSignThresholdPartialWithKeysetV1(
			share, uint64(i+1), keyset, message,
		)
		if err != nil {
			t.Fatal(err)
		}
		partials = append(partials, partial)
	}

	sig, randomness, err := rabbitVRFCombineThresholdPartialsWithKeysetV1(
		2, keyset, message, partials,
	)
	if err != nil {
		t.Fatal(err)
	}

	participationSignatures := make([]rabbitvrf.Signature, 0, 2)
	for index, share := range []*rabbitvrf.SecretShare{s1, s2} {
		participationMessage, _, err := lqc.RabbitVRFCompactParticipationMessageV1(
			ctx,
			root,
			request,
			messageHash,
			members[index],
		)
		if err != nil {
			t.Fatal(err)
		}
		participationPartial, err := share.SignPartial(participationMessage)
		if err != nil {
			t.Fatal(err)
		}
		participationSignatures = append(
			participationSignatures,
			participationPartial.Signature,
		)
	}
	participationAggregateSignature, err := rabbitvrf.AggregateSignaturesV1(
		participationSignatures,
	)
	if err != nil {
		t.Fatal(err)
	}
	participationBitmap, err := lqc.RabbitVRFCompactParticipationFixedBitmapV1(
		ctx,
		[]uint64{1, 2},
	)
	if err != nil {
		t.Fatal(err)
	}

	sessionID, err := lqc.RabbitVRFDKGSessionIDV1(ctx)
	if err != nil {
		t.Fatal(err)
	}

	cert := lqc.RabbitVRFKeysetCertificateV1{
		Version:            lqc.RabbitVRFKeysetCertificateVersionV1,
		SessionID:          sessionID,
		KeysetRoot:         root,
		ThresholdPublicKey: pub,
		TranscriptRoot:     transcriptRoot,
		VerificationShareSamples: append(
			[]lqc.RabbitVRFVerificationShareV1(nil),
			canonicalShares[:int(ctx.Threshold)]...,
		),
		Signatures: [][]byte{make([]byte, 65), nil},
	}

	round := uint64(1234)
	proof, err := lqc.RabbitVRFFinalizationProofHashWithParticipationV1(
		request,
		11,
		round,
		sig[:],
		participationBitmap,
		participationAggregateSignature,
	)
	if err != nil {
		t.Fatal(err)
	}

	valid := lqc.RabbitVRFFinalizationV1{
		Version:                         lqc.RabbitVRFFinalizationVersionV1,
		RequestID:                       request,
		KeysetRoot:                      root,
		Epoch:                           11,
		Round:                           round,
		Randomness:                      randomness,
		ProofHash:                       proof,
		Signature:                       sig,
		ParticipationBitmap:             participationBitmap,
		ParticipationAggregateSignature: participationAggregateSignature,
	}

	if err := lqc.ValidateRabbitVRFFinalizationProofV1(ctx, members, cert, valid); err != nil {
		t.Fatalf("valid finalization rejected: %v", err)
	}

	// Exact repeats may reuse cryptography; altered inputs must not.
	for i := 0; i < 3; i++ {
		if err := rabbitVRFValidateFinalizationMemoV1(ctx, members, cert, valid); err != nil {
			t.Fatal(err)
		}
	}
	alteredProof := valid
	alteredProof.Signature[0] ^= 1
	if err := rabbitVRFValidateFinalizationMemoV1(ctx, members, cert, alteredProof); err == nil {
		t.Fatal("cached verification accepted altered signature")
	}
	alteredCertificate := cert
	alteredCertificate.KeysetRoot[0] ^= 1
	if err := rabbitVRFValidateFinalizationMemoV1(ctx, members, alteredCertificate, valid); err == nil {
		t.Fatal("cached verification accepted altered certificate")
	}
	alteredContext := ctx
	alteredContext.ChainID = new(big.Int).Add(ctx.ChainID, big.NewInt(1))
	if err := rabbitVRFValidateFinalizationMemoV1(alteredContext, members, cert, valid); err == nil {
		t.Fatal("cached verification accepted another chain")
	}
	alteredMembers := append([]lqc.RabbitVRFCommitteeMemberV1(nil), members...)
	alteredMembers[0].Participant[0] ^= 1
	if err := rabbitVRFValidateFinalizationMemoV1(ctx, alteredMembers, cert, valid); err == nil {
		t.Fatal("cached verification accepted another committee")
	}

	runtime := &rabbitVRFDKGRuntime{current: rabbitVRFDKGLocalContextV1{
		SessionID: sessionID, CanonicalSession: ctx, CanonicalMembers: members,
	}}
	pending := true
	lookups := 0
	runtime.canonicalRequestLookup = func(id common.Hash) (rabbitVRFCanonicalRequestV1, error) {
		lookups++
		if id != request {
			return rabbitVRFCanonicalRequestV1{}, errors.New("unexpected request")
		}
		if !pending {
			return rabbitVRFCanonicalRequestV1{}, errRabbitVRFRequestNotPendingV1
		}
		return rabbitVRFCanonicalRequestV1{RequestID: request, RequestBlock: round}, nil
	}
	receiver := &rabbitVRFDKGTransport{runtime: runtime}
	accepted, err := receiver.acceptFinalizationFromPeerV2(valid, cert)
	if err != nil || !accepted || len(receiver.finalizations) != 1 {
		t.Fatalf("pending admission: accepted=%v err=%v", accepted, err)
	}
	pending = false
	for i := 0; i < 3; i++ {
		accepted, err = receiver.acceptFinalizationFromPeerV2(valid, cert)
		if err != nil || accepted {
			t.Fatalf("valid late replay became peer error: accepted=%v err=%v", accepted, err)
		}
	}
	if len(receiver.finalizations) != 1 {
		t.Fatal("late replay changed proof store")
	}
	before := lookups
	forged := valid
	forged.Signature[0] ^= 1
	accepted, err = receiver.acceptFinalizationFromPeerV2(forged, cert)
	if err == nil || accepted {
		t.Fatal("forged late proof accepted")
	}
	if lookups != before {
		t.Fatal("forged proof reached pending-state exception before cryptographic verification")
	}
	pending = true
	runtime.canonicalRequestLookup = func(common.Hash) (rabbitVRFCanonicalRequestV1, error) {
		return rabbitVRFCanonicalRequestV1{}, errors.New("state unavailable")
	}
	accepted, err = receiver.acceptFinalizationFromPeerV2(valid, cert)
	if err == nil || accepted {
		t.Fatal("unrelated lookup error was hidden")
	}
}
