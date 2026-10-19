//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"crypto/ecdsa"
	"errors"
	"math/big"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/consensus/lqc"
	gethcrypto "github.com/ethereum/go-ethereum/crypto"
	rabbitvrf "github.com/ethereum/go-ethereum/crypto/rabbitvrf"
	"github.com/ethereum/go-ethereum/internal/rabbitvrfstate"
	"github.com/ethereum/go-ethereum/p2p"
)

func TestRabbitVRFKeysetCertificateRestartV1(t *testing.T) {
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

	for i, member := range members[:int(session.Threshold)] {
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

	collector := &rabbitVRFDKGTransport{
		runtime: runtime,
	}

	for i := 0; i < int(session.Threshold); i++ {
		envelope, err := lqc.NewRabbitVRFDKGEnvelopeV1(
			session,
			members[i],
			lqc.RabbitVRFDKGMessageKeysetCertificateV1,
			payloadHash,
		)
		if err != nil {
			t.Fatal(err)
		}

		envelope.Signature =
			append([]byte(nil), certificate.Signatures[i]...)

		if _, err :=
			collector.collectKeysetCertificateEnvelopeV1(
				envelope,
			); err != nil {
			t.Fatalf(
				"collect certificate share %d: %v",
				i+1,
				err,
			)
		}

		collector.mu.RLock()
		count := len(collector.keysetCertificateSignatures)
		ready :=
			collector.keysetCertificate.Version ==
				lqc.RabbitVRFKeysetCertificateVersionV1
		collector.mu.RUnlock()

		if count != i+1 {
			t.Fatalf(
				"collector signatures=%d want=%d",
				count,
				i+1,
			)
		}

		if i+1 < int(session.Threshold) && ready {
			t.Fatalf(
				"certificate became ready before threshold at share %d",
				i+1,
			)
		}

		if i+1 == int(session.Threshold) && !ready {
			t.Fatal(
				"certificate was not produced at threshold",
			)
		}
	}

	collector.mu.RLock()
	collected :=
		cloneRabbitVRFKeysetCertificateV1(
			collector.keysetCertificate,
		)
	collector.mu.RUnlock()

	if !reflect.DeepEqual(collected, certificate) {
		t.Fatal("collector certificate mismatch")
	}

	if len(collected.Signatures) != 4 ||
		len(collected.Signatures[3]) != 0 {
		t.Fatal(
			"offline member must remain an empty canonical slot",
		)
	}

	certificate = collected

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

	if len(restarted.keysetCertificateSignatures) !=
		int(session.Threshold) {
		t.Fatalf(
			"restored signatures=%d want=%d",
			len(restarted.keysetCertificateSignatures),
			session.Threshold,
		)
	}

	leftRW, rightRW := p2p.MsgPipe()
	t.Cleanup(func() {
		leftRW.Close()
		rightRW.Close()
	})

	peer := &rabbitVRFDKGPeer{
		rw: leftRW,
	}

	sendResult := make(chan error, 1)
	go func() {
		sendResult <- restarted.sendPendingTransportArtifactsV1(peer)
	}()

	envelopes := make(
		chan lqc.RabbitVRFDKGEnvelopeV1,
		int(session.Threshold),
	)
	readErr := make(chan error, 1)

	go func() {
		for i := uint64(0); i < session.Threshold; i++ {
			message, err := rightRW.ReadMsg()
			if err != nil {
				readErr <- err
				return
			}

			if message.Code !=
				rabbitVRFDKGKeysetCertificateMsg {
				message.Discard()
				readErr <- errors.New(
					"unexpected late-peer message code",
				)
				return
			}

			var envelope lqc.RabbitVRFDKGEnvelopeV1
			if err := message.Decode(&envelope); err != nil {
				message.Discard()
				readErr <- err
				return
			}
			message.Discard()

			envelopes <- envelope
		}
	}()

	seen := make(map[uint64]bool)

	for i := uint64(0); i < session.Threshold; i++ {
		select {
		case err := <-readErr:
			t.Fatalf(
				"late-peer certificate sync failed: %v",
				err,
			)

		case envelope := <-envelopes:
			if envelope.SessionID != sessionID ||
				envelope.MessageType !=
					lqc.RabbitVRFDKGMessageKeysetCertificateV1 ||
				envelope.PayloadHash != payloadHash {
				t.Fatal(
					"late-peer certificate envelope binding mismatch",
				)
			}

			if envelope.SenderShareID == 0 ||
				envelope.SenderShareID >
					uint64(len(members)) {
				t.Fatal(
					"late-peer certificate share id invalid",
				)
			}

			if seen[envelope.SenderShareID] {
				t.Fatal(
					"late-peer received duplicate certificate share",
				)
			}
			seen[envelope.SenderShareID] = true

			member :=
				members[envelope.SenderShareID-1]

			if err := lqc.VerifyRabbitVRFDKGEnvelopeV1(
				session,
				member,
				envelope,
			); err != nil {
				t.Fatalf(
					"late-peer certificate signature invalid: %v",
					err,
				)
			}

		case <-time.After(2 * time.Second):
			t.Fatal(
				"late peer did not receive all keyset certificate signatures",
			)
		}
	}

	select {
	case err := <-sendResult:
		if err != nil {
			t.Fatalf(
				"late-peer certificate sender failed: %v",
				err,
			)
		}

	case <-time.After(2 * time.Second):
		t.Fatal(
			"late-peer certificate sender did not finish",
		)
	}

	if len(seen) != int(session.Threshold) {
		t.Fatalf(
			"late peer received %d signatures, want %d",
			len(seen),
			session.Threshold,
		)
	}

	if seen[4] {
		t.Fatal(
			"late peer received signature from offline member",
		)
	}

}
