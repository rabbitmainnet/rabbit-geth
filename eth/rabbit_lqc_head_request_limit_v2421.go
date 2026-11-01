package eth

import (
	"sync"

	"github.com/ethereum/go-ethereum/common"
	ethproto "github.com/ethereum/go-ethereum/eth/protocols/eth"
)

type lqcHeadProbeTarget struct {
	number uint64
	hash   common.Hash
}

type lqcHeadProbeLimiter struct {
	mu      sync.Mutex
	busy    map[*ethproto.Peer]bool
	pending map[*ethproto.Peer]lqcHeadProbeTarget
	slots   chan struct{}
}

func newLQCHeadProbeLimiter(capacity int) *lqcHeadProbeLimiter {
	if capacity < 1 {
		capacity = 1
	}
	return &lqcHeadProbeLimiter{
		busy:    make(map[*ethproto.Peer]bool),
		pending: make(map[*ethproto.Peer]lqcHeadProbeTarget),
		slots:   make(chan struct{}, capacity),
	}
}

func (l *lqcHeadProbeLimiter) submit(
	peer *ethproto.Peer, number uint64, hash common.Hash,
) bool {
	if peer == nil || hash == (common.Hash{}) {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	old, exists := l.pending[peer]
	if !exists || number > old.number ||
		(number == old.number && hash != old.hash) {
		l.pending[peer] = lqcHeadProbeTarget{number, hash}
	}
	if l.busy[peer] {
		return false
	}
	l.busy[peer] = true
	return true
}

func (l *lqcHeadProbeLimiter) next(peer *ethproto.Peer) (
	lqcHeadProbeTarget, bool,
) {
	l.mu.Lock()
	defer l.mu.Unlock()

	target, ok := l.pending[peer]
	if ok {
		delete(l.pending, peer)
	} else {
		delete(l.busy, peer)
	}
	return target, ok
}

var rabbitLQCHeadProbesV2421 = newLQCHeadProbeLimiter(8)

func (h *ethHandler) requestLQCHeadBounded(
	peer *ethproto.Peer, number uint64, hash common.Hash,
) {
	l := rabbitLQCHeadProbesV2421
	if !l.submit(peer, number, hash) {
		return
	}
	go func() {
		for {
			// Every submitted target remains pending until this worker
			// can process it. Only one worker exists per peer.
			l.slots <- struct{}{}
			target, ok := l.next(peer)
			if !ok {
				<-l.slots
				return
			}
			if shouldStartLQCSync(
				h.chain.CurrentBlock(), target.number, target.hash,
			) {
				h.requestAndSyncLQCHead(
					peer, target.number, target.hash,
				)
			}
			<-l.slots
		}
	}()
}
