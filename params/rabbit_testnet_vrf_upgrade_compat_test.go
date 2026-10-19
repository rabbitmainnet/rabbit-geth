package params

import "testing"

func TestRabbitTestnetVRFUpgradeCompatibilityWithPublicHistory(t *testing.T) {
	stored := &ChainConfig{
		LQC: &LQCConfig{
			EpochLength:              128,
			ConsensusLivenessV3Block: 77_000,
			ConsensusLivenessV4Block: 97_991,
			ConsensusLivenessV5Block: 115_000,
			ConsensusLivenessV6Block: 115_022,
		},
	}
	upgraded := &ChainConfig{
		LQC: &LQCConfig{
			EpochLength:              128,
			ConsensusLivenessV3Block: 77_000,
			ConsensusLivenessV4Block: 97_991,
			ConsensusLivenessV5Block: 115_000,
			ConsensusLivenessV6Block: 115_022,
			VRFProtocolBlock:         136_193,
		},
	}
	if err := stored.CheckCompatible(upgraded, 135_729, 0); err != nil {
		t.Fatalf("public Testnet history rejected before VRF activation: %v", err)
	}
	err := stored.CheckCompatible(upgraded, 136_193, 0)
	if err == nil {
		t.Fatal("late VRF upgrade was incorrectly accepted")
	}
	if err.What != "LQC VRF protocol fork block" {
		t.Fatalf("unexpected late-upgrade error: %+v", err)
	}
}
