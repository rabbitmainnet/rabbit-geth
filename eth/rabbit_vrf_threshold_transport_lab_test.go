package eth

import (
	"errors"
	"math/big"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	gethcrypto "github.com/ethereum/go-ethereum/crypto"
	rabbitvrf "github.com/ethereum/go-ethereum/crypto/rabbitvrf"
	"github.com/ethereum/go-ethereum/internal/rabbitvrfstate"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/enode"
)

func TestRabbitVRFDKGTransportV1WireThresholdPartialCollector(t *testing.T) {
	chainID := big.NewInt(9280)
	vrfEpoch := uint64(11)
	committeeRoot := gethcrypto.Keccak256Hash([]byte("rabbit-vrf-threshold-wire-committee"))

	context, err := lqc.NewRabbitVRFDKGSessionContextV1(
		chainID,
		vrfEpoch,
		committeeRoot,
		3,
	)
	if err != nil {
		t.Fatal(err)
	}
	if context.Threshold != 3 {
		t.Fatalf("threshold=%d want=3", context.Threshold)
	}

	sessionID, err := lqc.RabbitVRFDKGSessionIDV1(context)
	if err != nil {
		t.Fatal(err)
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

	verificationShares := make([]lqc.RabbitVRFVerificationShareV1, len(shares))
	for index, share := range shares {
		publicKey, err := share.PublicKey()
		if err != nil {
			t.Fatal(err)
		}
		verificationShares[index] = lqc.RabbitVRFVerificationShareV1{
			ShareID:   uint64(index + 1),
			PublicKey: publicKey,
		}
	}

	transcriptRoot := gethcrypto.Keccak256Hash([]byte("rabbit-vrf-threshold-wire-transcript"))
	keysetRoot, canonicalShares, err := lqc.RabbitVRFKeysetRootV1(
		chainID,
		vrfEpoch,
		committeeRoot,
		3,
		context.Threshold,
		thresholdPublicKey,
		transcriptRoot,
		verificationShares,
	)
	if err != nil {
		t.Fatal(err)
	}

	keyset := rabbitvrfstate.DKGFinalKeysetV1{
		KeysetRoot:         keysetRoot,
		ThresholdPublicKey: thresholdPublicKey,
		TranscriptRoot:     transcriptRoot,
		VerificationShares: canonicalShares,
	}

	instanceDir := t.TempDir()
	keysetStore, err := rabbitvrfstate.NewDKGFinalKeysetStoreV1(
		filepath.Join(instanceDir, "rabbit-vrf", "dkg-final-keysets"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := keysetStore.Store(context, keyset); err != nil {
		t.Fatal(err)
	}

	requestID := gethcrypto.Keccak256Hash([]byte("rabbit-vrf-threshold-wire-request"))

	runtime := &rabbitVRFDKGRuntime{
		backend: &Ethereum{
			vrfDKGInstanceDir: instanceDir,
		},
		current: rabbitVRFDKGLocalContextV1{
			SessionID:        sessionID,
			CanonicalSession: context,
		},
		canonicalRequestLookup: func(got common.Hash) (rabbitVRFCanonicalRequestV1, error) {
			if got != requestID {
				return rabbitVRFCanonicalRequestV1{}, errors.New("request not canonical")
			}
			return rabbitVRFCanonicalRequestV1{
				RequestID:    got,
				Requester:    common.Address{1},
				RequestBlock: 1,
				Status:       rabbitVRFRequestStatusPendingV1,
			}, nil
		},
	}

	receiver, err := newRabbitVRFDKGTransport(
		rabbitVRFDKGTransportConfig{
			ChainID:   new(big.Int).Set(chainID),
			NetworkID: 9280,
			Genesis:   gethcrypto.Keccak256Hash([]byte("rabbit-vrf-threshold-wire-genesis")),
			Runtime:   runtime,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	rw, result := startRabbitVRFDKGTransportArtifactWireV1(t, receiver)

	remotePeerID := enode.ID{9}.String()
	receiver.mu.Lock()
	receiver.routeSession = sessionID
	receiver.routes[1] = remotePeerID
	receiver.routes[2] = remotePeerID
	receiver.mu.Unlock()

	message, _, err := lqc.RabbitVRFThresholdMessageV1(
		context,
		keysetRoot,
		requestID,
	)
	if err != nil {
		t.Fatal(err)
	}

	partial1, _, err := rabbitVRFSignThresholdPartialWithKeysetV1(
		shares[0],
		1,
		keyset,
		message,
	)
	if err != nil {
		t.Fatal(err)
	}
	packet1, err := lqc.NewRabbitVRFThresholdPartialV1(
		context,
		keysetRoot,
		requestID,
		partial1,
	)
	if err != nil {
		t.Fatal(err)
	}

	nonCanonical := packet1
	nonCanonical.RequestID = gethcrypto.Keccak256Hash([]byte("rabbit-vrf-non-canonical-request"))
	if err := runtime.validateInboundThresholdPartialV1(nonCanonical); err == nil || !strings.Contains(err.Error(), "request not canonical") {
		t.Fatalf("non-canonical request was not rejected: %v", err)
	}

	if err := p2p.Send(rw, rabbitVRFDKGThresholdPartialMsg, packet1); err != nil {
		t.Fatal(err)
	}
	if err := p2p.Send(rw, rabbitVRFDKGThresholdPartialMsg, packet1); err != nil {
		t.Fatal(err)
	}

	// A verified inbound partial must survive process restart.
	partialStore, err := rabbitvrfstate.NewThresholdPartialStoreV1(
		filepath.Join(instanceDir, "rabbit-vrf", "threshold-partials"),
	)
	if err != nil {
		t.Fatal(err)
	}
	persistDeadline := time.Now().Add(2 * time.Second)
	for {
		persisted, loadErr := partialStore.Load(
			context,
			keysetRoot,
			requestID,
			packet1.ShareID,
		)
		if loadErr == nil {
			if persisted != packet1 {
				t.Fatal("persisted inbound threshold partial differs from canonical packet")
			}
			break
		}
		if time.Now().After(persistDeadline) {
			t.Fatalf("verified inbound threshold partial was not persisted: %v", loadErr)
		}
		time.Sleep(10 * time.Millisecond)
	}

	partial2, _, err := rabbitVRFSignThresholdPartialWithKeysetV1(
		shares[1],
		2,
		keyset,
		message,
	)
	if err != nil {
		t.Fatal(err)
	}
	packet2, err := lqc.NewRabbitVRFThresholdPartialV1(
		context,
		keysetRoot,
		requestID,
		partial2,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := p2p.Send(rw, rabbitVRFDKGThresholdPartialMsg, packet2); err != nil {
		t.Fatal(err)
	}

	partial3, _, err := rabbitVRFSignThresholdPartialWithKeysetV1(
		shares[2],
		3,
		keyset,
		message,
	)
	if err != nil {
		t.Fatal(err)
	}
	packet3, err := lqc.NewRabbitVRFThresholdPartialV1(
		context,
		keysetRoot,
		requestID,
		partial3,
	)
	if err != nil {
		t.Fatal(err)
	}
	receiver.mu.Lock()
	receiver.routes[3] = remotePeerID
	receiver.mu.Unlock()
	if err := p2p.Send(rw, rabbitVRFDKGThresholdPartialMsg, packet3); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(2 * time.Second)
	var final rabbitVRFThresholdResultV1
	for {
		receiver.mu.RLock()
		got, ok := receiver.partialResults[requestID]
		receiver.mu.RUnlock()
		if ok {
			final = got
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("threshold result was not produced")
		}
		time.Sleep(10 * time.Millisecond)
	}

	expectedRandomness, err := rabbitvrf.VerifyAndDeriveRandomness(
		thresholdPublicKey,
		message,
		final.Signature,
	)
	if err != nil {
		t.Fatal(err)
	}
	if final.Randomness == (common.Hash{}) {
		t.Fatal("zero threshold randomness")
	}
	if final.Randomness != expectedRandomness {
		t.Fatal("threshold wire randomness mismatch")
	}

	receiver.mu.Lock()
	conflicting := packet1
	conflicting.Signature[0] ^= 0x01
	receiver.partials[requestID][1] = conflicting
	receiver.mu.Unlock()

	if err := p2p.Send(rw, rabbitVRFDKGThresholdPartialMsg, packet1); err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-result:
		if err == nil || !strings.Contains(err.Error(), "threshold partial share conflict") {
			t.Fatalf("unexpected conflict result: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("threshold partial conflict wire test timed out")
	}
}
