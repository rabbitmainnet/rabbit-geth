package vesting

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/holiman/uint256"
)

const (
	LockedMiningEndBlock uint64 = 100000
	UnlockInstallments   uint64 = 4
)

var vestingSystemAddress = common.HexToAddress(
	"0x0000000000000000000000000000000000001001",
)

// ensureVestingSystemAccount prevents EIP-158 empty-account cleanup from
// deleting the vesting storage. Storage alone does not make an Ethereum
// account non-empty; a nonce does, without changing the monetary supply.
func ensureVestingSystemAccount(st vm.StateDB) {
	if st.GetNonce(vestingSystemAddress) == 0 {
		st.SetNonce(
			vestingSystemAddress,
			1,
			tracing.NonceChangeUnspecified,
		)
	}
}

func ShouldLockReward(block uint64) bool {
	return block > 0 && block <= LockedMiningEndBlock
}

func lockedBalanceSlot(addr common.Address) common.Hash {
	return crypto.Keccak256Hash(
		[]byte("RABBIT_VESTING_LOCKED_BALANCE"),
		addr.Bytes(),
	)
}

func releasedStageSlot(addr common.Address) common.Hash {
	return crypto.Keccak256Hash(
		[]byte("RABBIT_VESTING_RELEASE_STAGE"),
		addr.Bytes(),
	)
}

func originalLockedBalanceSlot(addr common.Address) common.Hash {
	return crypto.Keccak256Hash(
		[]byte("RABBIT_VESTING_ORIGINAL_BALANCE"),
		addr.Bytes(),
	)
}

func vestingIndexCountSlot() common.Hash {
	return crypto.Keccak256Hash(
		[]byte("RABBIT_VESTING_INDEX_COUNT"),
	)
}

func vestingIndexSlot(index uint64) common.Hash {
	return crypto.Keccak256Hash(
		[]byte("RABBIT_VESTING_INDEX"),
		new(big.Int).SetUint64(index).Bytes(),
	)
}

func vestingExistsSlot(addr common.Address) common.Hash {
	return crypto.Keccak256Hash(
		[]byte("RABBIT_VESTING_EXISTS"),
		addr.Bytes(),
	)
}

func VestingAddressCount(st vm.StateDB) uint64 {
	value := st.GetState(
		vestingSystemAddress,
		vestingIndexCountSlot(),
	)

	return value.Big().Uint64()
}

func SetVestingAddressCount(st vm.StateDB, count uint64) {
	st.SetState(
		vestingSystemAddress,
		vestingIndexCountSlot(),
		common.BigToHash(new(big.Int).SetUint64(count)),
	)
}

func VestingAddressAt(st vm.StateDB, index uint64) common.Address {
	value := st.GetState(
		vestingSystemAddress,
		vestingIndexSlot(index),
	)

	return common.BytesToAddress(value.Bytes())
}

func AddVestingAddress(st vm.StateDB, addr common.Address) {
	ensureVestingSystemAccount(st)

	exists := st.GetState(
		vestingSystemAddress,
		vestingExistsSlot(addr),
	)

	if exists.Big().Sign() != 0 {
		return
	}

	count := VestingAddressCount(st)

	st.SetState(
		vestingSystemAddress,
		vestingIndexSlot(count),
		common.BytesToHash(addr.Bytes()),
	)

	st.SetState(
		vestingSystemAddress,
		vestingExistsSlot(addr),
		common.BigToHash(big.NewInt(1)),
	)

	SetVestingAddressCount(st, count+1)
}

func GetLockedBalance(st vm.StateDB, addr common.Address) *uint256.Int {
	value := st.GetState(
		vestingSystemAddress,
		lockedBalanceSlot(addr),
	)

	return new(uint256.Int).SetBytes(value.Bytes())
}

func SetLockedBalance(
	st vm.StateDB,
	addr common.Address,
	amount *uint256.Int,
) {
	if amount == nil {
		amount = new(uint256.Int)
	}

	value := common.BytesToHash(amount.ToBig().Bytes())

	st.SetState(
		vestingSystemAddress,
		lockedBalanceSlot(addr),
		value,
	)
}

func GetReleasedStage(
	st vm.StateDB,
	addr common.Address,
) uint8 {

	value := st.GetState(
		vestingSystemAddress,
		releasedStageSlot(addr),
	)

	b := value.Bytes()

	if len(b) == 0 {
		return 0
	}

	return b[len(b)-1]
}

