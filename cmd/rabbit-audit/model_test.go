package main

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/params"
)

func TestRewardEraBoundaries(t *testing.T) {
	eraLength := uint64(100)
	tests := []struct {
		block uint64
		era   uint64
		wei   string
	}{
		{block: 1, era: 0, wei: "1200000000000000000"},
		{block: 99, era: 0, wei: "1200000000000000000"},
		{block: 100, era: 1, wei: "600000000000000000"},
		{block: 199, era: 1, wei: "600000000000000000"},
		{block: 200, era: 2, wei: "300000000000000000"},
		{block: 300, era: 3, wei: "150000000000000000"},
		{block: 900, era: 9, wei: "150000000000000000"},
	}
	for _, test := range tests {
		reward, era := rewardForBlock(test.block, eraLength)
		if era != test.era {
			t.Fatalf("block %d era: got %d want %d", test.block, era, test.era)
		}
		if reward.String() != test.wei {
			t.Fatalf("block %d reward: got %s want %s", test.block, reward, test.wei)
		}
	}
}

func TestScheduledRewardsStartsAtBlockOne(t *testing.T) {
	got := scheduledRewards(1, 100, 100)
	want := new(big.Int).Add(
		new(big.Int).Mul(mustBig("1200000000000000000"), big.NewInt(99)),
		mustBig("600000000000000000"),
	)
	if got.Cmp(want) != 0 {
		t.Fatalf("scheduled reward: got %s want %s", got, want)
	}
}

func TestCommitteeSplitPreservesEveryWei(t *testing.T) {
	producer := common.HexToAddress("0x0000000000000000000000000000000000000001")
	committee := []common.Address{
		common.HexToAddress("0x0000000000000000000000000000000000000011"),
		common.HexToAddress("0x0000000000000000000000000000000000000012"),
		common.HexToAddress("0x0000000000000000000000000000000000000013"),
		common.HexToAddress("0x0000000000000000000000000000000000000014"),
		common.HexToAddress("0x0000000000000000000000000000000000000015"),
		common.HexToAddress("0x0000000000000000000000000000000000000016"),
		common.HexToAddress("0x0000000000000000000000000000000000000017"),
	}
	total := big.NewInt(101)
	allocations := allocateReward(total, producer, committee, 3_000)
	sum := new(big.Int)
	for _, allocation := range allocations {
		sum.Add(sum, allocation.Amount)
	}
	if sum.Cmp(total) != 0 {
		t.Fatalf("allocated %s of %s", sum, total)
	}
	if allocations[0].Amount.String() != "70" {
		t.Fatalf("producer got %s, want 70", allocations[0].Amount)
	}
	if allocations[1].Amount.String() != "7" {
		t.Fatalf("first committee member must receive remainder: got %s want 7", allocations[1].Amount)
	}
}

func TestLegacyDynamicSelectionSizes(t *testing.T) {
	tests := []struct {
		active    int
		fallbacks uint64
		committee uint64
	}{
		{active: 1, fallbacks: 0, committee: 0},
		{active: 2, fallbacks: 1, committee: 0},
		{active: 5, fallbacks: 3, committee: 1},
		{active: 20, fallbacks: 5, committee: 5},
		{active: 100, fallbacks: 7, committee: 10},
	}
	for _, test := range tests {
		got := legacySelectionConfig(test.active)
		if got.FallbackCount != test.fallbacks || got.CommitteeSize != test.committee {
			t.Fatalf("active %d: got %d/%d want %d/%d", test.active, got.FallbackCount, got.CommitteeSize, test.fallbacks, test.committee)
		}
	}
}

func TestCanonicalLQCUsesConfiguredSelectionSizes(t *testing.T) {
	config := effectiveSelectionConfig(&params.LQCConfig{
		FallbackCount: 2,
		CommitteeSize: 5,
	})
	if config.FallbackCount != 2 || config.CommitteeSize != 5 || config.DynamicCommittee {
		t.Fatalf("configured selection = %d/%d, want 2/5", config.FallbackCount, config.CommitteeSize)
	}
}

func TestCanonicalDynamicCommitteeSizing(t *testing.T) {
	config := effectiveSelectionConfig(&params.LQCConfig{
		FallbackCount: 5,
		CommitteeMin:  32,
		CommitteeMax:  128,
	})
	if !config.DynamicCommittee {
		t.Fatal("committee without explicit committeeSize must be dynamic")
	}
	if got := config.canonicalCommitteeSize(20); got != 32 {
		t.Fatalf("20 active participants: got %d want bounded target 32", got)
	}
	if got := config.canonicalCommitteeSize(1_000); got != 100 {
		t.Fatalf("1000 active participants: got %d want 100", got)
	}
	if got := config.canonicalCommitteeSize(10_000); got != 128 {
		t.Fatalf("10000 active participants: got %d want max 128", got)
	}
}

func TestCanonicalCommitteeRemovesFallbackProducer(t *testing.T) {
	queue := make([]common.Address, 0, 8)
	for i := byte(1); i <= 8; i++ {
		queue = append(queue, common.BytesToAddress([]byte{i}))
	}
	config := effectiveSelectionConfig(&params.LQCConfig{
		FallbackCount: 2,
		CommitteeSize: 3,
	})
	producer := queue[3]
	committee := canonicalRewardCommittee(queue, producer, config)
	if len(committee) != 2 || committee[0] != queue[4] || committee[1] != queue[5] {
		t.Fatalf("committee = %v, want queue[4:6] after removing producer", committee)
	}
}

func TestNoCommitteePaysProducerEverything(t *testing.T) {
	producer := common.HexToAddress("0x0000000000000000000000000000000000000001")
	allocations := allocateReward(big.NewInt(101), producer, nil, 3_000)
	if len(allocations) != 1 || allocations[0].Amount.String() != "101" {
		t.Fatalf("unexpected allocation: %+v", allocations)
	}
}

func TestMiningRewardCreditsLiquidImmediately(t *testing.T) {
	state := accountState{
		Balance:  big.NewInt(7),
		Locked:   new(big.Int),
		Original: new(big.Int),
	}
	creditExpected(&state, big.NewInt(5))
	if state.Balance.String() != "12" {
		t.Fatalf("liquid balance = %s, want 12", state.Balance)
	}
	if state.Locked.Sign() != 0 || state.Original.Sign() != 0 || state.Stage != 0 {
		t.Fatalf("legacy vesting state changed: %+v", state)
	}
}

func TestRegistryParticipantIsTrackedBeforeFirstProduction(t *testing.T) {
	bootstrap := common.HexToAddress("0x0000000000000000000000000000000000000001")
	newcomer := common.HexToAddress("0x0000000000000000000000000000000000000021")
	snapshot, err := lqc.NewBootstrapRegistrySnapshot(
		7,
		common.HexToHash("0x1234"),
		[]common.Address{bootstrap, newcomer},
	)
	if err != nil {
		t.Fatal(err)
	}
	runner := &auditRunner{participants: []common.Address{bootstrap}}
	added := runner.addRegistryParticipants(snapshot)
	if len(added) != 1 || added[0] != newcomer {
		t.Fatalf("added = %v, want newcomer %s", added, newcomer)
	}
	if !containsAddress(runner.participants, newcomer) {
		t.Fatal("newcomer was not tracked from the canonical registry snapshot")
	}
}
