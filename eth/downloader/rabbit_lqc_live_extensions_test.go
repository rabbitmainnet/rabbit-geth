package downloader

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/eth/protocols/eth"
)

func TestRabbitLQC25PeersLiveExtensions(t *testing.T) {
	tester := newTester(t, FullSync)
	defer tester.terminate()
	// Ativar a estrategia LCQ somente neste teste local.
	// Executado antes de BeaconSync e antes de iniciar tarefas.
	tester.downloader.skeleton.lqcHashAnchored = true
	t.Log("LAB LCQ HASH ANCHORED: true")
	t.Logf("LQC HASH ANCHORED: %v",
		tester.downloader.skeleton.lqcHashAnchored)

	// Apenas teste: limita a espera por peers silenciosos.
	if os.Getenv("RABBIT_FAST_TIMEOUT") == "1" {
		tester.downloader.peers.rates.OverrideTTLLimit = 3 * time.Second
	}
	t.Logf("TIMEOUT CONFIGURADO: %v",
		tester.downloader.peers.rates.OverrideTTLLimit)

	chain := testChainBase.shorten(blockCacheMaxItems - 15)
	blocks := chain.blocks[1:]

	for i := 0; i < 25; i++ {
		peer := tester.newPeer(fmt.Sprintf("peer-%02d", i), eth.ETH69, blocks)
		switch {
		case os.Getenv("RABBIT_ALL_HEALTHY") == "1":
			// Todos respondem normalmente.
		case i < 5:
			peer.headerDelay = 2 * time.Second
		case i < 10:
			peer.dropHeaders = true
		}
	}

	last := len(chain.blocks) - 1
	first := last - 5

	if err := tester.downloader.BeaconSync(
		chain.blocks[first].Header(), nil,
	); err != nil {
		t.Fatal("initial sync:", err)
	}

	// Confirmar que o primeiro alvo foi realmente importado.
	initialDeadline := time.Now().Add(30 * time.Second)
	for tester.chain.CurrentBlock().Number.Uint64() < uint64(first) {
		if time.Now().After(initialDeadline) {

			t.Log("=== INITIAL PEER DIAGNOSTIC ===")
			tester.lock.RLock()
			for id, peer := range tester.peers {
				n := peer.numberRequests.Load()
				h := peer.headerRequests.Load()
				if n > 0 || h > 0 {
					t.Logf("PEER %s | hash=%d number=%d", id, h, n)
				}
			}
			tester.lock.RUnlock()

			t.Fatalf("INITIAL SYNC STALLED: local=%d expected=%d",
				tester.chain.CurrentBlock().Number.Uint64(), first)
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Logf("INITIAL SYNC CONFIRMED: block %d", first)

	for n := first + 1; n <= last; n++ {
		time.Sleep(200 * time.Millisecond)

		err := tester.downloader.BeaconExtend(
			chain.blocks[n].Header(),
		)
		if err != nil {
			t.Fatalf("extension %d failed: %v", n, err)
		}
		t.Logf("new head announced: %d", n)
	}

	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		head := tester.chain.CurrentBlock()
		if head.Number.Uint64() == uint64(last) {
			if head.Hash() != chain.blocks[last].Hash() {
				t.Fatal("final canonical hash mismatch")
			}
			t.Logf("RECOVERED: block %d, canonical hash matches", last)
			return
		}
		time.Sleep(100 * time.Millisecond)
	}

	t.Log("=== REQUISICOES POR PEER ===")
	tester.lock.RLock()
	for id, peer := range tester.peers {
		t.Logf("PEER %s | headers por hash: %d",
			id, peer.headerRequests.Load())
		t.Logf("PEER %s | headers por numero: %d", id, peer.numberRequests.Load())
	}
	tester.lock.RUnlock()

	t.Fatalf("STALLED: local=%d expected=%d",
		tester.chain.CurrentBlock().Number.Uint64(), last)
}
