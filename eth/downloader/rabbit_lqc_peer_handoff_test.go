package downloader

import (
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/eth/protocols/eth"
)

func TestRabbitLQCSilentPeerHandoff(t *testing.T) {
	tester := newTester(t, FullSync)
	defer tester.terminate()

	tester.downloader.skeleton.lqcHashAnchored = true
	// Testar com o timeout original de producao.
	// Nao alterar o timeout global.
	t.Logf("TTL ORIGINAL: %v",
		tester.downloader.peers.rates.OverrideTTLLimit)

	chain := testChainBase.shorten(blockCacheMaxItems - 15)
	blocks := chain.blocks[1:]
	target := chain.blocks[len(chain.blocks)-1]

	silent := tester.newPeer("silent", eth.ETH69, blocks)
	silent.dropHeaders = true

	if err := tester.downloader.BeaconSync(target.Header(), nil); err != nil {
		t.Fatalf("BeaconSync: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for silent.headerRequests.Load()+silent.numberRequests.Load() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("silent peer never received a header request")
		}
		time.Sleep(20 * time.Millisecond)
	}

	t.Log("CONFIRMED: silent peer received a header request")

	second := tester.newPeer("silent-2", eth.ETH69, blocks)
	second.headerDelay = 4 * time.Second
	healthy := tester.newPeer("healthy", eth.ETH69, blocks)
	t.Log("HEALTHY PEER CONNECTED BEFORE RECOVERY GRACE")
	t.Cleanup(func() {
		t.Logf("SECOND SILENT REQUESTS: %d",
			second.headerRequests.Load()+second.numberRequests.Load())
	})

	deadline = time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		head := tester.chain.CurrentBlock()

		if head.Number.Uint64() == target.NumberU64() {
			if head.Hash() != target.Hash() {
				t.Fatal("canonical hash mismatch")
			}
			t.Logf(
				"RECOVERED: block=%d silent_requests=%d healthy_requests=%d",
				head.Number.Uint64(),
				silent.headerRequests.Load()+silent.numberRequests.Load(),
				healthy.headerRequests.Load()+healthy.numberRequests.Load(),
			)
			return
		}
		time.Sleep(50 * time.Millisecond)
	}

	t.Fatalf(
		"STALLED: local=%d target=%d silent_requests=%d healthy_requests=%d",
		tester.chain.CurrentBlock().Number.Uint64(),
		target.NumberU64(),
		silent.headerRequests.Load()+silent.numberRequests.Load(),
		healthy.headerRequests.Load()+healthy.numberRequests.Load(),
	)
}
