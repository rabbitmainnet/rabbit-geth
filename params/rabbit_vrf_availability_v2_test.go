package params

import "testing"

func TestVRFAvailabilityV2Compatibility(t *testing.T) {
	old := &ChainConfig{LQC: &LQCConfig{}}
	next := &ChainConfig{LQC: &LQCConfig{VRFAvailabilityV2Block: 200001}}
	if err := old.CheckCompatible(next, 200000, 0); err != nil {
		t.Fatal(err)
	}
	for _, height := range []uint64{200001, 200002} {
		err := old.CheckCompatible(next, height, 0)
		if err == nil || err.What != "LQC VRF availability V2 fork block" {
			t.Fatalf("unexpected compatibility: %v", err)
		}
		if err.RewindToBlock != 200000 {
			t.Fatal("wrong rewind boundary")
		}
		if err := next.CheckCompatible(old, height, 0); err == nil {
			t.Fatal("activated schedule removed")
		}
	}
}

func TestVRFAvailabilityV2Schedule(t *testing.T) {
	for _, tc := range []struct {
		name                       string
		fork, epoch, window, grace uint64
		valid                      bool
	}{
		{"disabled", 0, 0, 0, 0, true}, {"aligned", 1025, 128, 64, 16, true},
		{"unaligned", 1026, 128, 64, 16, false}, {"no-epoch", 1025, 0, 64, 16, false},
		{"no-window", 1025, 128, 0, 16, false}, {"overflow", 1025, 128, 1, ^uint64(0), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &LQCConfig{RegistryProtocolBlock: 1, VRFProtocolBlock: 513, VRFAvailabilityV2Block: tc.fork, EpochLength: tc.epoch, HeartbeatWindow: tc.window, HeartbeatGrace: tc.grace}
			if err := c.validateVRFAvailabilityV2(); (err == nil) != tc.valid {
				t.Fatalf("unexpected validation: %v", err)
			}
		})
	}
	c := &LQCConfig{VRFAvailabilityV2Block: 1025}
	if c.IsVRFAvailabilityV2Preparation(1024) || !c.IsVRFAvailabilityV2Preparation(1025) {
		t.Fatal("wrong preparation gate")
	}
	var disabled *LQCConfig
	if disabled.IsVRFAvailabilityV2Preparation(1025) {
		t.Fatal("nil enabled")
	}
}
