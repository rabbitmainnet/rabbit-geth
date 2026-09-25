//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"reflect"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/p2p"
)

func TestRabbitVRFDKGTransportV1NewPeerReceivesRetainedRemoteArtifact(
	t *testing.T,
) {
	transport, packet :=
		newRabbitVRFDKGTransportArtifactFixtureV1(t)

	inserted, err :=
		transport.storeRemoteTransportArtifactV1(packet)
	if err != nil {
		t.Fatal(err)
	}
	if !inserted {
		t.Fatal("retained remote artifact was not inserted")
	}

	transport.runtime.mu.Lock()
	transport.runtime.current.Members = nil
	transport.runtime.current.TransportBindings = nil
	transport.runtime.current.TransportEnvelopes = nil
	transport.runtime.mu.Unlock()

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
		sendResult <- transport.sendPendingTransportArtifactsV1(peer)
	}()

	readResult := make(
		chan rabbitVRFDKGTransportArtifactPacketV1,
		1,
	)
	readErr := make(chan error, 1)

	go func() {
		message, err := rightRW.ReadMsg()
		if err != nil {
			readErr <- err
			return
		}
		defer message.Discard()

		if message.Code != rabbitVRFDKGTransportArtifactMsg {
			readErr <- errRabbitVRFDKGCanonicalTransportKeySetV1
			return
		}

		var got rabbitVRFDKGTransportArtifactPacketV1
		if err := message.Decode(&got); err != nil {
			readErr <- err
			return
		}
		readResult <- got
	}()

	select {
	case err := <-readErr:
		t.Fatalf("retained remote artifact read failed: %v", err)
	case got := <-readResult:
		if !reflect.DeepEqual(got, packet) {
			t.Fatal("replayed retained remote artifact mismatch")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("retained remote artifact replay timed out")
	}

	select {
	case err := <-sendResult:
		if err != nil {
			t.Fatalf("retained remote artifact send failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("retained remote artifact sender did not finish")
	}
}
