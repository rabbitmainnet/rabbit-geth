package main

import (
	"testing"
	"time"
)

func TestReadinessUnknownSyncTargetRecovery(t *testing.T) {
	for _, tc := range []struct {
		name    string
		height  uint64
		peers   uint64
		fresh   bool
		syncing bool
		target  uint64
		elapsed time.Duration
		want    bool
	}{
		{"advancing-live-head", 101, 3, true, false, 0, syncDiscoveryGrace + time.Second, true},
		{"no-advance", 100, 3, true, false, 0, syncDiscoveryGrace + time.Second, false},
		{"no-peers", 101, 0, true, false, 0, syncDiscoveryGrace + time.Second, false},
		{"stale-head", 101, 3, false, false, 0, syncDiscoveryGrace + time.Second, false},
		{"still-syncing", 101, 3, true, true, 0, syncDiscoveryGrace + time.Second, false},
		{"discovery-not-finished", 101, 3, true, false, 0, 0, false},
		{"known-target-not-reached", 101, 3, true, false, 200, syncDiscoveryGrace + time.Second, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Unix(1000, 0)
			tracker := newReadinessTracker(now)
			if ready, _ := tracker.observe(networkTelemetry{
				Height: 99, Peers: 3, Syncing: true, SyncHighest: tc.target,
			}, now); ready {
				t.Fatal("active synchronization accepted")
			}
			ended := now.Add(time.Second)
			if ready, _ := tracker.observe(networkTelemetry{
				Height: 100, Peers: 3, HeadFresh: true,
			}, ended); ready {
				t.Fatal("recovery accepted without confirmation")
			}
			ready, reason := tracker.observe(networkTelemetry{
				Height: tc.height, Peers: tc.peers,
				HeadFresh: tc.fresh, Syncing: tc.syncing,
			}, ended.Add(tc.elapsed))
			if ready != tc.want {
				t.Fatalf("ready=%v want=%v reason=%s", ready, tc.want, reason)
			}
		})
	}
}
