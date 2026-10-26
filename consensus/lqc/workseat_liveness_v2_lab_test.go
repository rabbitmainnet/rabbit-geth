//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package lqc

import (
	"errors"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/params"
)

func TestWorkSeatLivenessV2ActivationAndPenalty(t *testing.T) {
	address := common.HexToAddress("0x1234")
	cfg := &params.LQCConfig{WorkSeatLivenessV2Block: 200}
	engine := New(cfg, nil)
	for _, height := range []uint64{198, 199} {
		reg := NewCanonicalRegistry()
		root := reg.Root()
		if err := engine.workSeatMissedTurnWithLivenessV2(reg, address, height, 3, 256); !errors.Is(err, ErrParticipantNotActive) {
			t.Fatalf("historical error: %v", err)
		}
		if reg.Root() != root {
			t.Fatal("historical state changed")
		}
	}
	cfg.WorkSeatLivenessV2Block = 0
	reg := NewCanonicalRegistry()
	root := reg.Root()
	if err := engine.workSeatMissedTurnWithLivenessV2(reg, address, 300, 3, 256); !errors.Is(err, ErrParticipantNotActive) {
		t.Fatal("disabled gate did not preserve legacy behavior")
	}
	if reg.Root() != root {
		t.Fatal("disabled gate changed state")
	}
	cfg.WorkSeatLivenessV2Block = 200
	for n := uint64(200); n <= 202; n++ {
		if err := engine.workSeatMissedTurnWithLivenessV2(reg, address, n, 3, 256); err != nil {
			t.Fatal(err)
		}
	}
	entry, exists := reg.Participant(address)
	if !exists || entry.RegisteredAt != 200 || entry.LastHeartbeat != 0 || entry.Sequence != 0 || entry.Active || entry.MissedTurns != 0 || entry.JailedUntil != 458 {
		t.Fatalf("incorrect admission bookkeeping/penalty: %+v", entry)
	}
	root = reg.Root()
	if err := engine.workSeatMissedTurnWithLivenessV2(reg, address, 203, 3, 256); err != nil {
		t.Fatal(err)
	}
	if reg.Root() != root {
		t.Fatal("existing jail was extended")
	}
	if err := engine.workSeatMissedTurnWithLivenessV2(reg, common.Address{}, 203, 3, 256); !errors.Is(err, ErrInvalidRegistryAddress) {
		t.Fatalf("zero address accepted: %v", err)
	}
	if reg.Root() != root {
		t.Fatal("invalid input changed registry")
	}
}

func TestWorkSeatLivenessV2ExistingEntriesUnchanged(t *testing.T) {
	address := common.HexToAddress("0x2345")
	before := NewCanonicalRegistry()
	before.entries[address] = CanonicalParticipant{Address: address, RegisteredAt: 100, LastHeartbeat: 150, Active: true, Sequence: 7}
	after := before.Clone()
	engine := New(&params.LQCConfig{WorkSeatLivenessV2Block: 200}, nil)
	for n := uint64(200); n <= 206; n++ {
		oldErr := before.ApplyWorkSeatMissedTurn(address, n, 3, 256)
		newErr := engine.workSeatMissedTurnWithLivenessV2(after, address, n, 3, 256)
		if oldErr != newErr || before.Root() != after.Root() {
			t.Fatalf("existing entry changed at %d", n)
		}
	}
}
