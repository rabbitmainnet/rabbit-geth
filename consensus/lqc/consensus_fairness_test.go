//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package lqc

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/params"
)

func fairnessAddr(hex string) common.Address {
	return common.HexToAddress(hex)
}

func TestConsensusFairnessBoundary(t *testing.T) {
	engine := New(&params.LQCConfig{
		ConsensusHardeningBlock: 50,
		ConsensusFairnessBlock:  100,
	}, nil)

	if engine.isConsensusFairnessBlock(99) {
		t.Fatal("fairness boundary active before fork")
	}
	if !engine.isConsensusFairnessBlock(100) {
		t.Fatal("fairness boundary inactive at fork")
	}
	if engine.isConsensusFairnessBlock(101) {
		t.Fatal("fairness boundary remained active after fork")
	}
	if engine.consensusFairnessActive(99) {
		t.Fatal("fairness active before fork")
	}
	if !engine.consensusFairnessActive(100) ||
		!engine.consensusFairnessActive(101) {
		t.Fatal("fairness inactive at or after fork")
	}
}

func TestConsensusFairnessPreservesRawSeatOrder(t *testing.T) {
	a := fairnessAddr("0x1000000000000000000000000000000000000001")
	b := fairnessAddr("0x2000000000000000000000000000000000000002")
	c := fairnessAddr("0x30000000000000000000000000000000000000003")

	seats := []WorkSeatV1{
		{Participant: a},
		{Participant: b},
		{Participant: c},
	}

	registry := NewCanonicalRegistry()
	registry.entries[a] = CanonicalParticipant{Address: a, Active: true}
	registry.entries[b] = CanonicalParticipant{
		Address:     b,
		Active:      true,
		JailedUntil: 10_000,
		MissedTurns: 2,
	}
	registry.entries[c] = CanonicalParticipant{Address: c, Active: true}

	engine := New(&params.LQCConfig{
		ConsensusHardeningBlock: 50,
		ConsensusFairnessBlock:  100,
	}, nil)

	before, err := engine.workV1EngineLabOrderSeatsByLiveness(
		registry, seats, 99,
	)
	if err != nil {
		t.Fatal(err)
	}
	if before[0].Participant != a ||
		before[1].Participant != c ||
		before[2].Participant != b {
		t.Fatalf("pre-fork order changed: %+v", before)
	}

	atFork, err := engine.workV1EngineLabOrderSeatsByLiveness(
		registry, seats, 100,
	)
	if err != nil {
		t.Fatal(err)
	}
	if atFork[0].Participant != a ||
		atFork[1].Participant != b ||
		atFork[2].Participant != c {
		t.Fatalf("fairness order deformed: %+v", atFork)
	}

	after, err := engine.workV1EngineLabOrderSeatsByLiveness(
		registry, seats, 101,
	)
	if err != nil {
		t.Fatal(err)
	}
	if after[0].Participant != a ||
		after[1].Participant != b ||
		after[2].Participant != c {
		t.Fatalf("post-fork order deformed: %+v", after)
	}
}

