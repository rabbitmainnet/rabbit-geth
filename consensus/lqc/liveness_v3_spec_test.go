//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package lqc

import (
	"math/big"
	"reflect"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/params"
)

func livenessV3TestSelection() (HybridSelection, []WorkSeatV1) {
	seats := make([]WorkSeatV1, 10)
	for i := range seats {
		seats[i] = WorkSeatV1{
			TicketHash:  common.BigToHash(big.NewInt(int64(i + 1))),
			Participant: common.BigToAddress(big.NewInt(int64(i + 1))),
		}
	}
	work := buildWorkSelectionFromOrderedSeatsV1(seats, 5, 2)
	return workV1EngineLabHybridSelection(work), seats
}

func TestWorkV1EngineLabRoleCountsPreserveBoundedLiveness(t *testing.T) {
	tests := []struct {
		name          string
		seats         uint64
		fallbacks     uint64
		committee     uint64
		reserve       bool
		wantFallbacks uint64
		wantCommittee uint64
	}{
		{name: "empty", seats: 0, fallbacks: 5, committee: 32, reserve: true, wantFallbacks: 0, wantCommittee: 0},
		{name: "single seat", seats: 1, fallbacks: 5, committee: 32, reserve: true, wantFallbacks: 0, wantCommittee: 0},
		{name: "legacy four seats saturated", seats: 4, fallbacks: 5, committee: 4, reserve: false, wantFallbacks: 0, wantCommittee: 3},
		{name: "v4 two seats keep takeover", seats: 2, fallbacks: 5, committee: 32, reserve: true, wantFallbacks: 1, wantCommittee: 0},
		{name: "v4 four seats saturated", seats: 4, fallbacks: 5, committee: 4, reserve: true, wantFallbacks: 1, wantCommittee: 2},
		{name: "v4 testnet minimum saturated", seats: 33, fallbacks: 5, committee: 32, reserve: true, wantFallbacks: 1, wantCommittee: 31},
		{name: "v4 one natural fallback", seats: 34, fallbacks: 5, committee: 32, reserve: true, wantFallbacks: 1, wantCommittee: 32},
		{name: "v4 full fallback budget", seats: 38, fallbacks: 5, committee: 32, reserve: true, wantFallbacks: 5, wantCommittee: 32},
		{name: "v4 fallback disabled", seats: 4, fallbacks: 0, committee: 4, reserve: true, wantFallbacks: 0, wantCommittee: 3},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fallbacks, committee := workV1EngineLabRoleCounts(
				test.seats,
				test.fallbacks,
				test.committee,
				test.reserve,
			)
			if fallbacks != test.wantFallbacks || committee != test.wantCommittee {
				t.Fatalf(
					"roles=%d/%d want=%d/%d",
					fallbacks,
					committee,
					test.wantFallbacks,
					test.wantCommittee,
				)
			}
		})
	}
}

func TestLivenessV3SaturatedCommitteeKeepsOneBoundedFallback(t *testing.T) {
	seats := make([]WorkSeatV1, 4)
	for index := range seats {
		seats[index] = WorkSeatV1{
			TicketHash:  selectionV1TicketHash(index + 1),
			Participant: common.BigToAddress(big.NewInt(int64(index + 1))),
		}
	}

	fallbacks, committee := workV1EngineLabRoleCounts(4, 5, 4, true)
	selection := workV1EngineLabHybridSelection(
		buildWorkSelectionFromOrderedSeatsV1(seats, fallbacks, committee),
	)
	if len(selection.Fallbacks) != 1 || len(selection.Committee) != 2 {
		t.Fatalf(
			"roles fallback=%d committee=%d want=1/2",
			len(selection.Fallbacks),
			len(selection.Committee),
		)
	}

	if allowed, pos := IsAuthorAllowedBounded(selection, seats[1].Participant); !allowed || pos != 1 {
		t.Fatalf("bounded fallback rejected: allowed=%v pos=%d", allowed, pos)
	}
	for _, seat := range seats[2:] {
		if allowed, _ := IsAuthorAllowedBounded(selection, seat.Participant); allowed {
			t.Fatalf("committee seat %s became a producer fallback", seat.Participant)
		}
	}
}

