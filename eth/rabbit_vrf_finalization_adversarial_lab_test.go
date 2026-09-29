package eth

import (
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
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

	keyset := rabbitvrfstate.DKGFinalKeysetV1{
		ThresholdPublicKey: pub,
		VerificationShares: []lqc.RabbitVRFVerificationShareV1{
			{ShareID: 1, PublicKey: p1},
			{ShareID: 2, PublicKey: p2},
		},
	}

	root := common.HexToHash("0x2222")
	request := common.HexToHash("0x3333")
	message, _, err := lqc.RabbitVRFThresholdMessageV1(ctx, root, request)
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

	sessionID, err := lqc.RabbitVRFDKGSessionIDV1(ctx)
	if err != nil {
		t.Fatal(err)
	}

	cert := lqc.RabbitVRFKeysetCertificateV1{
		Version:            lqc.RabbitVRFKeysetCertificateVersionV1,
		SessionID:          sessionID,
		KeysetRoot:         root,
		ThresholdPublicKey: pub,
		TranscriptRoot:     common.HexToHash("0x4444"),
		Signatures:         [][]byte{make([]byte, 65), nil},
	}

	round := uint64(1234)
	proof, err := lqc.RabbitVRFFinalizationProofHashV1(
		request, 11, round, sig[:],
	)
	if err != nil {
		t.Fatal(err)
	}

	valid := lqc.RabbitVRFFinalizationV1{
		Version:    lqc.RabbitVRFFinalizationVersionV1,
		RequestID:  request,
		KeysetRoot: root,
		Epoch:      11,
		Round:      round,
		Randomness: randomness,
		ProofHash:  proof,
		Signature:  sig,
	}

	if err := lqc.ValidateRabbitVRFFinalizationProofV1(ctx, cert, valid); err != nil {
		t.Fatalf("valid finalization rejected: %v", err)
	}

	cases := map[string]func(*lqc.RabbitVRFFinalizationV1){
		"wrong_keyset":   func(v *lqc.RabbitVRFFinalizationV1) { v.KeysetRoot[0] ^= 1 },
		"wrong_epoch":    func(v *lqc.RabbitVRFFinalizationV1) { v.Epoch++ },
		"wrong_request":  func(v *lqc.RabbitVRFFinalizationV1) { v.RequestID[0] ^= 1 },
		"bad_randomness": func(v *lqc.RabbitVRFFinalizationV1) { v.Randomness[0] ^= 1 },
		"bad_proof":      func(v *lqc.RabbitVRFFinalizationV1) { v.ProofHash[0] ^= 1 },
		"bad_signature":  func(v *lqc.RabbitVRFFinalizationV1) { v.Signature[0] ^= 1 },
		"wrong_round":    func(v *lqc.RabbitVRFFinalizationV1) { v.Round++ },
	}

	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			bad := valid
			mutate(&bad)
			err := lqc.ValidateRabbitVRFFinalizationProofV1(ctx, cert, bad)
			if !errors.Is(err, lqc.ErrInvalidRabbitVRFFinalizationV1) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}
