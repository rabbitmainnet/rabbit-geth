//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"crypto/ecdsa"
	"math/big"
	"path/filepath"
	"testing"

	"github.com/ethereum/go-ethereum/consensus/lqc"
	gethcrypto "github.com/ethereum/go-ethereum/crypto"
	rabbitvrf "github.com/ethereum/go-ethereum/crypto/rabbitvrf"
	"github.com/ethereum/go-ethereum/internal/rabbitvrfstate"
)

func TestRabbitVRFKeysetCertificateRelayedSignatureV1(t *testing.T) {
	chainID := big.NewInt(9280)
	epoch := uint64(11)
	committeeRoot := gethcrypto.Keccak256Hash([]byte("cert-restart"))

	session, err := lqc.NewRabbitVRFDKGSessionContextV1(
		chainID, epoch, committeeRoot, 4,
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

	sessionID, err := lqc.RabbitVRFDKGSessionIDV1(session)
	if err != nil {
		t.Fatal(err)
	}

	keys := make([]*ecdsa.PrivateKey, 4)
	members := make([]lqc.RabbitVRFCommitteeMemberV1, 4)

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
		rabbitVRFTestSecretShareV1(t, 4, 19),
	}

	verificationShares := make(
		[]lqc.RabbitVRFVerificationShareV1,
		4,
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
			4,
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

	transport := &rabbitVRFDKGTransport{
		runtime: &rabbitVRFDKGRuntime{
			backend: &Ethereum{vrfDKGInstanceDir: instanceDir},
			current: rabbitVRFDKGLocalContextV1{
				SessionID:        sessionID,
				CanonicalSession: session,
				CanonicalMembers: members,
			},
		},
	}
	_, _, _, payloadHash, ready, err := transport.currentKeysetCertificateBaseV1()
	if err != nil || !ready {
		t.Fatalf("keyset not ready: %v", err)
	}
	envelope, err := lqc.NewRabbitVRFDKGEnvelopeV1(
		session, members[0],
		lqc.RabbitVRFDKGMessageKeysetCertificateV1, payloadHash,
	)
	if err != nil {
		t.Fatal(err)
	}
	signingHash, err := lqc.RabbitVRFDKGEnvelopeSigningHashV1(session, envelope)
	if err != nil {
		t.Fatal(err)
	}
	envelope.Signature, err = gethcrypto.Sign(signingHash[:], keys[0])
	if err != nil {
		t.Fatal(err)
	}

	// No direct-author route exists: this models a retained signature relay.
	peer := &rabbitVRFDKGPeer{}
	if transport.keysetCertificatePeerAuthenticatedV1(peer, envelope) {
		t.Fatal("fixture unexpectedly has a direct-author route")
	}
	accepted, err := transport.handleInboundKeysetCertificateEnvelopeV1(peer, envelope)
	if err != nil || !accepted {
		t.Fatalf("valid relayed signature rejected: accepted=%v err=%v", accepted, err)
	}

	tampered := envelope
	tampered.Signature = append([]byte(nil), envelope.Signature...)
	tampered.Signature[0] ^= 1
	if _, err := transport.handleInboundKeysetCertificateEnvelopeV1(peer, tampered); err == nil {
		t.Fatal("tampered signature accepted")
	}

	wrongMember := envelope
	wrongMember.Participant = members[1].Participant
	if _, err := transport.handleInboundKeysetCertificateEnvelopeV1(peer, wrongMember); err == nil {
		t.Fatal("wrong participant accepted")
	}

	if _, err := transport.handleInboundKeysetCertificateEnvelopeV1(nil, envelope); err == nil {
		t.Fatal("nil peer accepted")
	}

	transport.closed = true
	if _, err := transport.handleInboundKeysetCertificateEnvelopeV1(peer, envelope); err == nil {
		t.Fatal("closed transport accepted")
	}
}
