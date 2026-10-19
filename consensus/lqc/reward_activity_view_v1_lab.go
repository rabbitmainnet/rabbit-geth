//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package lqc

import (
	"errors"
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
	out := RewardActivityViewV1{
		ProducerReward:   new(big.Int),
		CommitteeCredits: make([]CommitteeRewardCreditViewV1, 0),
	}

	if l == nil ||
		chain == nil ||
		header == nil ||
		header.Number == nil ||
		header.Number.Sign() <= 0 ||
		participant == (common.Address{}) {
		return out, ErrRewardActivityUnavailableV1
	}

	out.Producer = header.Coinbase

	selection, mode := l.workV1EngineLabRewardSelection(
		chain,
		header,
	)

	totalReward := l.blockRewardFor(header)

	switch mode {
	case workV1EngineLabRewardLegacy:
		committee := l.rewardCommitteeAddresses(chain, header)
		out.ProducerFullReward = len(committee) == 0

		if totalReward != nil && !totalReward.IsZero() {
			if out.ProducerFullReward {
				out.ProducerReward = totalReward.ToBig()
			} else {
				producerBps := uint64(7000)
				if l.config != nil && l.config.CommitteeRatioBps > 0 {
					producerBps = 10000 - l.config.CommitteeRatioBps
				}
				if producerBps > 10000 {
					producerBps = 0
				}
				amount := new(big.Int).Set(totalReward.ToBig())
				amount.Mul(amount, new(big.Int).SetUint64(producerBps))
				amount.Div(amount, big.NewInt(10000))
				out.ProducerReward = amount
			}
		}
		return out, nil

	case workV1EngineLabRewardEmergencyNoSubsidy:
		return out, nil

	case workV1EngineLabRewardSeats:
		out.ProducerFullReward = len(selection.Committee) == 0
		producerReward := workV1EngineLabProducerRewardForSelectionV3(
			totalReward,
			selection,
		)
		if producerReward != nil {
			out.ProducerReward = producerReward.ToBig()
		}

	default:
		return out, ErrRewardActivityUnavailableV1
	}

	envelope, resolver, verified, err :=
		l.workV1EngineLabVerifiedClaimsForRewardV3(
			chain,
			header,
		)
	if err != nil {
		if errors.Is(err, ErrUnsupportedLQCHeaderV4) {
			return out, nil
		}
		return out, err
	}

	for _, group := range envelope.CommitteeParticipationClaims {
		claimContext, err := resolver(group.TargetBlock)
		if err != nil {
			return out, err
		}

		targetHeader := workV1EngineLabAncestorHeader(
			chain,
			header.Number.Uint64()-1,
			header.ParentHash,
			group.TargetBlock,
		)
		if targetHeader == nil {
			return out, ErrRewardActivityUnavailableV1
		}

		result, err := CommitteeClaimRewardCreditsV1(
			l.blockRewardFor(targetHeader),
			claimContext.Committee,
			verifiedCommitteeClaimsForTargetV3(
				verified,
				group.TargetBlock,
			),
		)
		if err != nil {
			return out, err
		}

		for _, credit := range result.Credits {
			if credit.Address != participant ||
				credit.Amount == nil ||
				credit.Amount.IsZero() {
				continue
			}

			out.CommitteeCredits = append(
				out.CommitteeCredits,
				CommitteeRewardCreditViewV1{
					TargetBlock: group.TargetBlock,
					Amount:      credit.Amount.ToBig(),
				},
			)
		}
	}

	return out, nil
}
