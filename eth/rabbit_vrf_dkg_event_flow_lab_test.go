//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/eth/ethconfig"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/internal/rabbitvrfstate"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/enode"
)

func TestRabbitVRFDKGHeadFeedProgressesWithSlowWorkerV1(t *testing.T) {
	var feed event.Feed
	raw := make(chan core.ChainHeadEvent, 16)
	wake := make(chan core.ChainHeadEvent, 1)
	stop := make(chan struct{})
	pumpDone := make(chan struct{})
	sub := feed.Subscribe(raw)
	t.Cleanup(func() { sub.Unsubscribe(); close(stop); <-pumpDone })
	go func() { defer close(pumpDone); rabbitVRFDKGCoalesceHeadsV1(raw, wake, stop) }()
	sent := make(chan struct{})
	go func() {
		for i := 0; i < 1000; i++ {
			feed.Send(core.ChainHeadEvent{})
		}
		close(sent)
	}()
	// Do not consume wake: model a DKG worker spending seconds in scrypt.
	select {
	case <-sent:
	case <-time.After(3 * time.Second):
		t.Fatal("canonical head feed blocked behind DKG worker")
	}
	if len(wake) != 1 {
		t.Fatal("expected one coalesced head notification")
	}
	<-wake
	sentAgain := make(chan struct{})
	go func() { feed.Send(core.ChainHeadEvent{}); close(sentAgain) }()
	select {
	case <-sentAgain:
	case <-time.After(time.Second):
		t.Fatal("later head feed blocked")
	}
	select {
	case <-wake:
	case <-time.After(time.Second):
		t.Fatal("later head notification lost")
	}
}

func TestRabbitVRFDKGFinalizedReplayAvoidsSecretStoresV1(t *testing.T) {
	transport, members, artifacts, keys := newRabbitVRFDKGCanonicalTransportKeySetFixtureV1(t)
	artifact := artifacts[0]
	runtime := transport.runtime
	context := runtime.currentContext()
	recipient, dealer, dealerKey := members[0], members[1], keys[1]
	store, err := rabbitvrfstate.NewDKGDealerPolynomialStoreV1(t.TempDir(), 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	polynomial, commitment, _, err := store.Create(context.CanonicalSession, dealer, "test-password")
	if err != nil {
		t.Fatal(err)
	}
	defer polynomial.Destroy()
	value, err := polynomial.Evaluate(recipient.ShareID)
	if err != nil {
		t.Fatal(err)
	}
	packet, _, _, err := lqc.NewRabbitVRFDKGEncryptedEvaluationV1(context.CanonicalSession, dealer, commitment,
		recipient, artifact.Binding, artifact.Envelope, value)
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
	context.CanonicalTransportBindings = []lqc.RabbitVRFDKGTransportKeyBindingV1{artifacts[0].Binding, artifacts[1].Binding}
	context.CanonicalTransportEnvelopes = []lqc.RabbitVRFDKGEnvelopeV1{artifacts[0].Envelope, artifacts[1].Envelope}
	context.PolynomialCommitments = []lqc.RabbitVRFDKGPolynomialCommitmentV1{commitment}
	context.Members = []lqc.RabbitVRFCommitteeMemberV1{dealer}
	runtime.current = context // Fixture installs already-authenticated session artifacts.
	manager := accounts.NewManager(&accounts.Config{})
	t.Cleanup(func() { manager.Close() })
	instance := filepath.Join(t.TempDir(), "secret-store-must-remain-absent")
	runtime.backend = &Ethereum{
		accountManager: manager, config: &ethconfig.Config{},
		vrfDKGInstanceDir: instance, vrfDKGTransport: transport,
	}
	runtime.cacheDKGEvaluationV1(context.SessionID, packet)
	if !runtime.markDKGFinalizedV1(context.SessionID) {
		t.Fatal("failed to mark test session finalized")
	}
	cached, ok := runtime.cachedDKGEvaluationV1(context.SessionID, dealer.ShareID, recipient.ShareID)
	if !ok {
		t.Fatal("outbound ciphertext missing")
	}
	cached.Ciphertext[0] ^= 1
	cached.Signature[0] ^= 1
	stillCached, ok := runtime.cachedDKGEvaluationV1(context.SessionID, dealer.ShareID, recipient.ShareID)
	if !ok || !reflect.DeepEqual(stillCached, packet) {
		t.Fatal("cache exposed mutable ciphertext/signature")
	}

	left, right := p2p.MsgPipe()
	t.Cleanup(func() { left.Close(); right.Close() })
	peer := &rabbitVRFDKGPeer{peer: p2p.NewPeer(enode.ID{9}, "recipient", nil), rw: left}
	transport.peers[peer.id()] = peer
	transport.routeSession = context.SessionID
	transport.routes[recipient.ShareID] = peer.id()
	for attempt := 0; attempt < 2; attempt++ {
		runtime.mu.Lock()
		runtime.evalLastSent = nil
		runtime.evalDispatchSession = [32]byte{}
		runtime.mu.Unlock()
		sent := make(chan error, 1)
		go func() { sent <- runtime.progressDKGV1() }()
		received := make(chan error, 1)
		go func() {
			message, err := right.ReadMsg()
			if err != nil {
				received <- err
				return
			}
			defer message.Discard()
			if message.Code != rabbitVRFDKGEncryptedEvaluationMsg {
				received <- errors.New("wrong replay message code")
				return
			}
			var got lqc.RabbitVRFDKGEncryptedEvaluationV1
			if err := message.Decode(&got); err != nil {
				received <- err
				return
			}
			if !reflect.DeepEqual(got, packet) {
				received <- errors.New("encrypted evaluation changed on replay")
				return
			}
			received <- lqc.VerifyRabbitVRFDKGEncryptedEvaluationSignatureV1(context.CanonicalSession, dealer, got)
		}()
		select {
		case err := <-received:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("lagging recipient did not receive replay after local finalization")
		}
		select {
		case err := <-sent:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(time.Second):
			t.Fatal("finalized replay blocked")
		}
	}
	if _, err := os.Stat(instance); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("finalized DKG touched secret storage: %v", err)
	}
	next := context
	next.SessionID[0] ^= 1
	runtime.setCurrent(next)
	if _, ok := runtime.cachedDKGEvaluationV1(context.SessionID, dealer.ShareID, recipient.ShareID); ok {
		t.Fatal("old ciphertext reused across sessions")
	}
	if runtime.dkgAlreadyFinalizedV1(next.SessionID) {
		t.Fatal("new session incorrectly considered finalized")
	}
}
