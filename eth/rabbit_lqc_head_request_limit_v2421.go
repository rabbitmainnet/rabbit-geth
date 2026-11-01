package eth

import (
	"sync"

	"github.com/ethereum/go-ethereum/common"
	ethproto "github.com/ethereum/go-ethereum/eth/protocols/eth"
)

// Limits concurrent LQC header probes. A peer can have only one active
// probe, preventing duplicate requests during repeated range announcements.
type lqcHeadProbeLimiter struct {
	mu       sync.Mutex
	active   map[*ethproto.Peer]struct{}
	capacity int
}

func (l *lqcHeadProbeLimiter) acquire(peer *ethproto.Peer) bool {
	if peer == nil {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.active == nil {
		l.active = make(map[*ethproto.Peer]struct{})
	}
	if _, exists := l.active[peer]; exists {
		return false
	}
	if len(l.active) >= l.capacity {
		return false
	}
	l.active[peer] = struct{}{}
	return true
}

func (l *lqcHeadProbeLimiter) release(peer *ethproto.Peer) {
	l.mu.Lock()
	delete(l.active, peer)
	l.mu.Unlock()
}

var rabbitLQCHeadProbesV2421 = lqcHeadProbeLimiter{
	capacity: 8,
}

func (h *ethHandler) requestLQCHeadBounded(
	peer *ethproto.Peer,
	number uint64,
	hash common.Hash,
) {
	if !rabbitLQCHeadProbesV2421.acquire(peer) {
		return
	}

	go func() {
		defer rabbitLQCHeadProbesV2421.release(peer)
		h.requestAndSyncLQCHead(peer, number, hash)
	}()
}
