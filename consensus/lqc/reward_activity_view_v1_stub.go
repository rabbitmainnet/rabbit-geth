//go:build (!rabbit_workv1_engine_lab && !rabbit_workv1) || !rabbit_randomx

package lqc

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core/types"
)

func (l *LQC) RewardActivityForHeader(
	chain consensus.ChainHeaderReader,
	header *types.Header,
	participant common.Address,
) (RewardActivityViewV1, error) {
	return RewardActivityViewV1{
		ProducerReward:   new(big.Int),
		CommitteeCredits: make([]CommitteeRewardCreditViewV1, 0),
	}, ErrRewardActivityUnavailableV1
}
