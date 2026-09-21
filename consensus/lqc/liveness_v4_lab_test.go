//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package lqc

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/params"
)

func TestLivenessV4ExcludesJailedSeatFromAuthorQueue(t *testing.T) {
	registry := NewCanonicalRegistry()

	a := common.BigToAddress(big.NewInt(1))
	b := common.BigToAddress(big.NewInt(2))
	c := common.BigToAddress(big.NewInt(3))

	for _, address := range []common.Address{a, b, c} {
		if err := registry.MarkWorkSeatProducerHeartbeat(address, 100); err != nil {
			t.Fatal(err)
		}
	}
	if err := registry.ApplyWorkSeatMissedTurn(a, 100, 1, 256); err != nil {
		t.Fatal(err)
	}

	seats := []WorkSeatV1{
		{TicketHash: common.BigToHash(big.NewInt(1)), Participant: a},
		{TicketHash: common.BigToHash(big.NewInt(2)), Participant: b},
		{TicketHash: common.BigToHash(big.NewInt(3)), Participant: c},
	}

	engine := &LQC{config: &params.LQCConfig{
		ConsensusLivenessV3Block: 1,
		ConsensusLivenessV4Block: 1,
		HeartbeatWindow:          64,
		HeartbeatGrace:           16,
	}}

	ordered, err := engine.workV1EngineLabOrderSeatsByLivenessV4(
		registry,
		seats,
		101,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(ordered) != 2 ||
		ordered[0].Participant != b ||
		ordered[1].Participant != c {
		t.Fatalf("V4 author queue contains jailed/unexpected seat: %+v", ordered)
	}
	for _, seat := range ordered {
		if seat.Participant == a {
			t.Fatal("jailed WorkSeat remained authorizable in V4 Ordered queue")
		}
	}
}

func TestLivenessV4KeepsStaleNonJailedSeatAsDelayedRecovery(t *testing.T) {
	registry := NewCanonicalRegistry()

	stale := common.BigToAddress(big.NewInt(1))
	ready := common.BigToAddress(big.NewInt(2))

	if err := registry.MarkWorkSeatProducerHeartbeat(stale, 1); err != nil {
		t.Fatal(err)
	}
	if err := registry.MarkWorkSeatProducerHeartbeat(ready, 100); err != nil {
		t.Fatal(err)
	}

	seats := []WorkSeatV1{
		{TicketHash: common.BigToHash(big.NewInt(1)), Participant: stale},
		{TicketHash: common.BigToHash(big.NewInt(2)), Participant: ready},
	}

	engine := &LQC{config: &params.LQCConfig{
		ConsensusLivenessV3Block: 1,
		ConsensusLivenessV4Block: 1,
		HeartbeatWindow:          64,
		HeartbeatGrace:           16,
	}}

	ordered, err := engine.workV1EngineLabOrderSeatsByLivenessV4(
		registry,
		seats,
		101,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(ordered) != 2 ||
		ordered[0].Participant != ready ||
		ordered[1].Participant != stale {
		t.Fatalf("stale recovery ordering invalid: %+v", ordered)
	}
}

func TestLivenessV4ActivationRestoresLegacySeatLiveness(t *testing.T) {
	registry := NewCanonicalRegistry()

	a := common.BigToAddress(big.NewInt(1))
	b := common.BigToAddress(big.NewInt(2))

	for _, address := range []common.Address{a, b} {
		if err := registry.MarkWorkSeatProducerHeartbeat(address, 3); err != nil {
			t.Fatal(err)
		}
	}
	if err := registry.ApplyWorkSeatMissedTurn(a, 3, 1, 256); err != nil {
		t.Fatal(err)
	}

	ordered := []HybridParticipant{
		{Address: a},
		{Address: b},
	}
	selection := HybridSelection{
		Ordered:  ordered,
		Producer: &ordered[0],
	}

	engine := &LQC{config: &params.LQCConfig{
		ConsensusHardeningBlock:  1,
		ConsensusFairnessBlock:   2,
		ConsensusLivenessV3Block: 3,
		ConsensusLivenessV4Block: 4,
		MaxMissedTurns:           3,
		JailBlocks:               256,
		HeartbeatWindow:          64,
		HeartbeatGrace:           16,
	}}

	if err := engine.workV1EngineLabApplySeatLiveness(
		registry,
		4,
		selection,
		a,
		engine.registryRules(),
	); err != nil {
		t.Fatal(err)
	}

	pa, _ := registry.Participant(a)
	pb, _ := registry.Participant(b)
	if pa.JailedUntil != 0 || pa.MissedTurns != 0 || pa.LastHeartbeat != 4 {
		t.Fatalf("V4 activation did not restore producer liveness: %+v", pa)
	}
	if pb.JailedUntil != 0 || pb.MissedTurns != 0 {
		t.Fatalf("V4 activation carried legacy jail state: %+v", pb)
	}
}

func TestLivenessV4AppliesMissedTurnsAfterFairnessFork(t *testing.T) {
	registry := NewCanonicalRegistry()

	a := common.BigToAddress(big.NewInt(1))
	b := common.BigToAddress(big.NewInt(2))
	c := common.BigToAddress(big.NewInt(3))

	for _, address := range []common.Address{a, b, c} {
		if err := registry.MarkWorkSeatProducerHeartbeat(address, 9); err != nil {
			t.Fatal(err)
		}
	}

	ordered := []HybridParticipant{
		{Address: a},
		{Address: b},
		{Address: c},
	}
	selection := HybridSelection{
		Ordered:  ordered,
		Producer: &ordered[0],
	}

	engine := &LQC{config: &params.LQCConfig{
		ConsensusHardeningBlock:  1,
		ConsensusFairnessBlock:   2,
		ConsensusLivenessV3Block: 3,
		ConsensusLivenessV4Block: 4,
		MaxMissedTurns:           3,
		JailBlocks:               256,
		HeartbeatWindow:          64,
		HeartbeatGrace:           16,
	}}

	rules := engine.registryRules()
	for block := uint64(10); block <= 12; block++ {
		if err := engine.workV1EngineLabApplySeatLiveness(
			registry,
			block,
			selection,
			c,
			rules,
		); err != nil {
			t.Fatal(err)
		}
	}

	pa, _ := registry.Participant(a)
	pb, _ := registry.Participant(b)
	pc, _ := registry.Participant(c)

	if pa.JailedUntil <= 12 || pb.JailedUntil <= 12 {
		t.Fatalf("offline prefix was not jailed: a=%+v b=%+v", pa, pb)
	}
	if pc.LastHeartbeat != 12 || pc.JailedUntil != 0 {
		t.Fatalf("live producer state invalid: %+v", pc)
	}
}
