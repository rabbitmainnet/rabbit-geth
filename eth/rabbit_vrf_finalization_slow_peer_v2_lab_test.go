//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/p2p"
)

func TestRabbitVRFSlowPeerDoesNotBlockOtherPeerV2(t *testing.T) {
	slowSender, slowReceiver := p2p.MsgPipe()
	fastSender, fastReceiver := p2p.MsgPipe()

	defer slowSender.Close()
	defer slowReceiver.Close()
	defer fastSender.Close()
	defer fastReceiver.Close()

	slowPeer := &rabbitVRFDKGPeer{
		rw:      slowSender,
		version: rabbitVRFDKGProtocolVersionV2,
	}
	fastPeer := &rabbitVRFDKGPeer{
		rw:      fastSender,
		version: rabbitVRFDKGProtocolVersionV2,
	}

	proof := lqc.RabbitVRFFinalizationV1{
		Version:   lqc.RabbitVRFFinalizationVersionV1,
		ProofHash: common.HexToHash("0x123456"),
	}

	slowDone := make(chan error, 1)
	go func() {
		slowDone <- slowPeer.sendVerifiedFinalizationV2(proof)
	}()

	// No reader exists on the slow connection.
	select {
	case err := <-slowDone:
		t.Fatalf("slow peer unexpectedly finished: %v", err)
	case <-time.After(30 * time.Millisecond):
	}

	fastDone := make(chan error, 1)
	go func() {
		fastDone <- fastPeer.sendVerifiedFinalizationV2(proof)
	}()

	received := make(chan error, 1)
	go func() {
		msg, err := fastReceiver.ReadMsg()
		if err != nil {
			received <- err
			return
		}
		defer msg.Discard()

		var decoded lqc.RabbitVRFFinalizationV1
		if err := msg.Decode(&decoded); err != nil {
			received <- err
			return
		}
		if decoded.ProofHash != proof.ProofHash {
			received <- errRabbitVRFDKGArtifactSessionMismatch
			return
		}
		received <- nil
	}()

	select {
	case err := <-received:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("healthy peer blocked by slow peer")
	}

	select {
	case err := <-fastDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("healthy peer send did not finish")
	}

	// Disconnect the stalled peer and verify its sender exits.
	slowReceiver.Close()

	select {
	case <-slowDone:
	case <-time.After(time.Second):
		t.Fatal("stalled sender did not exit after disconnect")
	}
}
