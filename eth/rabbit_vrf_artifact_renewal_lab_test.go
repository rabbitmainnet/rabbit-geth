//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"reflect"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/p2p"
)

func TestRabbitVRFDKGEstablishedPeerArtifactRetryV1(t *testing.T) {
	transport, packet := newRabbitVRFDKGTransportArtifactFixtureV1(t)
	if inserted, err := transport.storeRemoteTransportArtifactV1(packet); err != nil || !inserted {
		t.Fatalf("retain artifact: inserted=%v err=%v", inserted, err)
	}
	transport.runtime.mu.Lock()
	transport.runtime.current.Members = nil
	transport.runtime.current.TransportBindings = nil
	transport.runtime.current.TransportEnvelopes = nil
	transport.runtime.mu.Unlock()

	left, right := p2p.MsgPipe()
	t.Cleanup(func() {
		left.Close()
		right.Close()
	})
	peer := &rabbitVRFDKGPeer{rw: left}
	done := make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-done:
		default:
			close(done)
		}
	})
	sent := make(chan error, 1)
	go func() {
		sent <- transport.syncPeerArtifactsUntilDoneV1(peer, done, 20*time.Millisecond)
	}()

	received := make(chan error, 1)
	go func() {
		for attempt := 0; attempt < 2; attempt++ {
			message, err := right.ReadMsg()
			if err != nil {
				received <- err
				return
			}
			if message.Code != rabbitVRFDKGTransportArtifactMsg {
				message.Discard()
				received <- errRabbitVRFDKGCanonicalTransportKeySetV1
				return
			}
			var got rabbitVRFDKGTransportArtifactPacketV1
			err = message.Decode(&got)
			message.Discard()
			if err != nil {
				received <- err
				return
			}
			if !reflect.DeepEqual(got, packet) {
				received <- errRabbitVRFDKGArtifactConflict
				return
			}
		}
		received <- nil
	}()

	select {
	case err := <-received:
		if err != nil {
			t.Fatal(err)
		}
	case err := <-sent:
		t.Fatalf("renewal stopped before retry: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("established peer did not receive artifact retry")
	}

	close(done)
	select {
	case err := <-sent:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("renewal worker did not stop")
	}
}
