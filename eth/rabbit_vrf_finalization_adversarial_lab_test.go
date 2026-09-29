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

func TestRabbitVRFFinalizationAdversarialV1(t *testing.T) {
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

	cases := map[string]func(*lqc.RabbitVRFFinalizationV1){
		"wrong_keyset":   func(v *lqc.RabbitVRFFinalizationV1) { v.KeysetRoot[0] ^= 1 },
		"wrong_epoch":    func(v *lqc.RabbitVRFFinalizationV1) { v.Epoch++ },
		"wrong_request":  func(v *lqc.RabbitVRFFinalizationV1) { v.RequestID[0] ^= 1 },
		"bad_randomness": func(v *lqc.RabbitVRFFinalizationV1) { v.Randomness[0] ^= 1 },
		"bad_proof":      func(v *lqc.RabbitVRFFinalizationV1) { v.ProofHash[0] ^= 1 },
		"bad_signature":  func(v *lqc.RabbitVRFFinalizationV1) { v.Signature[0] ^= 1 },
		"bad_participation_bitmap": func(v *lqc.RabbitVRFFinalizationV1) {
			v.ParticipationBitmap[0] ^= 0x01
		},
		"bad_participation_signature": func(v *lqc.RabbitVRFFinalizationV1) {
			v.ParticipationAggregateSignature[0] ^= 1
		},
		"wrong_round": func(v *lqc.RabbitVRFFinalizationV1) { v.Round++ },
	}

	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			bad := valid
			mutate(&bad)
			err := lqc.ValidateRabbitVRFFinalizationProofV1(ctx, members, cert, bad)
			if !errors.Is(err, lqc.ErrInvalidRabbitVRFFinalizationV1) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}
