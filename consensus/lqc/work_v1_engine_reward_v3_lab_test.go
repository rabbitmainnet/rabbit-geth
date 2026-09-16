//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package lqc

import (
	"testing"

	"github.com/holiman/uint256"
)

func TestWorkV1EngineLabProducerRewardV3PaysFullOnlyWithoutCommittee(t *testing.T) {
	totalReward := uint256.NewInt(1200)

	withoutCommittee := workV1EngineLabProducerRewardForSelectionV3(
		totalReward,
		HybridSelection{},
	)
	if withoutCommittee.Uint64() != 1200 {
		t.Fatalf(
			"producer without committee reward=%d want=1200",
			withoutCommittee.Uint64(),
		)
	}

	withCommittee := workV1EngineLabProducerRewardForSelectionV3(
		totalReward,
		HybridSelection{
			Committee: make([]HybridParticipant, 1),
		},
	)
	if withCommittee.Uint64() != 840 {
		t.Fatalf(
			"producer with committee reward=%d want=840",
			withCommittee.Uint64(),
		)
	}

	if totalReward.Uint64() != 1200 {
		t.Fatalf(
			"input reward mutated: got=%d want=1200",
			totalReward.Uint64(),
		)
	}
}
