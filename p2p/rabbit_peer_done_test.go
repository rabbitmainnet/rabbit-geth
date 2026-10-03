package p2p

import (
	"sync"
	"testing"

	"github.com/ethereum/go-ethereum/p2p/enode"
)

func TestRabbitPeerDoneLifecycle(t *testing.T) {
	for _, withPipe := range []bool{false, true} {
		name := "plain"
		if withPipe {
			name = "pipe"
		}
		t.Run(name, func(t *testing.T) {
			var peer *Peer
			if withPipe {
				pipe, other := MsgPipe()
				defer pipe.Close()
				defer other.Close()
				peer = NewPeerPipe(enode.ID{1}, "", nil, pipe)
			} else {
				peer = NewPeer(enode.ID{1}, "", nil)
			}
			select {
			case <-peer.Done():
				t.Fatal("test peer reports shutdown before Disconnect")
			default:
			}
			var wg sync.WaitGroup
			for i := 0; i < 4; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					peer.Disconnect(DiscRequested)
				}()
			}
			wg.Wait()
			select {
			case <-peer.Done():
			default:
				t.Fatal("Disconnect did not signal shutdown")
			}
		})
	}
	t.Run("productionSignal", func(t *testing.T) {
		peer := &Peer{closed: make(chan struct{})}
		if peer.Done() != peer.closed {
			t.Fatal("production shutdown channel changed")
		}
		select {
		case <-peer.Done():
			t.Fatal("open production peer reports shutdown")
		default:
		}
		close(peer.closed)
		select {
		case <-peer.Done():
		default:
			t.Fatal("production shutdown signal lost")
		}
	})
}
