package vesting

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/holiman/uint256"
)

func TestLockedRewardSurvivesEIP158Finalization(t *testing.T) {
	statedb, err := state.New(types.EmptyRootHash, state.NewDatabaseForTesting())
	if err != nil {
		t.Fatalf("create state: %v", err)
	}
	recipient := common.HexToAddress("0x0000000000000000000000000000000000001234")
	reward := uint256.NewInt(1_200_000_000_000_000_000)

	CreditReward(statedb, recipient, reward, 1)
	statedb.IntermediateRoot(true)

	if nonce := statedb.GetNonce(vestingSystemAddress); nonce != 1 {
		t.Fatalf("vesting system nonce: got %d want 1", nonce)
	}
	if got := GetLockedBalance(statedb, recipient); got.Cmp(reward) != 0 {
		t.Fatalf("locked reward: got %s want %s", got, reward)
	}
	if got := GetOriginalLockedBalance(statedb, recipient); got.Cmp(reward) != 0 {
		t.Fatalf("original locked reward: got %s want %s", got, reward)
	}
	if count := VestingAddressCount(statedb); count != 1 {
		t.Fatalf("vesting address count: got %d want 1", count)
	}
	if got := VestingAddressAt(statedb, 0); got != recipient {
		t.Fatalf("vesting address: got %s want %s", got, recipient)
	}
}

func TestRewardLockBoundary(t *testing.T) {
	statedb, err := state.New(types.EmptyRootHash, state.NewDatabaseForTesting())
	if err != nil {
		t.Fatalf("create state: %v", err)
	}
	recipient := common.HexToAddress("0x0000000000000000000000000000000000001234")

	CreditReward(statedb, recipient, uint256.NewInt(7), LockedMiningEndBlock)
	CreditReward(statedb, recipient, uint256.NewInt(3), LockedMiningEndBlock+1)

	if got := GetLockedBalance(statedb, recipient); got.Uint64() != 7 {
		t.Fatalf("locked at boundary: got %d want 7", got.Uint64())
	}
	if got := statedb.GetBalance(recipient); got.Uint64() != 3 {
		t.Fatalf("liquid after boundary: got %d want 3", got.Uint64())
	}
}

func TestReleaseBoundariesAndRemainder(t *testing.T) {
	statedb, err := state.New(types.EmptyRootHash, state.NewDatabaseForTesting())
	if err != nil {
		t.Fatalf("create state: %v", err)
	}
	recipient := common.HexToAddress("0x0000000000000000000000000000000000001234")
	CreditReward(statedb, recipient, uint256.NewInt(7), 1)

	boundaries := []uint64{
		VestingStartBlock + BlocksPerYear,
		VestingStartBlock + BlocksPerYear + BlocksPerQuarter,
		VestingStartBlock + BlocksPerYear + BlocksPerQuarter*2,
		VestingStartBlock + BlocksPerYear + BlocksPerQuarter*3,
	}
	wantLiquid := []uint64{1, 3, 5, 7}
	wantLocked := []uint64{6, 4, 2, 0}

	ReleaseUnlockedRewards(statedb, recipient, boundaries[0]-1)
	if got := statedb.GetBalance(recipient); !got.IsZero() {
		t.Fatalf("released before first boundary: %s", got)
	}
	for i, block := range boundaries {
		ReleaseUnlockedRewards(statedb, recipient, block)
		if got := statedb.GetBalance(recipient); got.Uint64() != wantLiquid[i] {
			t.Fatalf("stage %d liquid: got %d want %d", i+1, got.Uint64(), wantLiquid[i])
		}
		if got := GetLockedBalance(statedb, recipient); got.Uint64() != wantLocked[i] {
			t.Fatalf("stage %d locked: got %d want %d", i+1, got.Uint64(), wantLocked[i])
		}
		if got := GetReleasedStage(statedb, recipient); got != uint8(i+1) {
			t.Fatalf("stage marker: got %d want %d", got, i+1)
		}
	}
}
