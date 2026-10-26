package params

import (
	"encoding/json"
	"math/big"
	"testing"
)

func TestWorkSeatLivenessV2Configuration(t *testing.T) {
	var absent *LQCConfig
	if absent.validateWorkSeatLivenessV2() != nil || absent.workSeatLivenessV2ForkBlock() != nil {
		t.Fatal("nil configuration")
	}
	cfg := &LQCConfig{WorkSeatAdmissionV2Block: 100, ConsensusLivenessV5Block: 50}
	if cfg.validateWorkSeatLivenessV2() != nil {
		t.Fatal("disabled gate")
	}
	cfg.WorkSeatLivenessV2Block = 100
	if cfg.validateWorkSeatLivenessV2() == nil {
		t.Fatal("overlapping activation accepted")
	}
	cfg.WorkSeatLivenessV2Block = 200
	if err := cfg.validateWorkSeatLivenessV2(); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var roundtrip LQCConfig
	if err = json.Unmarshal(data, &roundtrip); err != nil || roundtrip.WorkSeatLivenessV2Block != 200 {
		t.Fatal("configuration roundtrip")
	}
	old := &ChainConfig{ChainID: big.NewInt(9280), LQC: &LQCConfig{WorkSeatAdmissionV2Block: 100, ConsensusLivenessV5Block: 50}}
	next := &ChainConfig{ChainID: big.NewInt(9280), LQC: cfg}
	if err := old.CheckCompatible(next, 199, 0); err != nil {
		t.Fatalf("future activation rejected: %v", err)
	}
	if err := old.CheckCompatible(next, 200, 0); err == nil {
		t.Fatal("retroactive activation was accepted")
	}
}
