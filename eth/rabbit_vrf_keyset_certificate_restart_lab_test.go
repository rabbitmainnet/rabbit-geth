//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"crypto/ecdsa"
	"math/big"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ethereum/go-ethereum/consensus/lqc"
	gethcrypto "github.com/ethereum/go-ethereum/crypto"
	rabbitvrf "github.com/ethereum/go-ethereum/crypto/rabbitvrf"
	"github.com/ethereum/go-ethereum/internal/rabbitvrfstate"
)

func TestRabbitVRFKeysetCertificateRestartV1(t *testing.T) {
	chainID := big.NewInt(9280)
	epoch := uint64(11)
	committeeRoot := gethcrypto.Keccak256Hash([]byte("cert-restart"))

	session, err := lqc.NewRabbitVRFDKGSessionContextV1(
		chainID, epoch, committeeRoot, 3,
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
				[]byte{byte(i + 1), 0x91},
			),
			Participant: gethcrypto.PubkeyToAddress(key.PublicKey),
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
		[]byte("cert-restart-transcript"),
	)

	keysetRoot, canonicalShares, err :=
		lqc.RabbitVRFKeysetRootV1(
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

	instanceDir := t.TempDir()

	keysetStore, err := rabbitvrfstate.NewDKGFinalKeysetStoreV1(
		filepath.Join(
			instanceDir,
			"rabbit-vrf",
			"dkg-final-keysets",
		),
	)
	if err != nil {
		t.Fatal(err)
	}

	err = keysetStore.Store(
		session,
		rabbitvrfstate.DKGFinalKeysetV1{
			KeysetRoot:         keysetRoot,
			ThresholdPublicKey: thresholdPublicKey,
			TranscriptRoot:     transcriptRoot,
			VerificationShares: canonicalShares,
		},
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
		Signatures:         make([][]byte, 3),
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

	certificateStore, err :=
		rabbitvrfstate.NewRabbitVRFKeysetCertificateStoreV1(
			filepath.Join(
				instanceDir,
				"rabbit-vrf",
				"keyset-certificates",
			),
		)
	if err != nil {
		t.Fatal(err)
	}

	if err := certificateStore.Store(
		session,
		members,
		certificate,
	); err != nil {
		t.Fatal(err)
	}

	runtime := &rabbitVRFDKGRuntime{
		backend: &Ethereum{
			vrfDKGInstanceDir: instanceDir,
		},
		current: rabbitVRFDKGLocalContextV1{
			SessionID:        sessionID,
			TargetVRFEpoch:   epoch,
			CanonicalSession: session,
			CanonicalMembers: members,
			Members:          members,
		},
	}

	restarted := &rabbitVRFDKGTransport{
		runtime: runtime,
	}

	if err := restarted.restorePersistedKeysetCertificateV1(); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(
		restarted.keysetCertificate,
		certificate,
	) {
		t.Fatal("restored certificate mismatch")
	}

	if len(restarted.keysetCertificateSignatures) != 3 {
		t.Fatal("restored certificate signatures missing")
	}
}
