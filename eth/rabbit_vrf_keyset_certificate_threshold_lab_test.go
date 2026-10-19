//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"crypto/ecdsa"
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/consensus/lqc"
	gethcrypto "github.com/ethereum/go-ethereum/crypto"
)

func TestRabbitVRFKeysetCertificateThresholdQuorumV1(
	t *testing.T,
) {
	session, err := lqc.NewRabbitVRFDKGSessionContextV1(
		big.NewInt(9280),
		11,
		gethcrypto.Keccak256Hash(
			[]byte("threshold-certificate-committee"),
		),
		4,
	)
	if err != nil {
		t.Fatal(err)
	}

	if session.Threshold != 3 || session.MaxFaults != 1 {
		t.Fatalf(
			"threshold=%d faults=%d want=3/1",
			session.Threshold,
			session.MaxFaults,
		)
	}

	sessionID, err :=
		lqc.RabbitVRFDKGSessionIDV1(session)
	if err != nil {
		t.Fatal(err)
	}

	keys := make([]*ecdsa.PrivateKey, 4)
	members := make(
		[]lqc.RabbitVRFCommitteeMemberV1,
		4,
	)

	for index := range members {
		key, err := gethcrypto.GenerateKey()
		if err != nil {
			t.Fatal(err)
		}

		keys[index] = key
		members[index] =
			lqc.RabbitVRFCommitteeMemberV1{
				ShareID: uint64(index + 1),
				TicketHash: gethcrypto.Keccak256Hash(
					[]byte{
						byte(index + 1),
						0xc4,
					},
				),
				Participant: gethcrypto.PubkeyToAddress(
					key.PublicKey,
				),
			}
	}

	secret :=
		rabbitVRFTestSecretShareV1(t, 99, 7)

	thresholdPublicKey, err := secret.PublicKey()
	if err != nil {
		t.Fatal(err)
	}

	// f(x) = 7 + 3x + 5x^2 gives the canonical 3-of-4 verification shares.
	verificationScalars := []byte{15, 33, 61, 99}
	verificationShares := make(
		[]lqc.RabbitVRFVerificationShareV1,
		len(verificationScalars),
	)
	for index, scalar := range verificationScalars {
		share := rabbitVRFTestSecretShareV1(
			t,
			uint64(index+1),
			uint64(scalar),
		)
		publicKey, err := share.PublicKey()
		if err != nil {
			t.Fatal(err)
		}
		verificationShares[index] = lqc.RabbitVRFVerificationShareV1{
			ShareID:   uint64(index + 1),
			PublicKey: publicKey,
		}
	}

	transcriptRoot := gethcrypto.Keccak256Hash(
		[]byte("threshold-transcript"),
	)
	keysetRoot, canonicalShares, err := lqc.RabbitVRFKeysetRootV1(
		session.ChainID,
		session.TargetVRFEpoch,
		session.CommitteeRoot,
		session.CommitteeSize,
		session.Threshold,
		thresholdPublicKey,
		transcriptRoot,
		verificationShares,
	)
	if err != nil {
		t.Fatal(err)
	}

	certificate :=
		lqc.RabbitVRFKeysetCertificateV1{
			Version:            lqc.RabbitVRFKeysetCertificateVersionV1,
			SessionID:          sessionID,
			KeysetRoot:         keysetRoot,
			ThresholdPublicKey: thresholdPublicKey,
			TranscriptRoot:     transcriptRoot,
			VerificationShareSamples: append(
				[]lqc.RabbitVRFVerificationShareV1(nil),
				canonicalShares[:int(session.Threshold)]...,
			),
			Signatures: make([][]byte, 4),
		}

	payloadHash, err :=
		lqc.RabbitVRFKeysetCertificatePayloadHashV1(
			session,
			certificate,
		)
	if err != nil {
		t.Fatal(err)
	}

	// Members 1,2,3 sign. Member 4 is offline.
	for index := 0; index < 3; index++ {
		envelope, err :=
			lqc.NewRabbitVRFDKGEnvelopeV1(
				session,
				members[index],
				lqc.RabbitVRFDKGMessageKeysetCertificateV1,
				payloadHash,
			)
		if err != nil {
			t.Fatal(err)
		}

		signingHash, err :=
			lqc.RabbitVRFDKGEnvelopeSigningHashV1(
				session,
				envelope,
			)
		if err != nil {
			t.Fatal(err)
		}

		certificate.Signatures[index], err =
			gethcrypto.Sign(
				signingHash[:],
				keys[index],
			)
		if err != nil {
			t.Fatal(err)
		}
	}

	if err :=
		lqc.ValidateRabbitVRFKeysetCertificateShapeV1(
			certificate,
		); err != nil {
		t.Fatalf(
			"3-of-4 shape rejected: %v",
			err,
		)
	}

	validated, err :=
		lqc.ValidateRabbitVRFKeysetCertificateV1(
			session,
			members,
			certificate,
		)
	if err != nil {
		t.Fatalf(
			"3-of-4 certificate rejected: %v",
			err,
		)
	}

	if len(validated.Signatures) != 4 {
		t.Fatalf(
			"slots=%d want=4",
			len(validated.Signatures),
		)
	}

	if len(validated.Signatures[3]) != 0 {
		t.Fatal(
			"offline member slot must remain empty",
		)
	}

	clone := func(
		input lqc.RabbitVRFKeysetCertificateV1,
	) lqc.RabbitVRFKeysetCertificateV1 {
		out := input
		out.VerificationShareSamples = append(
			[]lqc.RabbitVRFVerificationShareV1(nil),
			input.VerificationShareSamples...,
		)
		out.Signatures =
			make([][]byte, len(input.Signatures))
		for index := range input.Signatures {
			out.Signatures[index] =
				append(
					[]byte(nil),
					input.Signatures[index]...,
				)
		}
		return out
	}

	// 2/4 is below threshold and must fail.
	below := clone(validated)
	below.Signatures[2] = nil

	_, err =
		lqc.ValidateRabbitVRFKeysetCertificateV1(
			session,
			members,
			below,
		)

	if !errors.Is(
		err,
		lqc.ErrInvalidRabbitVRFKeysetCertificateV1,
	) {
		t.Fatalf(
			"2-of-4 error=%v",
			err,
		)
	}

	// Empty slots are allowed, malformed non-empty slots are not.
	malformed := clone(validated)
	malformed.Signatures[3] = make([]byte, 64)

	if err :=
		lqc.ValidateRabbitVRFKeysetCertificateShapeV1(
			malformed,
		); !errors.Is(
		err,
		lqc.ErrInvalidRabbitVRFKeysetCertificateV1,
	) {
		t.Fatalf(
			"malformed shape error=%v",
			err,
		)
	}
}
