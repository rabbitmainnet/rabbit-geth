//go:build (!rabbit_workv1_engine_lab && !rabbit_workv1) || !rabbit_randomx

package lqc

import (
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/holiman/uint256"
)

func (l *LQC) distributeWorkV1RewardsMaybeLab(
	chain consensus.ChainHeaderReader,
	header *types.Header,
	statedb vm.StateDB,
	totalReward *uint256.Int,
) bool {
	return false
}
