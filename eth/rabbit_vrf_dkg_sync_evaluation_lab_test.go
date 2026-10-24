//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/eth/ethconfig"
	"github.com/ethereum/go-ethereum/internal/rabbitvrfstate"
	"github.com/ethereum/go-ethereum/p2p"
)

func TestRabbitVRFDKGEncryptedEvaluationSyncGateKeepsPeerV1(t *testing.T) {
	transport, artifact := newRabbitVRFDKGTransportArtifactFixtureV1(t)
	runtime := transport.runtime
	context := runtime.currentContext()
	recipient := context.CanonicalMembers[0]
	dealerKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	dealer := lqc.RabbitVRFCommitteeMemberV1{
		ShareID: 2, Participant: crypto.PubkeyToAddress(dealerKey.PublicKey),
		TicketHash: crypto.Keccak256Hash([]byte("sync-gate-dealer")),
	}
	dealerStore, err := rabbitvrfstate.NewDKGDealerPolynomialStoreV1(t.TempDir(), 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	polynomial, commitment, _, err := dealerStore.Create(context.CanonicalSession, dealer, "test-password")
	if err != nil {
		t.Fatal(err)
	}
	defer polynomial.Destroy()
	evaluation, err := polynomial.Evaluate(recipient.ShareID)
	if err != nil {
		t.Fatal(err)
	}
	packet, _, _, err := lqc.NewRabbitVRFDKGEncryptedEvaluationV1(
		context.CanonicalSession, dealer, commitment, recipient,
		artifact.Binding, artifact.Envelope, evaluation,
	)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := lqc.RabbitVRFDKGEncryptedEvaluationSigningHashV1(context.CanonicalSession, packet)
	if err != nil {
		t.Fatal(err)
	}
	packet.Signature, err = crypto.Sign(hash[:], dealerKey)
	if err != nil {
		t.Fatal(err)
	}

	instance := filepath.Join(t.TempDir(), "must-not-open-secret-store")
	runtime.backend = &Ethereum{config: &ethconfig.Config{}, vrfDKGInstanceDir: instance, vrfDKGTransport: transport}
	runtime.current.CanonicalMembers = append(runtime.current.CanonicalMembers, dealer)
	runtime.current.Members = []lqc.RabbitVRFCommitteeMemberV1{recipient}
	runtime.syncing = true
	runtime.secretReady = false
	if err := runtime.validateInboundEncryptedEvaluationV1(packet); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.decryptInboundEncryptedEvaluationV1(packet); !errors.Is(err, errRabbitVRFDKGSecretTemporarilyUnavailableV1) {
		t.Fatalf("expected typed temporary gate error: %v", err)
	}

	rw, result := startRabbitVRFDKGTransportArtifactWireV1(t, transport)
	if err := p2p.Send(rw, rabbitVRFDKGEncryptedEvaluationMsg, packet); err != nil {
		t.Fatal(err)
	}
	// Receiving a subsequent authenticated artifact proves the same connection
	// survived the deferred evaluation and its read loop kept making progress.
	sent := make(chan error, 1)
	go func() { sent <- p2p.Send(rw, rabbitVRFDKGTransportArtifactMsg, artifact) }()
	select {
	case err := <-sent:
		if err != nil {
			t.Fatal(err)
		}
	case err := <-result:
		t.Fatalf("sync-gated evaluation disconnected peer: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("peer stopped reading after sync-gated evaluation")
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		transport.mu.RLock()
		_, accepted := transport.remoteArtifacts[recipient.ShareID]
		transport.mu.RUnlock()
		if accepted {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("subsequent artifact not accepted")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := os.Stat(instance); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("secret storage accessed while gate closed: %v", err)
	}
	// Invalid signatures remain fatal even while the secret gate is closed.
	forged := packet
	forged.Signature = append([]byte(nil), packet.Signature...)
	forged.Signature[0] ^= 1
	if err := p2p.Send(rw, rabbitVRFDKGEncryptedEvaluationMsg, forged); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-result:
		if err == nil || errors.Is(err, errRabbitVRFDKGSecretTemporarilyUnavailableV1) {
			t.Fatalf("forged evaluation not rejected cryptographically: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("forged evaluation did not terminate invalid peer")
	}
}
