package downloader

import (
	"fmt"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/eth/protocols/eth"
)

func TestRabbitLQCSlowPeersRecovery(t *testing.T) {
	success := make(chan struct{}, 1)

	tester := newTesterWithNotification(t, FullSync, func() {
		select {
		case success <- struct{}{}:
		default:
		}
	})
	defer tester.terminate()

	chain := testChainBase.shorten(blockCacheMaxItems - 15)
	blocks := chain.blocks[1:]

	// 25 peers: 15 healthy, 5 slow, 5 silent.
	for i := 0; i < 25; i++ {
		peer := tester.newPeer(
			fmt.Sprintf("peer-%02d", i),
			eth.ETH69,
			blocks,
		)
		switch {
		case i < 5:
			peer.headerDelay = 2 * time.Second
		case i < 10:
			peer.dropHeaders = true
		}
	}

	target := chain.blocks[len(chain.blocks)-1]

	if err := tester.downloader.BeaconSync(target.Header(), nil); err != nil {
		t.Fatal(err)
	}

	select {
	case <-success:
	case <-time.After(45 * time.Second):
		current := tester.chain.CurrentBlock()
		t.Fatalf("recovery failed with slow and silent peers: current=%d target=%d", current.Number.Uint64(), target.NumberU64())
	}

	if got, want := tester.chain.CurrentBlock().Number.Uint64(), target.NumberU64(); got != want {
		t.Fatalf("wrong head: got %d, want %d", got, want)
	}
}
