package main

import (
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
)

func TestObservedHeadFresh(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	block := observedBlock{Number: hexutil.Uint64(10), Timestamp: hexutil.Uint64(now.Add(-time.Minute).Unix())}
	if !observedHeadFresh(block, 10, now) {
		t.Fatal("recent canonical head should be fresh")
	}
	if observedHeadFresh(block, 11, now) {
		t.Fatal("mismatched latest block number must not be fresh")
	}
	block.Timestamp = hexutil.Uint64(now.Add(-headFreshness - time.Second).Unix())
	if observedHeadFresh(block, 10, now) {
		t.Fatal("stale head must not be fresh")
	}
	block.Timestamp = hexutil.Uint64(now.Add(headFutureTolerance + time.Second).Unix())
	if observedHeadFresh(block, 10, now) {
		t.Fatal("head too far in the future must not be fresh")
	}
}

func TestReadinessTrackerBlocksPartialFreshSync(t *testing.T) {
	start := time.Unix(1_800_000_000, 0)
	tracker := newReadinessTracker(start)

	ready, _ := tracker.observe(networkTelemetry{
		Height:      100,
		Peers:       2,
		Syncing:     true,
		SyncCurrent: 100,
		SyncHighest: 1000,
	}, start.Add(5*time.Second))
	if ready {
		t.Fatal("active sync must block Work V2")
	}

	ready, _ = tracker.observe(networkTelemetry{
		Height:    640,
		Peers:     0,
		HeadFresh: false,
	}, start.Add(2*time.Minute))
	if ready {
		t.Fatal("partial sync must remain blocked after eth_syncing becomes false")
	}

	ready, _ = tracker.observe(networkTelemetry{
		Height:    1000,
		Peers:     2,
		HeadFresh: false,
	}, start.Add(3*time.Minute))
	if !ready {
		t.Fatal("reaching the observed canonical target must unlock mining")
	}
}

func TestReadinessTrackerDiscoveryAndOfflineRecovery(t *testing.T) {
	start := time.Unix(1_800_000_000, 0)

	live := newReadinessTracker(start)
	ready, _ := live.observe(networkTelemetry{Height: 50, Peers: 2, HeadFresh: true}, start.Add(syncDiscoveryGrace-time.Second))
	if ready {
		t.Fatal("live head must still honor peer discovery grace")
	}
	ready, _ = live.observe(networkTelemetry{Height: 50, Peers: 2, HeadFresh: true}, start.Add(syncDiscoveryGrace))
	if !ready {
		t.Fatal("live head with peers should unlock after discovery grace")
	}

	offline := newReadinessTracker(start)
	ready, _ = offline.observe(networkTelemetry{Height: 500, Peers: 0, HeadFresh: false}, start)
	if ready {
		t.Fatal("stale isolated head must not unlock immediately")
	}
	ready, _ = offline.observe(networkTelemetry{Height: 500, Peers: 0, HeadFresh: false}, start.Add(offlineReadinessGrace-time.Second))
	if ready {
		t.Fatal("stale isolated head must wait for the full stable offline recovery grace")
	}
	ready, _ = offline.observe(networkTelemetry{Height: 500, Peers: 0, HeadFresh: false}, start.Add(offlineReadinessGrace))
	if !ready {
		t.Fatal("stale isolated head should unlock after the full stable offline recovery grace")
	}
}

func TestReadinessTrackerResetsTargetForNewSyncCycle(t *testing.T) {
	start := time.Unix(1_800_000_000, 0)
	tracker := newReadinessTracker(start)

	tracker.observe(networkTelemetry{Height: 900, Peers: 2, Syncing: true, SyncCurrent: 900, SyncHighest: 1000}, start.Add(time.Second))
	ready, _ := tracker.observe(networkTelemetry{Height: 1000, Peers: 2}, start.Add(2*time.Second))
	if !ready {
		t.Fatal("first sync cycle should complete at target 1000")
	}

	ready, _ = tracker.observe(networkTelemetry{Height: 850, Peers: 2, Syncing: true, SyncCurrent: 850, SyncHighest: 900}, start.Add(3*time.Second))
	if ready {
		t.Fatal("new sync cycle must block mining")
	}
	ready, _ = tracker.observe(networkTelemetry{Height: 900, Peers: 2}, start.Add(4*time.Second))
	if !ready {
		t.Fatal("new sync cycle should use its own target instead of the previous higher target")
	}
}

func TestReadinessTrackerFreshGenesisNeverUsesOfflineRecovery(t *testing.T) {
	start := time.Unix(1_800_000_000, 0)
	tracker := newReadinessTracker(start)

	ready, _ := tracker.observe(networkTelemetry{
		Height:    0,
		Peers:     0,
		HeadFresh: false,
	}, start)
	if ready {
		t.Fatal("fresh genesis must start with mining blocked")
	}

	ready, _ = tracker.observe(networkTelemetry{
		Height:    0,
		Peers:     0,
		HeadFresh: false,
	}, start.Add(10*offlineReadinessGrace))
	if ready {
		t.Fatal("fresh genesis must remain blocked even long after offline recovery grace")
	}
}
