package params

import "testing"

func TestMiningRewardV5ConfigCompatibility(t *testing.T) {
	old := &ChainConfig{LQC: &LQCConfig{}}
	next := &ChainConfig{LQC: &LQCConfig{MiningRewardV5Block: 200000}}

	if err := old.CheckCompatible(next, 199999, 0); err != nil {
		t.Fatalf("future activation rejected: %v", err)
	}
	for _, height := range []uint64{200000, 200001} {
		err := old.CheckCompatible(next, height, 0)
		if err == nil || err.What != "LQC mining reward V5 fork block" {
			t.Fatalf("height %d: expected mining reward incompatibility, got %v", height, err)
		}
		if err.RewindToBlock != 199999 {
			t.Fatalf("rewind=%d want=199999", err.RewindToBlock)
		}
		if err := next.CheckCompatible(old, height, 0); err == nil {
			t.Fatal("removing activated fork accepted")
		}
	}
	if err := next.CheckCompatible(next, 200001, 0); err != nil {
		t.Fatalf("unchanged config rejected: %v", err)
	}
}

func TestMiningRewardV5ConfigValidation(t *testing.T) {
	cases := []struct {
		name   string
		config *LQCConfig
		valid  bool
	}{
		{"nil", nil, true},
		{"disabled", &LQCConfig{}, true},
		{"missingVRF", &LQCConfig{MiningRewardV5Block: 200000}, false},
		{"sameHeight", &LQCConfig{
			MiningRewardV5Block:      136193,
			VRFProtocolBlock:         136193,
			ConsensusLivenessV3Block: 77000,
		}, false},
		{"validFuture", &LQCConfig{
			MiningRewardV5Block:      200000,
			VRFProtocolBlock:         136193,
			ConsensusLivenessV3Block: 77000,
		}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.config.validateMiningRewardV5()
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v want=%v err=%v", err == nil, tc.valid, err)
			}
		})
	}
}
