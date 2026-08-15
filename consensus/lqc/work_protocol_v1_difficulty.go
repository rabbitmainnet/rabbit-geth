package lqc

import (
	"errors"
	"math/big"
)

const (
	// WorkProtocolEpochLengthV1 matches Rabbit's current 128-block consensus
	// epoch and gives roughly 21 minutes per work epoch at a 10-second target.
	WorkProtocolEpochLengthV1 uint64 = 128

	// MaxWorkTicketsPerBlockV1 is deliberately verification-bound, not
	// header-space-bound. The 2026-08-12 lab measured RandomX LIGHT verification
	// as the limiting case; 8 proofs keeps the hard worst-case block workload
	// substantially below the 10-second block target on that test host.
	MaxWorkTicketsPerBlockV1 uint64 = 8

	// TargetWorkTicketsPerEpochV1 aims for two accepted tickets per block on
	// average. The commit window can carry four times that target, leaving
	// substantial burst/censorship-recovery headroom.
	TargetWorkTicketsPerEpochV1 uint64 = 256

	// WorkTicketCommitCapacityPerEpochV1 is the hard canonical transport
	// capacity when every block in the 128-block commit epoch is full.
	WorkTicketCommitCapacityPerEpochV1 uint64 = WorkProtocolEpochLengthV1 * MaxWorkTicketsPerBlockV1

	// Difficulty can rise by at most 4x per closed epoch and fall by at most 2x.
	// Faster upward response protects transport capacity; slower downward
	// response limits how quickly missing/censored tickets can make work easier.
	WorkDifficultyMaxIncreaseFactorV1 uint64 = 4
	WorkDifficultyMaxDecreaseFactorV1 uint64 = 2
)

var ErrInvalidWorkDifficultyV1 = errors.New("invalid lqc work difficulty v1")

// EffectiveWorkTicketCountV1 clamps the observed CLOSED canonical ticket count
// before retargeting.
//
// target/2 <= effective <= target*4
//
// This means:
//   - zero/very-low observed work can reduce difficulty by at most 2x;
//   - a saturated 1024-ticket commit epoch can raise difficulty by 4x.
func EffectiveWorkTicketCountV1(observed uint64) uint64 {
	minObserved := TargetWorkTicketsPerEpochV1 /
		WorkDifficultyMaxDecreaseFactorV1
	maxObserved := TargetWorkTicketsPerEpochV1 *
		WorkDifficultyMaxIncreaseFactorV1

	if observed < minObserved {
		return minObserved
	}
	if observed > maxObserved {
		return maxObserved
	}
	return observed
}

// NextWorkDifficultyV1 deterministically retargets from the number of CLOSED,
// canonically included work seats in the previous work epoch.
//
// next = current * effectiveObserved / target
//
// Integer division is rounded to nearest and difficulty is never allowed below
// one. This function is pure and contains no wall-clock or process-local input.
func NextWorkDifficultyV1(
	current *big.Int,
	observed uint64,
) (*big.Int, error) {
	if current == nil || current.Sign() <= 0 {
		return nil, ErrInvalidWorkDifficultyV1
	}

	effective := EffectiveWorkTicketCountV1(observed)
	target := new(big.Int).SetUint64(TargetWorkTicketsPerEpochV1)

	numerator := new(big.Int).Mul(
		new(big.Int).Set(current),
		new(big.Int).SetUint64(effective),
	)

	// Deterministic round-to-nearest for positive integers.
	halfTarget := new(big.Int).Rsh(new(big.Int).Set(target), 1)
	numerator.Add(numerator, halfTarget)

	next := new(big.Int).Div(numerator, target)
	if next.Sign() <= 0 {
		next.SetUint64(1)
	}
	return next, nil
}

// ValidateWorkProtocolProfileV1 protects the relationship between average
// target and hard commit capacity. V1 intentionally keeps 4x transport
// headroom over the expected ticket count.
func ValidateWorkProtocolProfileV1() error {
	if WorkProtocolEpochLengthV1 == 0 ||
		MaxWorkTicketsPerBlockV1 == 0 ||
		TargetWorkTicketsPerEpochV1 == 0 {
		return ErrInvalidWorkDifficultyV1
	}
	if WorkTicketCommitCapacityPerEpochV1 !=
		WorkProtocolEpochLengthV1*MaxWorkTicketsPerBlockV1 {
		return ErrInvalidWorkDifficultyV1
	}
	if WorkTicketCommitCapacityPerEpochV1 !=
		TargetWorkTicketsPerEpochV1*WorkDifficultyMaxIncreaseFactorV1 {
		return ErrInvalidWorkDifficultyV1
	}
	return nil
}
