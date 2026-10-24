//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package lqc

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/params"
)

func TestRabbitWorkSeatAdmissionV2EmptyQueueBoundary(t *testing.T) {
	for _, item := range []struct {
		name       string
		activation uint64
		block      uint64
		wantActive bool
	}{
		{"before", 320000, 319999, true},
		{"at", 320000, 320000, false},
		{"after", 320000, 320001, false},
		{"disabled", 0, 320001, true},
	} {
		t.Run(item.name, func(t *testing.T) {
			registry := NewCanonicalRegistry()
			address := common.HexToAddress("0x1234")
			if err := registry.MarkWorkSeatProducerHeartbeat(address, 319998); err != nil {
				t.Fatal(err)
			}
			if err := registry.ApplyWorkSeatMissedTurn(address, 319998, 1, 256); err != nil {
				t.Fatal(err)
			}
			engine := &LQC{config: &params.LQCConfig{
				ConsensusLivenessV3Block: 77000,
				ConsensusLivenessV4Block: 97991,
				ConsensusLivenessV5Block: 115000,
				WorkSeatAdmissionV2Block: item.activation,
			}}
			selection, active, err := engine.workV1EngineLabBuildSeatSelection(
				big.NewInt(9280), 1, common.HexToHash("0x11"),
				[]WorkSeatV1{{
					Participant: address, TicketHash: common.HexToHash("0x22"),
				}},
				common.HexToHash("0x33"), item.block, registry,
				RegistrySnapshotRules{},
				func(common.Hash, []byte) (common.Hash, error) {
					return common.HexToHash("0x44"), nil
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			if selection.Producer != nil || len(selection.Ordered) != 0 {
				t.Fatal("jailed seat entered producer queue")
			}
			if active != item.wantActive {
				t.Errorf("historical empty queue flag: got=%t want=%t", active, item.wantActive)
			}
		})
	}
}
