package eth

import (
	"testing"

	ethproto "github.com/ethereum/go-ethereum/eth/protocols/eth"
)

func TestLQCHeadProbeLimiterV2421(t *testing.T) {
	l := &lqcHeadProbeLimiter{capacity: 8}
	peers := make([]*ethproto.Peer, 9)

	for i := range peers {
		peers[i] = new(ethproto.Peer)
	}

	for i := 0; i < 8; i++ {
		if !l.acquire(peers[i]) {
			t.Fatalf("peer %d should acquire slot", i)
		}
		if l.acquire(peers[i]) {
			t.Fatalf("peer %d acquired duplicate slot", i)
		}
	}

	if l.acquire(peers[8]) {
		t.Fatal("concurrency limit exceeded")
	}

	l.release(peers[0])

	if !l.acquire(peers[8]) {
		t.Fatal("released slot was not reusable")
	}
}
