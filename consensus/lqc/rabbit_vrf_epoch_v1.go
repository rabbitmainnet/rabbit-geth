package lqc

import "errors"

const (
	// RabbitVRFDKGPreparationWorkEpochsV1 reserves one complete Work epoch
	// between deterministic committee availability and the intended VRF target
	// epoch.
	RabbitVRFDKGPreparationWorkEpochsV1 uint64 = 1

	// A Work epoch N first becomes a canonical role-selection source during
	// Work epoch N+2. Rabbit VRF then reserves that full N+2 epoch for DKG, so
	// the resulting session is permanently bound to target VRF epoch N+3.
	RabbitVRFSourceToTargetEpochDelayV1 uint64 = WorkSelectionDelayEpochsV1 +
		RabbitVRFDKGPreparationWorkEpochsV1
)

var ErrInvalidRabbitVRFEpochV1 = errors.New(
	"invalid rabbit vrf epoch v1",
)

// RabbitVRFDKGPreparationEpochForSourceV1 returns the Work epoch during which
// the committee derived from source Work epoch N performs its DKG.
//
// Work source N:
//
//	generated:             N
//	committed/closed:     N+1
//	selection available:  N+2
//	DKG preparation:      N+2
//	target VRF epoch:     N+3
//
// The preparation epoch is therefore the same epoch in which the delayed
// canonical Work selection source first becomes available.
func RabbitVRFDKGPreparationEpochForSourceV1(
	sourceWorkEpoch uint64,
) (uint64, error) {
	if sourceWorkEpoch == 0 ||
		sourceWorkEpoch >
			^uint64(0)-WorkSelectionDelayEpochsV1 {
		return 0, ErrInvalidRabbitVRFEpochV1
	}

	return sourceWorkEpoch +
			WorkSelectionDelayEpochsV1,
		nil
}

// RabbitVRFTargetEpochForSourceWorkEpochV1 deterministically binds a source
// Work epoch to exactly one intended VRF epoch.
//
// A failed ceremony MUST NOT silently retarget this same DKG session to another
// epoch. Final keyset qualification/activation policy is handled separately.
func RabbitVRFTargetEpochForSourceWorkEpochV1(
	sourceWorkEpoch uint64,
) (uint64, error) {
	preparationEpoch, err :=
		RabbitVRFDKGPreparationEpochForSourceV1(
			sourceWorkEpoch,
		)
	if err != nil {
		return 0, err
	}

	if preparationEpoch >
		^uint64(0)-RabbitVRFDKGPreparationWorkEpochsV1 {
		return 0, ErrInvalidRabbitVRFEpochV1
	}

	return preparationEpoch +
			RabbitVRFDKGPreparationWorkEpochsV1,
		nil
}

// RabbitVRFSourceWorkEpochForTargetEpochV1 reverses the canonical mapping.
//
// VRF epoch numbering is aligned one-for-one with Work epoch numbering. Epochs
// before the first possible delayed DKG target simply have no source committee.
// This does not imply Rabbit VRF was active during those historical epochs.
func RabbitVRFSourceWorkEpochForTargetEpochV1(
	targetVRFEpoch uint64,
) (
	uint64,
	bool,
	error,
) {
	if targetVRFEpoch == 0 {
		return 0,
			false,
			ErrInvalidRabbitVRFEpochV1
	}

	if targetVRFEpoch <=
		RabbitVRFSourceToTargetEpochDelayV1 {
		return 0, false, nil
	}

	return targetVRFEpoch -
			RabbitVRFSourceToTargetEpochDelayV1,
		true,
		nil
}

// RabbitVRFEpochForBlockV1 intentionally shares Work V1 epoch boundaries.
func RabbitVRFEpochForBlockV1(
	blockNumber,
	epochLength uint64,
) (uint64, error) {
	epoch, err :=
		WorkEpochForBlockV1(
			blockNumber,
			epochLength,
		)
	if err != nil {
		return 0, ErrInvalidRabbitVRFEpochV1
	}

	return epoch, nil
}

// RabbitVRFEpochStartBlockV1 intentionally shares Work V1 epoch boundaries.
func RabbitVRFEpochStartBlockV1(
	vrfEpoch,
	epochLength uint64,
) (uint64, error) {
	block, err :=
		WorkEpochStartBlockV1(
			vrfEpoch,
			epochLength,
		)
	if err != nil {
		return 0, ErrInvalidRabbitVRFEpochV1
	}

	return block, nil
}

func RabbitVRFDKGPreparationStartBlockV1(
	sourceWorkEpoch,
	epochLength uint64,
) (uint64, error) {
	preparationEpoch, err :=
		RabbitVRFDKGPreparationEpochForSourceV1(
			sourceWorkEpoch,
		)
	if err != nil {
		return 0, err
	}

	return RabbitVRFEpochStartBlockV1(
		preparationEpoch,
		epochLength,
	)
}

func RabbitVRFTargetEpochStartBlockForSourceV1(
	sourceWorkEpoch,
	epochLength uint64,
) (uint64, error) {
	targetEpoch, err :=
		RabbitVRFTargetEpochForSourceWorkEpochV1(
			sourceWorkEpoch,
		)
	if err != nil {
		return 0, err
	}

	return RabbitVRFEpochStartBlockV1(
		targetEpoch,
		epochLength,
	)
}
