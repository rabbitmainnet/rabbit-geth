package miner

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestLQCProducerStopWaitsForWorkerBeforeRestart(t *testing.T) {
	m := &Miner{}
	entered := make(chan struct{})
	cancelled := make(chan struct{})
	release := make(chan struct{})
	var active atomic.Int32
	m.lqcMu.Lock()
	m.startLQCDevnetLocked(func(ctx context.Context) {
		active.Add(1)
		defer active.Add(-1)
		close(entered)
		<-ctx.Done()
		close(cancelled)
		<-release
	})
	firstDone := m.lqcDone
	m.lqcMu.Unlock()
	<-entered
	stopped := make(chan struct{})
	go func() { m.StopLQCDevnet(); close(stopped) }()
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("stop did not cancel worker")
	}
	select {
	case <-stopped:
		t.Fatal("Stop returned while worker was still active")
	default:
	}
	restarted := make(chan struct{})
	overlap := make(chan bool, 1)
	go func() {
		m.lqcMu.Lock()
		m.startLQCDevnetLocked(func(ctx context.Context) {
			overlap <- active.Add(1) != 1
			defer active.Add(-1)
			<-ctx.Done()
		})
		m.lqcMu.Unlock()
		close(restarted)
	}()
	close(release)
	select {
	case <-restarted:
	case <-time.After(time.Second):
		t.Fatal("restart did not complete after old worker exited")
	}
	<-stopped
	select {
	case <-firstDone:
	default:
		t.Fatal("restart preceded old worker completion")
	}
	if <-overlap {
		t.Error("producer workers overlapped")
	}
	m.StopLQCDevnet()
	if active.Load() != 0 {
		t.Fatal("producer remained active after stop")
	}
}

func TestLQCProducerRepeatedStopRestart(t *testing.T) {
	m := &Miner{}
	for i := 0; i < 25; i++ {
		entered := make(chan struct{})
		m.lqcMu.Lock()
		m.startLQCDevnetLocked(func(ctx context.Context) {
			close(entered)
			<-ctx.Done()
		})
		done := m.lqcDone
		m.lqcMu.Unlock()
		<-entered
		m.StopLQCDevnet()
		m.StopLQCDevnet()
		select {
		case <-done:
		default:
			t.Fatal("worker did not stop")
		}
	}
}