func TestConsensusFairnessBoundaryClearsLegacyPenalties(t *testing.T) {
	a := fairnessAddr("0x4000000000000000000000000000000000000004")
	b := fairnessAddr("0x5000000000000000000000000000000000000005")

	engine := New(&params.LQCConfig{
		ConsensusHardeningBlock: 50,
		ConsensusFairnessBlock:  100,
	}, nil)

	registry := NewCanonicalRegistry()
	registry.entries[a] = CanonicalParticipant{
		Address: a, Active: true, LastHeartbeat: 60,
		MissedTurns: 2, JailedUntil: 999,
	}
	registry.entries[b] = CanonicalParticipant{
		Address: b, Active: true, LastHeartbeat: 70,
		MissedTurns: 1, JailedUntil: 999,
	}

	selection := HybridSelection{
		Ordered: []HybridParticipant{
			{Address: a},
			{Address: b},
		},
	}

	err := engine.workV1EngineLabApplySeatLiveness(
		registry,
		100,
		selection,
		b,
		RegistrySnapshotRules{
			MaxMissedTurns: 3,
			JailBlocks:     256,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	pa, ok := registry.Participant(a)
	if !ok {
		t.Fatal("participant A missing")
	}
	pb, ok := registry.Participant(b)
	if !ok {
		t.Fatal("participant B missing")
	}

	if pa.MissedTurns != 0 || pa.JailedUntil != 0 {
		t.Fatalf("fairness boundary kept A penalty: %+v", pa)
	}
	if pb.MissedTurns != 0 || pb.JailedUntil != 0 {
		t.Fatalf("fairness boundary kept B penalty: %+v", pb)
	}
	if pb.LastHeartbeat != 100 {
		t.Fatalf("producer heartbeat=%d want=100", pb.LastHeartbeat)
	}
}

func TestConsensusFairnessPostForkFallbackDoesNotPunishEarlierSeats(t *testing.T) {
	a := fairnessAddr("0x6000000000000000000000000000000000000006")
	b := fairnessAddr("0x7000000000000000000000000000000000000007")
	c := fairnessAddr("0x8000000000000000000000000000000000000008")

	engine := New(&params.LQCConfig{
		ConsensusHardeningBlock: 50,
		ConsensusFairnessBlock:  100,
	}, nil)

	registry := NewCanonicalRegistry()
	for _, address := range []common.Address{a, b, c} {
		registry.entries[address] = CanonicalParticipant{
			Address: address,
			Active:  true,
		}
	}

	selection := HybridSelection{
		Ordered: []HybridParticipant{
			{Address: a},
			{Address: b},
			{Address: c},
		},
	}
	rules := RegistrySnapshotRules{
		MaxMissedTurns: 3,
		JailBlocks:     256,
	}

	for block := uint64(101); block <= 120; block++ {
		if err := engine.workV1EngineLabApplySeatLiveness(
			registry,
			block,
			selection,
			c,
			rules,
		); err != nil {
			t.Fatalf("block %d: %v", block, err)
		}
	}

	pa, _ := registry.Participant(a)
	pb, _ := registry.Participant(b)
	pc, _ := registry.Participant(c)

	if pa.MissedTurns != 0 || pa.JailedUntil != 0 {
		t.Fatalf("A accumulated post-fork penalty: %+v", pa)
	}
	if pb.MissedTurns != 0 || pb.JailedUntil != 0 {
		t.Fatalf("B accumulated post-fork penalty: %+v", pb)
	}
	if pc.LastHeartbeat != 120 {
		t.Fatalf("producer heartbeat=%d want=120", pc.LastHeartbeat)
	}
}

func TestConsensusFairnessPreForkKeepsLegacyMissedTurn(t *testing.T) {
	a := fairnessAddr("0x9000000000000000000000000000000000000000009")
	b := fairnessAddr("0xa000000000000000000000000000000000000000a")
	c := fairnessAddr("0xb000000000000000000000000000000000000000b")

	engine := New(&params.LQCConfig{
		ConsensusHardeningBlock: 50,
		ConsensusFairnessBlock:  100,
	}, nil)

	registry := NewCanonicalRegistry()
	for _, address := range []common.Address{a, b, c} {
		registry.entries[address] = CanonicalParticipant{
			Address: address,
			Active:  true,
		}
	}

	selection := HybridSelection{
		Ordered: []HybridParticipant{
			{Address: a},
			{Address: b},
			{Address: c},
		},
	}

	err := engine.workV1EngineLabApplySeatLiveness(
		registry,
		99,
		selection,
		c,
		RegistrySnapshotRules{
			MaxMissedTurns: 3,
			JailBlocks:     256,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	pa, _ := registry.Participant(a)
	pb, _ := registry.Participant(b)
	pc, _ := registry.Participant(c)

	if pa.MissedTurns != 1 || pb.MissedTurns != 1 {
		t.Fatalf(
			"pre-fork legacy misses changed: A=%d B=%d",
			pa.MissedTurns,
			pb.MissedTurns,
		)
	}
	if pc.LastHeartbeat != 99 {
		t.Fatalf(
			"producer heartbeat=%d want=99",
			pc.LastHeartbeat,
		)
	}
}

func TestConsensusFairnessTenThousandSeatsNoMassPenalty(t *testing.T) {
	const count = 10000

	engine := New(&params.LQCConfig{
		ConsensusHardeningBlock: 50,
		ConsensusFairnessBlock:  100,
	}, nil)

	registry := NewCanonicalRegistry()
	selection := HybridSelection{
		Ordered: make([]HybridParticipant, 0, count),
	}
	addresses := make([]common.Address, 0, count)

	for i := 0; i < count; i++ {
		var address common.Address
		address[16] = byte(i >> 24)
		address[17] = byte(i >> 16)
		address[18] = byte(i >> 8)
		address[19] = byte(i)
		addresses = append(addresses, address)
		registry.entries[address] = CanonicalParticipant{
			Address: address,
			Active:  true,
		}
		selection.Ordered = append(selection.Ordered, HybridParticipant{Address: address})
	}

	producer := addresses[len(addresses)-1]
	err := engine.workV1EngineLabApplySeatLiveness(
		registry,
		101,
		selection,
		producer,
		RegistrySnapshotRules{
			MaxMissedTurns: 3,
			JailBlocks:     256,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	for i, address := range addresses {
		p, ok := registry.Participant(address)
		if !ok {
			t.Fatalf("seat %d missing", i)
		}
		if p.MissedTurns != 0 || p.JailedUntil != 0 {
			t.Fatalf("seat %d punished: %+v", i, p)
		}
	}

	producerState, _ := registry.Participant(producer)
	if producerState.LastHeartbeat != 101 {
		t.Fatalf("producer heartbeat=%d want=101", producerState.LastHeartbeat)
	}
}

func TestConsensusFairnessOneMillionSeatsDeepProducer(t *testing.T) {
	const count = 1_000_000

	engine := New(&params.LQCConfig{
		ConsensusHardeningBlock: 50,
		ConsensusFairnessBlock:  100,
	}, nil)

	makeAddress := func(i int) common.Address {
		var address common.Address
		address[16] = byte(i >> 24)
		address[17] = byte(i >> 16)
		address[18] = byte(i >> 8)
		address[19] = byte(i)
		return address
	}

	first := makeAddress(0)
	middle := makeAddress(count / 2)
	producer := makeAddress(count - 1)

	registry := NewCanonicalRegistry()
	for _, address := range []common.Address{first, middle, producer} {
		registry.entries[address] = CanonicalParticipant{
			Address: address,
			Active:  true,
		}
	}

	selection := HybridSelection{
		Ordered: make([]HybridParticipant, count),
	}
	for i := 0; i < count; i++ {
		selection.Ordered[i] = HybridParticipant{Address: makeAddress(i)}
	}

	err := engine.workV1EngineLabApplySeatLiveness(
		registry,
		101,
		selection,
		producer,
		RegistrySnapshotRules{
			MaxMissedTurns: 3,
			JailBlocks:     256,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	for label, address := range map[string]common.Address{
		"first":    first,
		"middle":   middle,
		"producer": producer,
	} {
		p, ok := registry.Participant(address)
		if !ok {
			t.Fatalf("%s participant missing", label)
		}
		if p.MissedTurns != 0 || p.JailedUntil != 0 {
			t.Fatalf("%s participant punished: %+v", label, p)
		}
	}

	p, _ := registry.Participant(producer)
	if p.LastHeartbeat != 101 {
		t.Fatalf("producer heartbeat=%d want=101", p.LastHeartbeat)
	}
}

func BenchmarkConsensusFairnessOneMillionDeterministicOrdering(b *testing.B) {
	const count = 1_000_000

	input := make([]HybridParticipant, count)
	for i := 0; i < count; i++ {
		var address common.Address
		address[16] = byte(i >> 24)
		address[17] = byte(i >> 16)
		address[18] = byte(i >> 8)
		address[19] = byte(i)
		input[i] = HybridParticipant{Address: address}
	}

	parentHash := common.HexToHash("0x1234")

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ordered := DeterministicallyOrderParticipants(input, parentHash, 101)
		if len(ordered) != count {
			b.Fatalf("ordered=%d want=%d", len(ordered), count)
		}
	}
}

func BenchmarkConsensusFairnessOneMillionWorkSeatOrdering(b *testing.B) {
	const count = 1_000_000

	seats := make([]WorkSeatV1, count)
	for i := 1; i <= count; i++ {
		v := uint64(i)
		var address common.Address
		var ticket common.Hash
		for j := 0; j < 8; j++ {
			address[19-j] = byte(v >> (8 * multiple(j, 1)))
			ticket[31-j] = byte(v >> (8 * multiple(j, 1)))
		}
		seats[i-1] = WorkSeatV1{Participant: address, TicketHash: ticket}
	}

	seed := common.HexToHash("0x1234")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ordered, err := DeterministicallyOrderWorkSeatsV1(seats, seed)
		if err != nil {
			b.Fatal(err)
		}
		if len(ordered) != count {
			b.Fatalf("ordered=%d want=%d", len(ordered), count)
		}
	}
}

func BenchmarkConsensusFairnessOneMillionCompactWorkSeatSelection(b *testing.B) {
	const count = 1_000_000
	const roleLimit = 1 + 5 + 128

	seats := make([]WorkSeatV1, count)
	for i := 1; i <= count; i++ {
		v := uint64(i)
		var address common.Address
		var ticket common.Hash
		for j := 0; j < 8; j++ {
			address[19-j] = byte(v >> (8 * multiple(j, 1)))
			ticket[31-j] = byte(v >> (8 * multiple(j, 1)))
		}
		seats[i-1] = WorkSeatV1{Participant: address, TicketHash: ticket}
	}

	seed := common.HexToHash("0x1234")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		selected, err := DeterministicallySelectWorkSeatsV1(seats, seed, roleLimit)
		if err != nil {
			b.Fatal(err)
		}
		if len(selected) != roleLimit {
			b.Fatalf("selected=%d want=%d", len(selected), roleLimit)
		}
	}
}

func multiple(a, b int) uint {
	return uint(a * b)
}
