package lqc

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vesting"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/holiman/uint256"
)

// Deprecated: vesting state belongs to core/vesting. These aliases and
// forwarding functions preserve the old lqc package API without duplicating
// consensus state logic.
const (
	LockedMiningEndBlock = vesting.LockedMiningEndBlock
	UnlockInstallments   = vesting.UnlockInstallments
	VestingStartBlock    = vesting.VestingStartBlock
	BlocksPerYear        = vesting.BlocksPerYear
	BlocksPerQuarter     = vesting.BlocksPerQuarter
)

func ShouldLockReward(block uint64) bool {
	return vesting.ShouldLockReward(block)
}

func GetLockedBalance(st vm.StateDB, addr common.Address) *uint256.Int {
	return vesting.GetLockedBalance(st, addr)
}

func SetLockedBalance(st vm.StateDB, addr common.Address, amount *uint256.Int) {
	vesting.SetLockedBalance(st, addr, amount)
}

func GetReleasedStage(st vm.StateDB, addr common.Address) uint8 {
	return vesting.GetReleasedStage(st, addr)
}

func SetReleasedStage(st vm.StateDB, addr common.Address, stage uint8) {
	vesting.SetReleasedStage(st, addr, stage)
}

func GetOriginalLockedBalance(st vm.StateDB, addr common.Address) *uint256.Int {
	return vesting.GetOriginalLockedBalance(st, addr)
}

func SetOriginalLockedBalance(st vm.StateDB, addr common.Address, amount *uint256.Int) {
	vesting.SetOriginalLockedBalance(st, addr, amount)
}

func AddLockedBalance(st vm.StateDB, addr common.Address, amount *uint256.Int) {
	vesting.AddLockedBalance(st, addr, amount)
}

func CreditReward(st vm.StateDB, addr common.Address, amount *uint256.Int, block uint64) {
	vesting.CreditReward(st, addr, amount, block)
}

func CurrentReleaseStage(block uint64) uint8 {
	return vesting.CurrentReleaseStage(block)
}

func ReleaseUnlockedRewards(st vm.StateDB, addr common.Address, block uint64) {
	vesting.ReleaseUnlockedRewards(st, addr, block)
}

func TargetReleasedAmount(original *uint256.Int, stage uint8) *uint256.Int {
	return vesting.TargetReleasedAmount(original, stage)
}
