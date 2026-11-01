package eth

import (
	"sync"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	ethproto "github.com/ethereum/go-ethereum/eth/protocols/eth"
)

func TestLQCHeadProbeQueueV2421(t *testing.T) {
	l := newLQCHeadProbeLimiter(8)
	p := new(ethproto.Peer)
	h1 := common.HexToHash("0x01")
	h2 := common.HexToHash("0x02")

	if !l.submit(p, 359613, h1) {
		t.Fatal("first worker not created")
	}
	if l.submit(p, 359633, h2) {
		t.Fatal("duplicate worker created")
	}
	target, ok := l.next(p)
	if !ok || target.number != 359633 || target.hash != h2 {
		t.Fatal("latest announced target was lost")
	}
	if _, ok := l.next(p); ok {
		t.Fatal("unexpected pending target")
	}
	if !l.submit(p, 359634, h1) {
		t.Fatal("peer failed to restart after draining")
	}
}

func TestLQCHeadProbeConcurrencyV2421(t *testing.T) {
	l := newLQCHeadProbeLimiter(8)
	var wg sync.WaitGroup
	var mu sync.Mutex
	active, peak := 0, 0

	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			l.slots <- struct{}{}
			mu.Lock()
			active++
			if active > peak {
				peak = active
			}
			mu.Unlock()
			mu.Lock()
			active--
			mu.Unlock()
			<-l.slots
		}()
	}
	wg.Wait()
	if peak > 8 {
		t.Fatalf("too many concurrent probes: %d", peak)
	}
}

func TestLQCHeadProbeDropDisconnectedPeerV2421(t *testing.T) {
	l := newLQCHeadProbeLimiter(8)
	peer := new(ethproto.Peer)

	if !l.submit(peer, 359613, common.HexToHash("0x01")) {
		t.Fatal("worker inicial nao criado")
	}
	if l.submit(peer, 359633, common.HexToHash("0x02")) {
		t.Fatal("worker duplicado")
	}

	l.drop(peer)

	if _, ok := l.next(peer); ok {
		t.Fatal("pedido pendente sobreviveu a desconexao")
	}
	if !l.submit(peer, 359634, common.HexToHash("0x03")) {
		t.Fatal("worker nao foi liberado")
	}
}
