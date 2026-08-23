//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package lqc

import (
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/holiman/uint256"
)

type workV1EngineLabRewardMode uint8

const (
	workV1EngineLabRewardLegacy workV1EngineLabRewardMode = iota
	workV1EngineLabRewardSeats
	workV1EngineLabRewardEmergencyNoSubsidy
)

func workV1EngineLabRewardModeForSource(
	hasWorkSource bool,
	hasEligibleSeats bool,
) workV1EngineLabRewardMode {
	if !hasWorkSource {
		return workV1EngineLabRewardLegacy
	}
	if hasEligibleSeats {
		return workV1EngineLabRewardSeats
	}
	return workV1EngineLabRewardEmergencyNoSubsidy
}

func workV1EngineLabCommitteeBps(l *LQC) uint64 {
	committeeBps := uint64(3000)
	if l != nil &&
		l.config != nil &&
		l.config.CommitteeRatioBps > 0 {
		committeeBps = l.config.CommitteeRatioBps
	}
	if committeeBps > 10000 {
		committeeBps = 10000
	}
	return committeeBps
}

// workV1EngineLabSeatRewardCredits calculates reward weight BY COMMITTEE SEAT.
//
// committee is intentionally not deduplicated and the producer is intentionally
// not excluded. If the producer owns committee seats, it receives the normal
// producer share plus those seat shares.
//
// The integer remainder goes to the first deterministic committee seat, then
// transfers are aggregated by address. This preserves exact wei conservation.
func workV1EngineLabSeatRewardCredits(
	totalReward *uint256.Int,
	producer common.Address,
	committee []HybridParticipant,
	committeeBps uint64,
) []WorkSeatRewardV1 {
	if totalReward == nil ||
		totalReward.IsZero() ||
		producer == (common.Address{}) {
		return nil
	}
	if committeeBps > 10000 {
		committeeBps = 10000
	}

	amounts := make(map[common.Address]*uint256.Int)

	add := func(address common.Address, amount *uint256.Int) {
		if address == (common.Address{}) ||
			amount == nil ||
			amount.IsZero() {
			return
		}
		current := amounts[address]
		if current == nil {
			current = uint256.NewInt(0)
			amounts[address] = current
		}
		current.Add(current, amount)
	}

	producerBps := uint64(10000 - committeeBps)
	producerReward := new(uint256.Int).Set(totalReward)
	producerReward.Mul(
		producerReward,
		uint256.NewInt(producerBps),
	)
	producerReward.Div(
		producerReward,
		uint256.NewInt(10000),
	)

	committeeReward := new(uint256.Int).Set(totalReward)
	committeeReward.Sub(committeeReward, producerReward)

	validSeats := make([]common.Address, 0, len(committee))
	for _, seat := range committee {
		if seat.Address != (common.Address{}) {
			validSeats = append(validSeats, seat.Address)
		}
	}

	if len(validSeats) == 0 || committeeReward.IsZero() {
		add(producer, totalReward)
	} else {
		add(producer, producerReward)

		perSeat := new(uint256.Int).Set(committeeReward)
		perSeat.Div(
			perSeat,
			uint256.NewInt(uint64(len(validSeats))),
		)

		allocated := new(uint256.Int).Set(perSeat)
		allocated.Mul(
			allocated,
			uint256.NewInt(uint64(len(validSeats))),
		)

		remainder := new(uint256.Int).Set(committeeReward)
		remainder.Sub(remainder, allocated)

		for index, address := range validSeats {
			amount := new(uint256.Int).Set(perSeat)
			if index == 0 {
				amount.Add(amount, remainder)
			}
			add(address, amount)
		}
	}

	addresses := make([]common.Address, 0, len(amounts))
	for address := range amounts {
		addresses = append(addresses, address)
	}
	sort.Slice(addresses, func(i, j int) bool {
		return addresses[i].Cmp(addresses[j]) < 0
	})

	out := make([]WorkSeatRewardV1, 0, len(addresses))
	for _, address := range addresses {
		out = append(out, WorkSeatRewardV1{
			Address: address,
			Amount:  new(uint256.Int).Set(amounts[address]),
		})
	}
	return out
}

func workV1EngineLabRewardTotalV1(
	credits []WorkSeatRewardV1,
) *uint256.Int {
	total := uint256.NewInt(0)
	for _, credit := range credits {
		if credit.Amount != nil {
			total.Add(total, credit.Amount)
		}
	}
	return total
}