func TestLivenessV4RoleBudgetForkBoundary(t *testing.T) {
	engine := &LQC{config: &params.LQCConfig{
		ConsensusLivenessV3Block: 5,
		ConsensusLivenessV4Block: 100,
	}}

	preFallbacks, preCommittee := workV1EngineLabRoleCounts(
		4, 5, 4, engine.consensusLivenessV4Active(99),
	)
	if preFallbacks != 0 || preCommittee != 3 {
		t.Fatalf("pre-fork roles=%d/%d want=0/3", preFallbacks, preCommittee)
	}

	postFallbacks, postCommittee := workV1EngineLabRoleCounts(
		4, 5, 4, engine.consensusLivenessV4Active(100),
	)
	if postFallbacks != 1 || postCommittee != 2 {
		t.Fatalf("post-fork roles=%d/%d want=1/2", postFallbacks, postCommittee)
	}
}

func TestLivenessV3CompactRolesMatchFullAvailabilityOrder(t *testing.T) {
	const count = 10_000
	const roleLimit = 1 + 5 + 128

	seats := make([]WorkSeatV1, count)
	registry := NewCanonicalRegistry()
	for index := range seats {
		seat := WorkSeatV1{
			TicketHash:  selectionV1TicketHash(index + 1),
			Participant: common.BigToAddress(big.NewInt(int64(index + 1))),
		}
		seats[index] = seat
		if index%3 == 0 {
			if err := registry.MarkWorkSeatProducerHeartbeat(seat.Participant, 95); err != nil {
				t.Fatal(err)
			}
		} else if index%3 == 1 {
			if err := registry.MarkWorkSeatProducerHeartbeat(seat.Participant, 70); err != nil {
				t.Fatal(err)
			}
		}
	}

	engine := &LQC{config: &params.LQCConfig{
		ConsensusLivenessV3Block: 100,
		HeartbeatWindow:          10,
		HeartbeatGrace:           2,
	}}
	seed := selectionV1SeedForTest(100)
	full, err := DeterministicallyOrderWorkSeatsV1(seats, seed)
	if err != nil {
		t.Fatal(err)
	}
	full, err = engine.workV1EngineLabOrderSeatsByLiveness(registry, full, 100)
	if err != nil {
		t.Fatal(err)
	}
	compact, err := engine.workV1EngineLabSelectRolesV3(
		registry,
		seats,
		seed,
		100,
		roleLimit,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(compact, full[:roleLimit]) {
		t.Fatal("compact V3 roles differ from full availability order")
	}
}

func TestLivenessV3FallbackAuthorizationIsBounded(t *testing.T) {
	selection, seats := livenessV3TestSelection()

	for position := 0; position <= 5; position++ {
		allowed, queuePos := IsAuthorAllowedBounded(
			selection,
			seats[position].Participant,
		)
		if !allowed || queuePos != position {
			t.Fatalf(
				"scheduled position %d rejected: allowed=%v queuePos=%d",
				position,
				allowed,
				queuePos,
			)
		}
	}

	for position := 6; position < len(seats); position++ {
		allowed, _ := IsAuthorAllowedBounded(
			selection,
			seats[position].Participant,
		)
		if allowed {
			t.Fatalf(
				"position %d outside producer and fallbacks was authorized",
				position,
			)
		}
	}
}

func TestLivenessV3AuthorizationForkBoundary(t *testing.T) {
	selection, seats := livenessV3TestSelection()
	engine := &LQC{config: &params.LQCConfig{
		ConsensusLivenessV3Block: 100,
	}}

	if allowed, pos := engine.isAuthorAllowedAt(
		99, selection, seats[9].Participant,
	); !allowed || pos != 9 {
		t.Fatalf(
			"pre-fork authorization changed: allowed=%v pos=%d",
			allowed,
			pos,
		)
	}

	if allowed, _ := engine.isAuthorAllowedAt(
		100, selection, seats[9].Participant,
	); allowed {
		t.Fatal("post-fork seat outside bounded fallbacks was authorized")
	}

	if allowed, pos := engine.isAuthorAllowedAt(
		100, selection, seats[5].Participant,
	); !allowed || pos != 5 {
		t.Fatalf(
			"final fallback rejected: allowed=%v pos=%d",
			allowed,
			pos,
		)
	}
}

func TestWorkSeatProducerHeartbeatCreatesOnlyLivenessState(t *testing.T) {
	registry := NewCanonicalRegistry()
	address := common.HexToAddress(
		"0x1000000000000000000000000000000000000001",
	)

	if err := registry.MarkWorkSeatProducerHeartbeat(address, 123); err != nil {
		t.Fatal(err)
	}

	participant, exists := registry.Participant(address)
	if !exists ||
		participant.Active ||
		participant.RegisteredAt != 123 ||
		participant.LastHeartbeat != 123 ||
		participant.MissedTurns != 0 ||
		participant.JailedUntil != 0 {
		t.Fatalf("unexpected WorkSeat liveness state: %+v", participant)
	}
}

func TestLivenessV3AvailabilityReordersWithoutLosingOwnership(t *testing.T) {
	offlineA := common.BigToAddress(big.NewInt(1))
	availableB := common.BigToAddress(big.NewInt(2))
	offlineC := common.BigToAddress(big.NewInt(3))

	ordered := []WorkSeatV1{
		{Participant: offlineA, TicketHash: common.BigToHash(big.NewInt(1))},
		{Participant: availableB, TicketHash: common.BigToHash(big.NewInt(2))},
		{Participant: offlineC, TicketHash: common.BigToHash(big.NewInt(3))},
	}

	registry := NewCanonicalRegistry()
	if err := registry.MarkWorkSeatProducerHeartbeat(offlineA, 80); err != nil {
		t.Fatal(err)
	}
	if err := registry.MarkWorkSeatProducerHeartbeat(availableB, 95); err != nil {
		t.Fatal(err)
	}

	engine := &LQC{config: &params.LQCConfig{
		ConsensusHardeningBlock:  50,
		ConsensusFairnessBlock:   70,
		ConsensusLivenessV3Block: 100,
		HeartbeatWindow:          10,
		HeartbeatGrace:           2,
	}}

	before, err := engine.workV1EngineLabOrderSeatsByLiveness(
		registry, ordered, 99,
	)
	if err != nil {
		t.Fatal(err)
	}
	if before[0].Participant != offlineA {
		t.Fatal("pre-fork deterministic order changed")
	}

	after, err := engine.workV1EngineLabOrderSeatsByLiveness(
		registry, ordered, 100,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(ordered) {
		t.Fatalf("WorkSeat ownership changed: got=%d want=%d", len(after), len(ordered))
	}

	want := []common.Address{availableB, offlineA, offlineC}
	for i := range want {
		if after[i].Participant != want[i] {
			t.Fatalf("position %d=%s want=%s", i, after[i].Participant, want[i])
		}
	}
}

func TestLivenessV3CommitteeExcludesUnavailableSeats(t *testing.T) {
	fresh := common.BigToAddress(big.NewInt(1))
	stale := common.BigToAddress(big.NewInt(2))
	missing := common.BigToAddress(big.NewInt(3))

	selection := WorkSelectionV1{
		Committee: []WorkSeatV1{
			{Participant: fresh},
			{Participant: stale},
			{Participant: missing},
		},
	}

	registry := NewCanonicalRegistry()
	if err := registry.MarkWorkSeatProducerHeartbeat(fresh, 95); err != nil {
		t.Fatal(err)
	}
	if err := registry.MarkWorkSeatProducerHeartbeat(stale, 80); err != nil {
		t.Fatal(err)
	}

	filtered := workV1EngineLabFilterCommitteeByAvailabilityV3(
		selection,
		registry,
		100,
		RegistrySnapshotRules{
			HeartbeatWindow: 10,
			HeartbeatGrace:  2,
		},
	)

	if len(filtered.Committee) != 1 ||
		filtered.Committee[0].Participant != fresh {
		t.Fatalf("unexpected paid committee: %+v", filtered.Committee)
	}
}
