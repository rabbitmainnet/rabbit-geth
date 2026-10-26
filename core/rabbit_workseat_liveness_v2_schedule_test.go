package core

import (
	"encoding/json"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
)

func TestRabbitWorkSeatLivenessV2Schedule(t *testing.T) {
	// Synthetic test height only; this does not schedule public activation.
	const activation uint64 = 500000
	official := common.HexToHash("0x9b71d7f2922fdf8383a4a12be5594e25938625195e0d84c05c3bd71b7bcf93f7")
	for _, scenario := range []string{
		"future", "late", "persisted-restart", "explicit-restart",
		"conflict", "persisted-conflict", "foreign", "disabled",
		"unknown-head", "invalid-order",
	} {
		t.Run(scenario, func(t *testing.T) {
			g := fork320000Fixture(t)
			g.Config.LQC.VRFEconomicsV2Block = RabbitTestnetFork320000
			g.Config.LQC.WorkSeatAdmissionV2Block = RabbitTestnetFork320000
			db := rawdb.NewMemoryDatabase()
			defer db.Close()
			hash := common.HexToHash("0x9876")
			height := activation - 1
			if scenario == "late" || scenario == "persisted-restart" || scenario == "explicit-restart" {
				height = activation + 1
			}
			rawdb.WriteHeadHeaderHash(db, hash)
			if scenario != "unknown-head" {
				rawdb.WriteHeaderNumber(db, hash, height)
			}

			switch scenario {
			case "explicit-restart":
				g.Config.LQC.WorkSeatLivenessV2Block = activation
			case "conflict":
				g.Config.LQC.WorkSeatLivenessV2Block = activation + 1
			case "persisted-restart", "persisted-conflict":
				stored := *g.Config
				lqc := *g.Config.LQC
				stored.LQC = &lqc
				lqc.WorkSeatLivenessV2Block = activation
				if scenario == "persisted-conflict" {
					lqc.WorkSeatLivenessV2Block++
				}
				rawdb.WriteChainConfig(db, official, &stored)
			}
			target, fork := official, activation
			if scenario == "foreign" {
				target = common.HexToHash("0x1234")
			}
			if scenario == "disabled" {
				fork = 0
			}
			if scenario == "invalid-order" {
				fork = RabbitTestnetFork320000
			}
			before, _ := json.Marshal(g.Config)
			err := configureRabbitTestnetWorkSeatLivenessV2At(db, target, g.Config, fork)
			rejected := scenario == "late" || scenario == "conflict" ||
				scenario == "persisted-conflict" || scenario == "unknown-head" ||
				scenario == "invalid-order"
			if rejected {
				if err == nil {
					t.Fatal("unsafe schedule accepted")
				}
				after, _ := json.Marshal(g.Config)
				if string(before) != string(after) {
					t.Fatal("config mutated on rejection")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			expected := activation
			if scenario == "foreign" || scenario == "disabled" {
				expected = 0
			}
			if g.Config.LQC.WorkSeatLivenessV2Block != expected {
				t.Fatal("wrong activation")
			}
			// No historical consensus or VRF field may be changed.
			g.Config.LQC.WorkSeatLivenessV2Block = 0
			after, _ := json.Marshal(g.Config)
			if string(before) != string(after) && scenario != "explicit-restart" {
				t.Fatal("unrelated configuration changed")
			}
		})
	}
}
