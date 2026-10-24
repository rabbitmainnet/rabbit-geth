//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"errors"
	"math/big"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/internal/rabbitvrfstate"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/enode"
)

func TestRabbitVRFKeysetCertificateAuthenticatedRelayV1LocalIntegration(t *testing.T) {
	session, members, _, certificate := rabbitVRFAdversarialCertificateV1(t)
	shares, err := lqc.RabbitVRFKeysetCertificateVerificationSharesV1(session, certificate)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	store, err := rabbitvrfstate.NewDKGFinalKeysetStoreV1(filepath.Join(dir, "rabbit-vrf", "dkg-final-keysets"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Store(session, rabbitvrfstate.DKGFinalKeysetV1{
		KeysetRoot:         certificate.KeysetRoot,
		ThresholdPublicKey: certificate.ThresholdPublicKey,
		TranscriptRoot:     certificate.TranscriptRoot,
		VerificationShares: shares,
	}); err != nil {
		t.Fatal(err)
	}
	runtime := &rabbitVRFDKGRuntime{
		backend: &Ethereum{vrfDKGInstanceDir: dir},
		current: rabbitVRFDKGLocalContextV1{
			SessionID:        certificate.SessionID,
			CanonicalSession: session,
			CanonicalMembers: members,
		},
	}
	transport, err := newRabbitVRFDKGTransport(rabbitVRFDKGTransportConfig{
		ChainID: new(big.Int).Set(session.ChainID), NetworkID: 9280,
		Genesis: crypto.Keccak256Hash([]byte("certificate-relay-test")), Runtime: runtime,
	})
	if err != nil {
		t.Fatal(err)
	}
	relay := &rabbitVRFDKGPeer{peer: p2p.NewPeer(enode.ID{9}, "relay", nil)}
	if err := transport.register(relay); err != nil {
		t.Fatal(err)
	}
	transport.routeSession = certificate.SessionID
	transport.routes[1] = enode.ID{8}.String()
	payload, err := lqc.RabbitVRFKeysetCertificatePayloadHashV1(session, certificate)
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := lqc.NewRabbitVRFDKGEnvelopeV1(session, members[0], lqc.RabbitVRFDKGMessageKeysetCertificateV1, payload)
	if err != nil {
		t.Fatal(err)
	}
	envelope.Signature = append([]byte(nil), certificate.Signatures[0]...)

	stale := envelope
	stale.SessionID[0] ^= 1
	if _, err := transport.handleInboundKeysetCertificateEnvelopeV1(relay, stale); !errors.Is(err, errRabbitVRFDKGArtifactSessionMismatch) {
		t.Fatalf("stale session was not classified for retry/drop: %v", err)
	}
	stranger := &rabbitVRFDKGPeer{peer: p2p.NewPeer(enode.ID{7}, "unregistered", nil)}
	if _, err := transport.handleInboundKeysetCertificateEnvelopeV1(stranger, envelope); err == nil {
		t.Fatal("unregistered relay accepted")
	}
	forged := envelope
	forged.Signature = append([]byte(nil), envelope.Signature...)
	forged.Signature[0] ^= 1
	if _, err := transport.handleInboundKeysetCertificateEnvelopeV1(relay, forged); err == nil {
		t.Fatal("forged signature accepted")
	}
	wrongKeyset := envelope
	wrongKeyset.PayloadHash[0] ^= 1
	if _, err := transport.handleInboundKeysetCertificateEnvelopeV1(relay, wrongKeyset); err == nil {
		t.Fatal("wrong keyset accepted")
	}
	// Connection authentication accepts a registered relay. Actual partial
	// ownership is proved by canonical BLS and participation signatures in the
	// inbound handler, exercised by the real threshold wire regression.
	partialBinding := lqc.RabbitVRFThresholdPartialV1{
		SessionID: certificate.SessionID, ShareID: 1,
	}
	if !transport.thresholdPartialPeerAuthenticatedV1(relay, partialBinding) {
		t.Fatal("registered partial relay rejected before signature verification")
	}
	if transport.thresholdPartialPeerAuthenticatedV1(stranger, partialBinding) {
		t.Fatal("unregistered partial relay accepted")
	}
	for i, member := range members {
		env, err := lqc.NewRabbitVRFDKGEnvelopeV1(session, member, lqc.RabbitVRFDKGMessageKeysetCertificateV1, payload)
		if err != nil {
			t.Fatal(err)
		}
		env.Signature = append([]byte(nil), certificate.Signatures[i]...)
		inserted, err := transport.handleInboundKeysetCertificateEnvelopeV1(relay, env)
		if err != nil || !inserted {
			t.Fatalf("valid relayed share %d: inserted=%v err=%v", member.ShareID, inserted, err)
		}
		if i < int(session.Threshold)-1 && transport.keysetCertificate.Version != 0 {
			t.Fatal("certificate completed below threshold")
		}
	}
	certificateStore, err := transport.keysetCertificateStoreV1()
	if err != nil {
		t.Fatal(err)
	}
	persisted, err := certificateStore.Load(session, members)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.KeysetRoot != certificate.KeysetRoot {
		t.Fatal("persisted keyset root mismatch")
	}
	if _, err := lqc.ValidateRabbitVRFKeysetCertificateV1(session, members, persisted); err != nil {
		t.Fatalf("persisted certificate is invalid: %v", err)
	}
}

func TestRabbitVRFKeysetCertificateReplayAfterEarlyDeliveryV1LocalIntegration(t *testing.T) {
	session, members, _, certificate := rabbitVRFAdversarialCertificateV1(t)
	payload, err := lqc.RabbitVRFKeysetCertificatePayloadHashV1(session, certificate)
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := lqc.NewRabbitVRFDKGEnvelopeV1(session, members[0], lqc.RabbitVRFDKGMessageKeysetCertificateV1, payload)
	if err != nil {
		t.Fatal(err)
	}
	envelope.Signature = append([]byte(nil), certificate.Signatures[0]...)
	left, right := p2p.MsgPipe()
	t.Cleanup(func() { left.Close(); right.Close() })
	id := crypto.Keccak256Hash(envelope.PayloadHash[:], envelope.Signature)
	peer := &rabbitVRFDKGPeer{rw: left, known: map[common.Hash]struct{}{id: {}}}
	result := make(chan error, 1)
	go func() { result <- peer.sendKeysetCertificateEnvelopeV1(envelope, true) }()
	received := make(chan error, 1)
	go func() {
		message, err := right.ReadMsg()
		if err != nil {
			received <- err
			return
		}
		defer message.Discard()
		if message.Code != rabbitVRFDKGKeysetCertificateMsg {
			received <- errors.New("wrong replay code")
			return
		}
		var got lqc.RabbitVRFDKGEnvelopeV1
		if err := message.Decode(&got); err != nil {
			received <- err
			return
		}
		if !reflect.DeepEqual(got, envelope) {
			received <- errors.New("certificate signature changed in replay")
			return
		}
		received <- lqc.VerifyRabbitVRFDKGEnvelopeV1(session, members[0], got)
	}()
	select {
	case err := <-received:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("known signature was not replayed")
	}
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("replay sender did not finish")
	}
}
