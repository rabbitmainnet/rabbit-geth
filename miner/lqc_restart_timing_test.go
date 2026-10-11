package miner

import (
	"context"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/txpool"
)

type lqcRestartTimingBackend struct{ checks chan struct{} }

func (b *lqcRestartTimingBackend) BlockChain() *core.BlockChain      { return nil }
func (b *lqcRestartTimingBackend) TxPool() *txpool.TxPool            { return nil }
func (b *lqcRestartTimingBackend) AccountManager() *accounts.Manager { return nil }
func (b *lqcRestartTimingBackend) PeerCount() int {
	select {
	case b.checks <- struct{}{}:
	default:
	}
	return 0
}

func TestLQCProducerRestartChecksImmediately(t *testing.T) {
	b := &lqcRestartTimingBackend{checks: make(chan struct{}, 4)}
	m := &Miner{backend: b}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { defer close(done); m.lqcDevnetLoop(ctx) }()
	select {
	case <-b.checks:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("producer restart waited instead of checking readiness immediately")
	}
	select {
	case <-b.checks:
		t.Fatal("producer polled repeatedly without its interval")
	case <-time.After(100 * time.Millisecond):
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("producer did not stop after cancellation")
	}
}
