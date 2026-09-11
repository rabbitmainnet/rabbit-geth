//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package lqc

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/params"
)

func TestConsensusStabilizationBoundary(t *testing.T) {
	engine := New(&params.LQCConfig{
		ConsensusStabilizationBlock: 100,
	}, nil)

	if engine.isConsensusStabilizationBlock(99) {
		t.Fatal("stabilization active before boundary")
	}
	if !engine.isConsensusStabilizationBlock(100) {
		t.Fatal("stabilization inactive at boundary")
	}
	if engine.isConsensusStabilizationBlock(101) {
		t.Fatal("boundary remained active after stabilization")
	}
	if engine.consensusCommitteeLivenessActive(100) {
		t.Fatal("committee filtering active during reset block")
	}
	if !engine.consensusCommitteeLivenessActive(101) {
		t.Fatal("committee filtering inactive after stabilization")
	}
}

func TestRestoreWorkSeatLivenessRecreatesRecoveryEntries(t *testing.T) {
	existing := common.HexToAddress("0x2000000000000000000000000000000000000002")
	missing := common.HexToAddress("0x3000000000000000000000000000000000000003")

	registry := NewCanonicalRegistry()
	registry.entries[existing] = CanonicalParticipant{
		Address:       existing,
		RegisteredAt:  10,
		LastHeartbeat: 20,
		MissedTurns:   2,
		JailedUntil:   999,
		Sequence:      7,
		Active:        true,
	}

	if err := registry.RestoreWorkSeatLiveness(
		[]common.Address{existing, missing},
		100,
	); err != nil {
		t.Fatal(err)
	}

	gotExisting, ok := registry.Participant(existing)
	if !ok ||
		gotExisting.RegisteredAt != 10 ||
		gotExisting.LastHeartbeat != 20 ||
		gotExisting.Sequence != 7 ||
		!gotExisting.Active ||
		gotExisting.MissedTurns != 0 ||
		gotExisting.JailedUntil != 0 {
		t.Fatalf("existing participant changed incorrectly: %+v", gotExisting)
	}

	gotMissing, ok := registry.Participant(missing)
	if !ok ||
		gotMissing.Address != missing ||
		gotMissing.RegisteredAt != 100 ||
		gotMissing.Active ||
		gotMissing.MissedTurns != 0 ||
		gotMissing.JailedUntil != 0 {
		t.Fatalf("missing WorkSeat not restored: %+v", gotMissing)
	}
}

func TestRecoveryPreservesParticipantMetadata(t *testing.T) {
	address := common.HexToAddress(
		"0x3100000000000000000000000000000000000003",
	)
	registry := NewCanonicalRegistry()
	registry.entries[address] = CanonicalParticipant{
		Address:       address,
		RegisteredAt:  10,
		LastHeartbeat: 20,
		MissedTurns:   2,
		JailedUntil:   999,
		Sequence:      7,
		Active:        false,
	}

	if err := registry.RecoverPermissionlessProducer(
		address,
		100,
	); err != nil {
		t.Fatal(err)
	}

	got, ok := registry.Participant(address)
	if !ok ||
		got.RegisteredAt != 10 ||
		got.Sequence != 7 ||
		got.LastHeartbeat != 100 ||
		got.MissedTurns != 0 ||
		got.JailedUntil != 0 ||
		!got.Active {
		t.Fatalf(
			"recovery changed participant incorrectly: %+v",
			got,
		)
	}
}

func TestConsensusHardeningRecreatesMissingWorkSeats(t *testing.T) {
	first := common.HexToAddress(
		"0x4100000000000000000000000000000000000004",
	)
	second := common.HexToAddress(
		"0x5100000000000000000000000000000000000005",
	)

	engine := New(&params.LQCConfig{
		ConsensusHardeningBlock: 260,
	}, nil)
	registry := NewCanonicalRegistry()
	selection := HybridSelection{
		Ordered: []HybridParticipant{
			{Address: first},
			{Address: second},
		},
	}

	if err := engine.workV1EngineLabApplySeatLiveness(
		registry,
		260,
		selection,
		first,
		RegistrySnapshotRules{},
	); err != nil {
		t.Fatal(err)
	}

	firstParticipant, firstExists := registry.Participant(first)
	secondParticipant, secondExists := registry.Participant(second)

	if !firstExists || !secondExists {
		t.Fatalf(
			"hardening failed to recreate WorkSeats: first=%v second=%v",
			firstExists,
			secondExists,
		)
	}
	if firstParticipant.LastHeartbeat != 260 {
		t.Fatalf(
			"producer heartbeat=%d want=260",
			firstParticipant.LastHeartbeat,
		)
	}
	if firstParticipant.JailedUntil != 0 ||
		secondParticipant.JailedUntil != 0 {
		t.Fatal("hardening retained WorkSeat penalties")
	}
}

func TestConsensusStabilizationRestoresRegistryAtBoundary(t *testing.T) {
	first := common.HexToAddress("0x4000000000000000000000000000000000000004")
	second := common.HexToAddress("0x5000000000000000000000000000000000000005")

	engine := New(&params.LQCConfig{
		ConsensusHardeningBlock:     50,
		ConsensusStabilizationBlock: 100,
	}, nil)
	registry := NewCanonicalRegistry()
	selection := HybridSelection{
		Ordered: []HybridParticipant{
			{Address: first},
			{Address: second},
		},
	}

	err := engine.workV1EngineLabApplySeatLiveness(
		registry,
		100,
		selection,
		first,
		RegistrySnapshotRules{},
	)
	if err != nil {
		t.Fatal(err)
	}

	firstParticipant, firstExists := registry.Participant(first)
	secondParticipant, secondExists := registry.Participant(second)

	if !firstExists || !secondExists {
		t.Fatalf(
			"registry not rebuilt: first=%v second=%v",
			firstExists,
			secondExists,
		)
	}
	if firstParticipant.LastHeartbeat != 100 {
		t.Fatalf(
			"producer heartbeat=%d want=100",
			firstParticipant.LastHeartbeat,
		)
	}
	if firstParticipant.JailedUntil != 0 ||
		secondParticipant.JailedUntil != 0 {
		t.Fatal("stabilization retained old penalties")
	}
}

func TestCommitteeLivenessExcludesPenalizedAndMissingSeats(t *testing.T) {
	ready := common.HexToAddress("0x6000000000000000000000000000000000000006")
	penalized := common.HexToAddress("0x7000000000000000000000000000000000000007")
	missing := common.HexToAddress("0x8000000000000000000000000000000000000008")

	registry := NewCanonicalRegistry()
	registry.entries[ready] = CanonicalParticipant{
		Address: ready,
	}
	registry.entries[penalized] = CanonicalParticipant{
		Address:     penalized,
		JailedUntil: 200,
	}

	selection := WorkSelectionV1{
		Committee: []WorkSeatV1{
			{Participant: ready},
			{Participant: penalized},
			{Participant: missing},
		},
	}

	filtered := workV1EngineLabFilterCommitteeByLiveness(
		selection,
		registry,
		100,
	)

	if len(filtered.Committee) != 1 ||
		filtered.Committee[0].Participant != ready {
		t.Fatalf("unexpected paid committee: %+v", filtered.Committee)
	}
}
