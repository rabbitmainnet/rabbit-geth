package lqc

import (
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

// A failed session is never shrunk or retargeted. A subsequent source epoch
// constructs its own session from its own frozen canonical availability view.
func rabbitVRFAvailableBridgeV2(chainID *big.Int, snapshot *WorkEpochSnapshotV1,
	datasetKey common.Hash, registry *RegistrySnapshot, epochLength, window uint64,
	hasher WorkSelectionBeaconHasherV1) (RabbitVRFDKGBridgeV1, bool, error) {
	var out RabbitVRFDKGBridgeV1
	if snapshot == nil || registry == nil {
		return out, false, ErrInvalidRabbitVRFCommitteeV1
	}
	if err := snapshot.Validate(chainID); err != nil {
		return out, false, err
	}
	start, err := RabbitVRFDKGPreparationStartBlockV1(snapshot.Epoch, epochLength)
	if err != nil || start == 0 {
		return out, false, ErrInvalidRabbitVRFEpochV1
	}
	if registry.Number != start-1 {
		return out, false, errors.New("VRF V2 requires frozen preparation parent registry")
	}
	if _, err := registry.Registry(); err != nil {
		return out, false, err
	}
	candidates, err := rabbitVRFAvailableCandidatesV2(snapshot.Seats, registry, registry.Number, window)
	if err != nil {
		return out, false, err
	}
	if len(candidates) == 0 {
		return out, false, nil
	}
	entropy, err := DeriveWorkSelectionEntropyV1(chainID, snapshot.Epoch, snapshot.Root, datasetKey, hasher)
	if err != nil {
		return out, false, err
	}
	root, seed, members, err := rabbitVRFAvailableCommitteeWithSeedV2(chainID, snapshot, registry, registry.Number, window, 128, entropy)
	if err != nil {
		return out, false, err
	}
	prep, err := RabbitVRFDKGPreparationEpochForSourceV1(snapshot.Epoch)
	if err != nil {
		return out, false, err
	}
	target, err := RabbitVRFTargetEpochForSourceWorkEpochV1(snapshot.Epoch)
	if err != nil {
		return out, false, err
	}
	session, err := NewRabbitVRFDKGSessionContextV1(chainID, target, root, uint64(len(members)))
	if err != nil {
		return out, false, err
	}
	id, err := RabbitVRFDKGSessionIDV1(session)
	if err != nil {
		return out, false, err
	}
	out = RabbitVRFDKGBridgeV1{SourceWorkEpoch: snapshot.Epoch, PreparationEpoch: prep,
		TargetVRFEpoch: target, SelectionRoot: snapshot.Root, DatasetKey: datasetKey,
		Entropy: entropy, CommitteeSeed: seed, CommitteeRoot: root, Members: members, Session: session, SessionID: id}
	return out, true, nil
}
