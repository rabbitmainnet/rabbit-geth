package lqc

import (
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

var ErrRewardActivityUnavailableV1 = errors.New(
	"lqc canonical reward activity unavailable",
)

type CommitteeRewardCreditViewV1 struct {
	TargetBlock uint64
	Amount      *big.Int
}

type RewardActivityViewV1 struct {
	Producer           common.Address
	ProducerReward     *big.Int
	ProducerFullReward bool
	CommitteeCredits   []CommitteeRewardCreditViewV1
}
