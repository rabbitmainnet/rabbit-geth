//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"crypto/ecdsa"
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/consensus/lqc"
	gethcrypto "github.com/ethereum/go-ethereum/crypto"
	rabbitvrf "github.com/ethereum/go-ethereum/crypto/rabbitvrf"
)

func rabbitVRFAdversarialCertificateV1(
	t *testing.T,
) (
	lqc.RabbitVRFDKGSessionContextV1,
	[]lqc.RabbitVRFCommitteeMemberV1,
	[]*ecdsa.PrivateKey,
	lqc.RabbitVRFKeysetCertificateV1,
) {
	t.Helper()

	chainID := big.NewInt(9280)
	epoch := uint64(11)
	committeeRoot := gethcrypto.Keccak256Hash(
		[]byte("cert-adversarial"),
	)

	session, err := lqc.NewRabbitVRFDKGSessionContextV1(
		chainID,
		epoch,
		committeeRoot,
		3,
	)
	if err != nil {
		t.Fatal(err)
	}

	sessionID, err := lqc.RabbitVRFDKGSessionIDV1(session)
	if err != nil {
		t.Fatal(err)
	}

	keys := make([]*ecdsa.PrivateKey, 3)
	members := make([]lqc.RabbitVRFCommitteeMemberV1, 3)

	for i := range members {
		key, err := gethcrypto.GenerateKey()
		if err != nil {
			t.Fatal(err)
		}
		keys[i] = key
		members[i] = lqc.RabbitVRFCommitteeMemberV1{
			ShareID: uint64(i + 1),
			TicketHash: gethcrypto.Keccak256Hash(
				[]byte{byte(i + 1), 0xa1},
			),
			Participant: gethcrypto.PubkeyToAddress(
				key.PublicKey,
			),
		}
	}

	master := rabbitVRFTestSecretShareV1(t, 99, 7)

	thresholdPublicKey, err := master.PublicKey()
	if err != nil {
		t.Fatal(err)
	}

	shares := []*rabbitvrf.SecretShare{
		rabbitVRFTestSecretShareV1(t, 1, 10),
		rabbitVRFTestSecretShareV1(t, 2, 13),
		rabbitVRFTestSecretShareV1(t, 3, 16),
	}

	verificationShares := make(
		[]lqc.RabbitVRFVerificationShareV1,
		3,
	)

	for i, share := range shares {
		publicKey, err := share.PublicKey()
		if err != nil {
			t.Fatal(err)
		}
		verificationShares[i] =
			lqc.RabbitVRFVerificationShareV1{
				ShareID:   uint64(i + 1),
				PublicKey: publicKey,
			}
	}

	transcriptRoot := gethcrypto.Keccak256Hash(
		[]byte("cert-adversarial-transcript"),
	)

	keysetRoot, canonicalShares, err := lqc.RabbitVRFKeysetRootV1(
		chainID,
		epoch,
		committeeRoot,
		3,
		session.Threshold,
		thresholdPublicKey,
		transcriptRoot,
		verificationShares,
	)
	if err != nil {
		t.Fatal(err)
	}

	certificate := lqc.RabbitVRFKeysetCertificateV1{
		Version:            lqc.RabbitVRFKeysetCertificateVersionV1,
		SessionID:          sessionID,
		KeysetRoot:         keysetRoot,
		ThresholdPublicKey: thresholdPublicKey,
		TranscriptRoot:     transcriptRoot,
		VerificationShareSamples: append(
			[]lqc.RabbitVRFVerificationShareV1(nil),
			canonicalShares[:int(session.Threshold)]...,
		),
		Signatures: make([][]byte, 3),
	}

	payloadHash, err :=
		lqc.RabbitVRFKeysetCertificatePayloadHashV1(
			session,
			certificate,
		)
	if err != nil {
		t.Fatal(err)
	}

	for i, member := range members {
		envelope, err := lqc.NewRabbitVRFDKGEnvelopeV1(
			session,
			member,
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

		certificate.Signatures[i], err =
			gethcrypto.Sign(signingHash[:], keys[i])
		if err != nil {
			t.Fatal(err)
		}
	}

	certificate, err =
		lqc.ValidateRabbitVRFKeysetCertificateV1(
			session,
			members,
			certificate,
		)
	if err != nil {
		t.Fatal(err)
	}

	return session, members, keys, certificate
}

func cloneRabbitVRFCertificateV1(
	certificate lqc.RabbitVRFKeysetCertificateV1,
) lqc.RabbitVRFKeysetCertificateV1 {
	out := certificate
	out.Signatures = make([][]byte, len(certificate.Signatures))

	for i := range certificate.Signatures {
		out.Signatures[i] =
			append([]byte(nil), certificate.Signatures[i]...)
	}

	return out
}

func TestRabbitVRFKeysetCertificateAdversarialV1(
	t *testing.T,
) {
	session, members, _, certificate :=
		rabbitVRFAdversarialCertificateV1(t)

	reject := func(
		t *testing.T,
		members []lqc.RabbitVRFCommitteeMemberV1,
		certificate lqc.RabbitVRFKeysetCertificateV1,
	) {
		t.Helper()

		_, err := lqc.ValidateRabbitVRFKeysetCertificateV1(
			session,
			members,
			certificate,
		)
		if !errors.Is(
			err,
			lqc.ErrInvalidRabbitVRFKeysetCertificateV1,
		) {
			t.Fatalf(
				"error=%v want=%v",
				err,
				lqc.ErrInvalidRabbitVRFKeysetCertificateV1,
			)
		}
	}

	t.Run("valid_control", func(t *testing.T) {
		if _, err := lqc.ValidateRabbitVRFKeysetCertificateV1(
			session,
			members,
			cloneRabbitVRFCertificateV1(certificate),
		); err != nil {
			t.Fatalf("valid certificate rejected: %v", err)
		}
	})

	t.Run("missing_signature", func(t *testing.T) {
		bad := cloneRabbitVRFCertificateV1(certificate)
		bad.Signatures = bad.Signatures[:2]
		reject(t, members, bad)
	})

	t.Run("short_signature", func(t *testing.T) {
		bad := cloneRabbitVRFCertificateV1(certificate)
		bad.Signatures[1] =
			append([]byte(nil), bad.Signatures[1][:64]...)
		reject(t, members, bad)
	})

	t.Run("swapped_member_signatures", func(t *testing.T) {
		bad := cloneRabbitVRFCertificateV1(certificate)
		bad.Signatures[0], bad.Signatures[1] =
			bad.Signatures[1], bad.Signatures[0]
		reject(t, members, bad)
	})

	t.Run("wrong_session_id", func(t *testing.T) {
		bad := cloneRabbitVRFCertificateV1(certificate)
		bad.SessionID = gethcrypto.Keccak256Hash(
			[]byte("wrong-session"),
		)
		reject(t, members, bad)
	})

	t.Run("tampered_keyset_root", func(t *testing.T) {
		bad := cloneRabbitVRFCertificateV1(certificate)
		bad.KeysetRoot = gethcrypto.Keccak256Hash(
			[]byte("tampered-keyset-root"),
		)
		reject(t, members, bad)
	})

	t.Run("non_canonical_members", func(t *testing.T) {
		badMembers := append(
			[]lqc.RabbitVRFCommitteeMemberV1(nil),
			members...,
		)
		badMembers[0], badMembers[1] =
			badMembers[1], badMembers[0]

		reject(
			t,
			badMembers,
			cloneRabbitVRFCertificateV1(certificate),
		)
	})
}
