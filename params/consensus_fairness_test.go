package params

import (
	"math/big"
	"testing"
)

func TestConsensusFairnessConfigCompatibility(t *testing.T) {
	config := func(activation uint64) *ChainConfig {
		return &ChainConfig{
			LQC: &LQCConfig{
				ConsensusFairnessBlock: activation,
			},
		}
	}

	fork := config(100)

	if fork.IsConsensusFairness(big.NewInt(99)) {
		t.Fatal("consensus fairness active before block 100")
	}
	if !fork.IsConsensusFairness(big.NewInt(100)) {
		t.Fatal("consensus fairness inactive at block 100")
	}
	if !fork.IsConsensusFairness(big.NewInt(101)) {
		t.Fatal("consensus fairness inactive after block 100")
	}

	stored := config(0)
	upgraded := config(100)

	if err := stored.CheckCompatible(upgraded, 99, 0); err != nil {
		t.Fatalf("future fairness fork rejected before activation: %v", err)
	}

	if err := stored.CheckCompatible(upgraded, 100, 0); err == nil {
		t.Fatal("fairness fork change accepted at activation height")
	}
}

func TestRabbitMainnetHasNoConsensusFairnessFork(t *testing.T) {
	if RabbitChainConfig.LQC != nil &&
		RabbitChainConfig.LQC.ConsensusFairnessBlock != 0 {
		t.Fatalf(
			"Rabbit Mainnet inherited Fairness fork: %+v",
			RabbitChainConfig.LQC,
		)
	}
}
