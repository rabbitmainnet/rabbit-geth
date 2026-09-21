package lqc

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

// RabbitVRFDKGBridgeV1 is the deterministic bridge from one CLOSED canonical
// Work source epoch to the immutable DKG session prepared for its target VRF
// epoch.
//
// DatasetKey is supplied by the canonical chain runtime. The bridge deliberately
// does not derive it from WorkEpochSnapshotV1.Anchor: that field is the Work
// challenge/commit anchor, while the RandomX dataset anchor follows its own
// canonical epoch schedule.
type RabbitVRFDKGBridgeV1 struct {
	SourceWorkEpoch  uint64
	PreparationEpoch uint64
	TargetVRFEpoch   uint64
	SelectionRoot    common.Hash
	DatasetKey       common.Hash
	Entropy          common.Hash
	CommitteeSeed    common.Hash
	CommitteeRoot    common.Hash
	Members          []RabbitVRFCommitteeMemberV1
	Session          RabbitVRFDKGSessionContextV1
	SessionID        common.Hash
}

// RabbitVRFDKGBridgeForSnapshotV1 composes the existing canonical Work entropy,
// Rabbit VRF committee commitment, epoch schedule and DKG session rules.
//
// It introduces no new randomness, committee ordering, threshold policy or
// epoch-retargeting rule.
func RabbitVRFDKGBridgeForSnapshotV1(
	chainID *big.Int,
	snapshot *WorkEpochSnapshotV1,
	datasetKey common.Hash,
	minSize uint64,
	maxSize uint64,
	hasher WorkSelectionBeaconHasherV1,
) (
	RabbitVRFDKGBridgeV1,
	error,
) {
	var out RabbitVRFDKGBridgeV1

	if snapshot == nil {
		return out, ErrInvalidRabbitVRFCommitteeV1
	}

	if err := snapshot.Validate(chainID); err != nil {
		return out, err
	}

	entropy, err :=
		DeriveWorkSelectionEntropyV1(
			chainID,
			snapshot.Epoch,
			snapshot.Root,
			datasetKey,
			hasher,
		)
	if err != nil {
		return out, err
	}

	committeeRoot, committeeSeed, members, err :=
		RabbitVRFCommitteeCommitmentForSnapshotV1(
			chainID,
			snapshot,
			entropy,
			minSize,
			maxSize,
		)
	if err != nil {
		return out, err
	}

	preparationEpoch, err :=
		RabbitVRFDKGPreparationEpochForSourceV1(
			snapshot.Epoch,
		)
	if err != nil {
		return out, err
	}

	targetVRFEpoch, err :=
		RabbitVRFTargetEpochForSourceWorkEpochV1(
			snapshot.Epoch,
		)
	if err != nil {
		return out, err
	}

	session, err :=
		NewRabbitVRFDKGSessionContextV1(
			chainID,
			targetVRFEpoch,
			committeeRoot,
			uint64(len(members)),
		)
	if err != nil {
		return out, err
	}

	sessionID, err :=
		RabbitVRFDKGSessionIDV1(
			session,
		)
	if err != nil {
		return out, err
	}

	out = RabbitVRFDKGBridgeV1{
		SourceWorkEpoch:  snapshot.Epoch,
		PreparationEpoch: preparationEpoch,
		TargetVRFEpoch:   targetVRFEpoch,
		SelectionRoot:    snapshot.Root,
		DatasetKey:       datasetKey,
		Entropy:          entropy,
		CommitteeSeed:    committeeSeed,
		CommitteeRoot:    committeeRoot,
		Members:          members,
		Session:          session,
		SessionID:        sessionID,
	}

	return out, nil
}
