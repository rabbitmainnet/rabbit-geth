package lqc

import (
	"math/big"
	"os"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vesting"
	"github.com/ethereum/go-ethereum/params"
	"github.com/holiman/uint256"
)

func TestBlockRewardEraBoundaries(t *testing.T) {
	engine := &LQC{config: &params.LQCConfig{EraLength: 100}}
	tests := []struct {
		block uint64
		want  uint64
	}{
		{block: 1, want: 1_200_000_000_000_000_000},
		{block: 99, want: 1_200_000_000_000_000_000},
		{block: 100, want: 600_000_000_000_000_000},
		{block: 200, want: 300_000_000_000_000_000},
		{block: 300, want: 150_000_000_000_000_000},
		{block: 900, want: 150_000_000_000_000_000},
	}
	for _, test := range tests {
		header := &types.Header{Number: new(big.Int).SetUint64(test.block)}
		if got := engine.blockRewardFor(header); got.Uint64() != test.want {
			t.Fatalf("block %d: got %d want %d", test.block, got.Uint64(), test.want)
		}
	}
}

func TestFinalizePaysImmediateRewardsAndConfiguredCommittee(t *testing.T) {
	ResetRuntimeRegistry()
	t.Cleanup(ResetRuntimeRegistry)

	participants := []common.Address{
		common.HexToAddress("0x0000000000000000000000000000000000002001"),
		common.HexToAddress("0x0000000000000000000000000000000000002002"),
		common.HexToAddress("0x0000000000000000000000000000000000002003"),
		common.HexToAddress("0x0000000000000000000000000000000000002004"),
		common.HexToAddress("0x0000000000000000000000000000000000002005"),
	}
	engine := &LQC{config: &params.LQCConfig{
		RegistryMode:          "bootstrap",
		BootstrapParticipants: participants,
		CommitteeRatioBps:     3_000,
		CommitteeSize:         2,
		FallbackCount:         1,
		EraLength:             100,
	}}
	header := &types.Header{
		Number:     big.NewInt(1),
		ParentHash: common.HexToHash("0x1234"),
	}
	selection := engine.selectionForHeader(nil, header)
	if selection.Producer == nil {
		t.Fatal("expected producer")
	}
	if len(selection.Fallbacks) != 1 {
		t.Fatalf("fallbacks: got %d want 1", len(selection.Fallbacks))
	}
	if len(selection.Committee) != 2 {
		t.Fatalf("committee: got %d want 2", len(selection.Committee))
	}
	header.Coinbase = selection.Producer.Address

	statedb, err := state.New(types.EmptyRootHash, state.NewDatabaseForTesting())
	if err != nil {
		t.Fatalf("create state: %v", err)
	}
	engine.Finalize(nil, header, statedb, new(types.Body), 0, nil)
	statedb.IntermediateRoot(true)

	producerWant := uint256.NewInt(840_000_000_000_000_000)
	if got := statedb.GetBalance(header.Coinbase); got.Cmp(producerWant) != 0 {
		t.Fatalf("producer locked: got %s want %s", got, producerWant)
	}
	committeeWant := uint256.NewInt(180_000_000_000_000_000)
	for _, member := range selection.Committee {
		if got := statedb.GetBalance(member.Address); got.Cmp(committeeWant) != 0 {
			t.Fatalf("committee %s locked: got %s want %s", member.Address, got, committeeWant)
		}
	}
	for _, fallback := range selection.Fallbacks {
		if got := statedb.GetBalance(fallback.Address); !got.IsZero() {
			t.Fatalf("fallback %s unexpectedly rewarded: %s", fallback.Address, got)
		}
	}

	total := new(uint256.Int).Set(statedb.GetBalance(header.Coinbase))
	for _, member := range selection.Committee {
		total.Add(total, statedb.GetBalance(member.Address))
	}
	wantTotal := uint256.NewInt(1_200_000_000_000_000_000)
	if total.Cmp(wantTotal) != 0 {
		t.Fatalf("total locked: got %s want %s", total, wantTotal)
	}
}

