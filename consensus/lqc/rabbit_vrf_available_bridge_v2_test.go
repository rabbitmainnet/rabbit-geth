package lqc

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestRabbitVRFAvailableBridgeV2FrozenRegistry(t *testing.T) {
	chainID := big.NewInt(9280)
	snapshot := rabbitVRFCommitteeTestSnapshotV1(t, chainID, 36)
	start, err := RabbitVRFDKGPreparationStartBlockV1(snapshot.Epoch, 128)
	if err != nil {
		t.Fatal(err)
	}
	registry := NewCanonicalRegistry()
	for i, seat := range snapshot.Seats {
		heartbeat := uint64(1)
		if i < 4 {
			heartbeat = start - 2
		}
		registry.entries[seat.Participant] = CanonicalParticipant{Address: seat.Participant, Active: true, LastHeartbeat: heartbeat}
	}
	view := &RegistrySnapshot{Number: start - 1, Hash: common.HexToHash("0x1234"), RegistryRoot: registry.Root(), Participants: registry.Participants()}
	dataset := common.HexToHash("0xabcd")
	build := func() (RabbitVRFDKGBridgeV1, bool, error) {
		return rabbitVRFAvailableBridgeV2(chainID, snapshot, dataset, view, 128, 80, WorkSelectionBeaconHasherV1(fakeSelectionBeaconHasherV1))
	}
	first, exists, err := build()
	if err != nil || !exists || len(first.Members) != 4 || first.Session.Threshold != 3 {
		t.Fatalf("bad bridge: %v %v %d", exists, err, len(first.Members))
	}
	if err := validateRabbitVRFDKGBridgeLifecycleInputV1(first); err != nil {
		t.Fatalf("incompatible lifecycle: %v", err)
	}
	second, exists, err := build()
	if err != nil || !exists || first.SessionID != second.SessionID {
		t.Fatal("session changed on replay")
	}
	if first.SelectionRoot != snapshot.Root || first.CommitteeSeed == (common.Hash{}) {
		t.Fatal("lost source commitment")
	}
	view.Number++
	if _, _, err := build(); err == nil {
		t.Fatal("accepted moving registry reference")
	}
	view.Number--
	view.RegistryRoot = common.HexToHash("0xffff")
	if _, _, err := build(); err == nil {
		t.Fatal("accepted forged registry root")
	}
}
