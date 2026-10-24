//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/p2p"
)

// The first delivery reaches a peer before it has adopted the session. A later
// delivery over the same connection must succeed despite sender-side known hashes.
func TestRabbitVRFDKGArtifactReplayAfterSessionLag(t *testing.T) {
	transport, packet := newRabbitVRFDKGTransportArtifactFixtureV1(t)
	if _, err := transport.storeRemoteTransportArtifactV1(packet); err != nil {
		t.Fatal(err)
	}
	context := transport.runtime.currentContext()
	receiver, _ := newRabbitVRFDKGTransportArtifactFixtureV1(t)
	receiver.runtime.clearCurrent()
	left, right := p2p.MsgPipe()
	stop := make(chan struct{})
	done := make(chan struct{})
	t.Cleanup(func() {
		close(stop)
		left.Close()
		right.Close()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("artifact gossip did not stop")
		}
	})
	peer := &rabbitVRFDKGPeer{rw: left}
	go func() {
		defer close(done)
		transport.gossipDKGArtifactsToPeerV1(peer, stop)
	}()
	result := make(chan error, 1)
	go func() {
		for delivery := 0; delivery < 2; delivery++ {
			msg, err := right.ReadMsg()
			if err != nil {
				result <- err
				return
			}
			var got rabbitVRFDKGTransportArtifactPacketV1
			if msg.Code != rabbitVRFDKGTransportArtifactMsg {
				msg.Discard()
				result <- errors.New("unexpected replay message")
				return
			}
			err = msg.Decode(&got)
			msg.Discard()
			if err != nil {
				result <- err
				return
			}
			if !reflect.DeepEqual(got, packet) {
				result <- errors.New("artifact changed during replay")
				return
			}
			inserted, err := receiver.storeRemoteTransportArtifactV1(got)
			if delivery == 0 {
				if !errors.Is(err, errRabbitVRFDKGArtifactSessionMismatch) || inserted {
					result <- errors.New("future session artifact was not rejected")
					return
				}
				receiver.runtime.setCurrent(context)
			} else if err != nil || !inserted {
				result <- errors.New("authenticated artifact was not accepted after session catch-up")
				return
			}
		}
		result <- nil
	}()
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("artifact was not replayed on the existing connection")
	}
}