func SetReleasedStage(
	st vm.StateDB,
	addr common.Address,
	stage uint8,
) {

	st.SetState(
		vestingSystemAddress,
		releasedStageSlot(addr),
		common.BigToHash(new(big.Int).SetUint64(uint64(stage))),
	)
}

func GetOriginalLockedBalance(
	st vm.StateDB,
	addr common.Address,
) *uint256.Int {

	value := st.GetState(
		vestingSystemAddress,
		originalLockedBalanceSlot(addr),
	)

	return new(uint256.Int).SetBytes(value.Bytes())
}

func SetOriginalLockedBalance(
	st vm.StateDB,
	addr common.Address,
	amount *uint256.Int,
) {

	if amount == nil {
		amount = new(uint256.Int)
	}

	st.SetState(
		vestingSystemAddress,
		originalLockedBalanceSlot(addr),
		common.BytesToHash(amount.ToBig().Bytes()),
	)
}

func AddLockedBalance(
	st vm.StateDB,
	addr common.Address,
	amount *uint256.Int,
) {
	if amount == nil || amount.IsZero() {
		return
	}

	current := GetLockedBalance(st, addr)
	current.Add(current, amount)

	SetLockedBalance(st, addr, current)
	SetOriginalLockedBalance(st, addr, current)
}

func CreditReward(
	st vm.StateDB,
	addr common.Address,
	amount *uint256.Int,
	block uint64,
) {
	if amount == nil || amount.IsZero() {
		return
	}

	ReleaseUnlockedRewards(
		st,
		addr,
		block,
	)

	if ShouldLockReward(block) {
		AddVestingAddress(st, addr)
		AddLockedBalance(st, addr, amount)
		return
	}

	st.AddBalance(
		addr,
		amount,
		tracing.BalanceIncreaseRewardMineBlock,
	)
}

const VestingStartBlock uint64 = 100000

// 1 year in 10-second blocks.
const BlocksPerYear uint64 = 3153600

// 3 months.
const BlocksPerQuarter uint64 = BlocksPerYear / 4

func CurrentReleaseStage(block uint64) uint8 {

	if block < VestingStartBlock+BlocksPerYear {
		return 0
	}

	if block < VestingStartBlock+BlocksPerYear+BlocksPerQuarter {
		return 1
	}

	if block < VestingStartBlock+BlocksPerYear+BlocksPerQuarter*2 {
		return 2
	}

	if block < VestingStartBlock+BlocksPerYear+BlocksPerQuarter*3 {
		return 3
	}

	return 4
}

func ReleaseUnlockedRewards(
	st vm.StateDB,
	addr common.Address,
	block uint64,
) {

	currentStage := CurrentReleaseStage(block)
	releasedStage := GetReleasedStage(st, addr)

	if currentStage <= releasedStage {
		return
	}

	original := GetOriginalLockedBalance(st, addr)
	locked := GetLockedBalance(st, addr)

	if original.IsZero() || locked.IsZero() {
		return
	}

	target := TargetReleasedAmount(
		original,
		currentStage,
	)

	already := TargetReleasedAmount(
		original,
		releasedStage,
	)

	if target.Cmp(already) <= 0 {
		return
	}

	release := new(uint256.Int).Sub(target, already)

	if release.Cmp(locked) > 0 {
		release = new(uint256.Int).Set(locked)
	}

	remaining := new(uint256.Int).Sub(locked, release)

	SetLockedBalance(
		st,
		addr,
		remaining,
	)

	SetReleasedStage(
		st,
		addr,
		currentStage,
	)

	st.AddBalance(
		addr,
		release,
		tracing.BalanceIncreaseRewardMineBlock,
	)

}

func TargetReleasedAmount(
	original *uint256.Int,
	stage uint8,
) *uint256.Int {

	if original == nil {
		return new(uint256.Int)
	}

	result := new(uint256.Int).Set(original)

	switch stage {

	case 0:
		return new(uint256.Int)

	case 1:
		result.Div(result, uint256.NewInt(4))
		return result

	case 2:
		result.Div(result, uint256.NewInt(2))
		return result

	case 3:
		result.Mul(result, uint256.NewInt(3))
		result.Div(result, uint256.NewInt(4))
		return result

	default:
		return result
	}
}

func ReleaseAllUnlockedRewards(
	st vm.StateDB,
	block uint64,
) {
	count := VestingAddressCount(st)

	for i := uint64(0); i < count; i++ {
		addr := VestingAddressAt(st, i)

		if addr == (common.Address{}) {
			continue
		}

		ReleaseUnlockedRewards(
			st,
			addr,
			block,
		)
	}
}
