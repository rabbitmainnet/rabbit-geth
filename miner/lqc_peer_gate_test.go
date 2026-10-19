package miner

import "testing"

type lqcPeerGateStub struct {
	peers int
}

func (b *lqcPeerGateStub) PeerCount() int {
	return b.peers
}

func TestLQCProductionStopsAtZeroPeers(t *testing.T) {
	backend := &lqcPeerGateStub{peers: 1}

	if !lqcHasConnectedPeer(backend) {
		t.Fatal("one connected peer must allow the low-level peer gate")
	}

	backend.peers = 0

	if lqcHasConnectedPeer(backend) {
		t.Fatal("producer must be blocked when peer count drops to zero")
	}

	backend.peers = 1

	if !lqcHasConnectedPeer(backend) {
		t.Fatal("restored peer connection must be visible to the peer gate")
	}
}

func TestLQCProductionPeerGateFailsClosed(t *testing.T) {
	if lqcHasConnectedPeer(struct{}{}) {
		t.Fatal("backend without PeerCount must fail closed")
	}

	backend := &lqcPeerGateStub{peers: -1}
	if lqcHasConnectedPeer(backend) {
		t.Fatal("non-positive peer count must block production")
	}
}