func workV1EngineLabRewardCreditsForAuthor(
	selection HybridSelection,
	author common.Address,
	totalReward *uint256.Int,
	committeeBps uint64,
) ([]WorkSeatRewardV1, bool) {
	if author == (common.Address{}) || totalReward == nil || totalReward.IsZero() {
		return nil, false
	}
	allowed, _ := IsAuthorAllowed(selection, author)
	if !allowed {
		return nil, false
	}
	credits := workV1EngineLabSeatRewardCredits(
		totalReward,
		author,
		selection.Committee,
		committeeBps,
	)
	return credits, workV1EngineLabRewardTotalV1(credits).Cmp(totalReward) == 0
}

func (l *LQC) workV1EngineLabRewardSelection(
	chain consensus.ChainHeaderReader,
	header *types.Header,
) (
	HybridSelection,
	workV1EngineLabRewardMode,
) {
	if chain == nil ||
		header == nil ||
		header.Number == nil ||
		header.Number.Sign() <= 0 {
		return HybridSelection{}, workV1EngineLabRewardLegacy
	}
	if header.Number.Uint64() > 1 && l.openActivationForHeader(chain, header) {
		return HybridSelection{}, workV1EngineLabRewardEmergencyNoSubsidy
	}

	_, hasSource, err := WorkSelectionSourceEpochV1(
		header.Number.Uint64(),
		WorkProtocolEpochLengthV1,
	)
	if err != nil || !hasSource {
		return HybridSelection{}, workV1EngineLabRewardLegacy
	}

	// Once a Work source is expected, inability to derive eligible WorkSeats is
	// fail-closed economically in this LAB path: keep liveness via the registry
	// fallback, but do not mint the base subsidy.
	parentRuntime, err := l.workV1EngineLabRuntimeAt(
		chain,
		header.Number.Uint64()-1,
		header.ParentHash,
	)
	if err != nil {
		return HybridSelection{},
			workV1EngineLabRewardEmergencyNoSubsidy
	}
	parentRegistry, err := l.registryParentSnapshot(
		chain,
		header,
	)
	if err != nil {
		return HybridSelection{},
			workV1EngineLabRewardEmergencyNoSubsidy
	}

	selection, active, err := l.workV1EngineLabSelectionForHeader(
		chain,
		parentRuntime,
		parentRegistry,
		header,
	)
	if err != nil {
		return HybridSelection{},
			workV1EngineLabRewardEmergencyNoSubsidy
	}

	mode := workV1EngineLabRewardModeForSource(true, active)
	return selection, mode
}

// distributeWorkV1RewardsMaybeLab returns true when the Work V1 policy has
// fully handled the base block subsidy.
//
// Before Work selection exists (epochs 1-2), false preserves the old bootstrap
// reward path. After that:
//   - WorkSeat mode pays 70/30 (or configured ratio) by seat;
//   - zero eligible seats uses registry only for liveness and mints NO subsidy.
func (l *LQC) distributeWorkV1RewardsMaybeLab(
	chain consensus.ChainHeaderReader,
	header *types.Header,
	statedb vm.StateDB,
	totalReward *uint256.Int,
) bool {
	if header == nil ||
		header.Number == nil ||
		statedb == nil ||
		totalReward == nil ||
		totalReward.IsZero() {
		return false
	}

	selection, mode := l.workV1EngineLabRewardSelection(
		chain,
		header,
	)
	switch mode {
	case workV1EngineLabRewardLegacy:
		return false

	case workV1EngineLabRewardEmergencyNoSubsidy:
		// Deliberate emergency policy:
		// keep producing blocks through registry fallback, but do not mint the
		// normal block subsidy while no eligible WorkSeats exist.
		return true

	case workV1EngineLabRewardSeats:
		credits, ok := workV1EngineLabRewardCreditsForAuthor(
			selection,
			header.Coinbase,
			totalReward,
			workV1EngineLabCommitteeBps(l),
		)
		if !ok {
			// Header verification should make this unreachable. Fail closed:
			// never reward an unauthorized author or mint a partial subsidy.
			return true
		}
		for _, credit := range credits {
			if credit.Amount == nil || credit.Amount.IsZero() {
				continue
			}
			statedb.AddBalance(
				credit.Address,
				credit.Amount,
				tracing.BalanceIncreaseRewardMineBlock,
			)
		}
		return true

	default:
		return true
	}
}