func TestFinalizePaysProducerFullyWithoutCommittee(t *testing.T) {
	ResetRuntimeRegistry()
	t.Cleanup(ResetRuntimeRegistry)

	statedb, err := state.New(types.EmptyRootHash, state.NewDatabaseForTesting())
	if err != nil {
		t.Fatalf("create state: %v", err)
	}
	producer := common.HexToAddress("0x0000000000000000000000000000000000003456")
	header := &types.Header{Number: big.NewInt(1), Coinbase: producer}
	new(LQC).Finalize(nil, header, statedb, new(types.Body), 0, nil)
	statedb.IntermediateRoot(true)

	want := uint256.NewInt(1_200_000_000_000_000_000)
	if got := statedb.GetBalance(producer); got.Cmp(want) != 0 {
		t.Fatalf("producer locked: got %s want %s", got, want)
	}
}

func TestLegacyRewardLockerDelegatesToCanonicalVesting(t *testing.T) {
	statedb, err := state.New(types.EmptyRootHash, state.NewDatabaseForTesting())
	if err != nil {
		t.Fatalf("create state: %v", err)
	}
	recipient := common.HexToAddress("0x0000000000000000000000000000000000004567")
	CreditReward(statedb, recipient, uint256.NewInt(7), 1)
	statedb.IntermediateRoot(true)

	if got := vesting.GetLockedBalance(statedb, recipient); got.Uint64() != 7 {
		t.Fatalf("canonical locked: got %d want 7", got.Uint64())
	}
	if count := vesting.VestingAddressCount(statedb); count != 1 {
		t.Fatalf("canonical vesting index: got %d want 1", count)
	}
	if got := vesting.VestingAddressAt(statedb, 0); got != recipient {
		t.Fatalf("canonical indexed address: got %s want %s", got, recipient)
	}
}

func TestCommitteeRemainderConservesEveryWei(t *testing.T) {
	ResetRuntimeRegistry()
	t.Cleanup(ResetRuntimeRegistry)

	participants := make([]common.Address, 9)
	for i := range participants {
		participants[i] = common.BigToAddress(big.NewInt(int64(0x3001 + i)))
	}
	engine := &LQC{config: &params.LQCConfig{
		RegistryMode:          "bootstrap",
		BootstrapParticipants: participants,
		CommitteeRatioBps:     3_000,
		CommitteeSize:         7,
		FallbackCount:         1,
		EraLength:             100,
	}}
	header := &types.Header{
		Number:     big.NewInt(300),
		ParentHash: common.HexToHash("0x5678"),
	}
	selection := engine.selectionForHeader(nil, header)
	if selection.Producer == nil {
		t.Fatal("expected producer")
	}
	if len(selection.Committee) != 7 {
		t.Fatalf("committee: got %d want 7", len(selection.Committee))
	}
	header.Coinbase = selection.Producer.Address

	statedb, err := state.New(types.EmptyRootHash, state.NewDatabaseForTesting())
	if err != nil {
		t.Fatalf("create state: %v", err)
	}
	engine.Finalize(nil, header, statedb, new(types.Body), 0, nil)

	producerWant := uint256.NewInt(105_000_000_000_000_000)
	if got := statedb.GetBalance(header.Coinbase); got.Cmp(producerWant) != 0 {
		t.Fatalf("producer locked: got %s want %s", got, producerWant)
	}
	perMember := uint256.NewInt(6_428_571_428_571_428)
	total := new(uint256.Int).Set(statedb.GetBalance(header.Coinbase))
	for i, member := range selection.Committee {
		want := new(uint256.Int).Set(perMember)
		if i == 0 {
			want.Add(want, uint256.NewInt(4))
		}
		got := statedb.GetBalance(member.Address)
		if got.Cmp(want) != 0 {
			t.Fatalf("committee %d locked: got %s want %s", i, got, want)
		}
		total.Add(total, got)
	}
	wantTotal := uint256.NewInt(150_000_000_000_000_000)
	if total.Cmp(wantTotal) != 0 {
		t.Fatalf("total locked: got %s want %s", total, wantTotal)
	}
}

func TestFinalizeDoesNotRouteMiningRewardsThroughLegacyVesting(t *testing.T) {
	raw, err := os.ReadFile("lqc.go")
	if err != nil {
		t.Fatalf("read lqc.go: %v", err)
	}
	source := string(raw)
	if strings.Contains(source, "vesting.CreditReward(") {
		t.Fatal("active LCQ still routes mining rewards through legacy vesting")
	}
	if strings.Contains(source, "vesting.ReleaseAllUnlockedRewards(") {
		t.Fatal("active LCQ still executes legacy global vesting release")
	}
}
