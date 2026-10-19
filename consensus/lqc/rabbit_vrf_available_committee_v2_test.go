package lqc

import (
	"math/big"
	"reflect"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestRabbitVRFAvailableCommitteeV2CanonicalBinding(t *testing.T) {
	chainID := big.NewInt(9280)
	snapshot := rabbitVRFCommitteeTestSnapshotV1(t, chainID, 36)
	original := append([]WorkSeatV1(nil), snapshot.Seats...)
	const head uint64 = 156958
	registry := &RegistrySnapshot{
		Number:       head,
		Hash:         common.HexToHash("0x1234"),
		RegistryRoot: common.HexToHash("0x5678"),
	}
	for i, seat := range snapshot.Seats {
		heartbeat := uint64(72995)
		if i < 4 {
			heartbeat = head - uint64(i)
		}
		registry.Participants = append(registry.Participants, CanonicalParticipant{
			Address: seat.Participant, Active: true, LastHeartbeat: heartbeat,
		})
	}
	entropy := common.HexToHash("0xabcd")
	build := func() (common.Hash, []RabbitVRFCommitteeMemberV1, error) {
		return rabbitVRFAvailableCommitteeV2(
			chainID, snapshot, registry, head, 128, 128, entropy,
		)
	}
	root, members, err := build()
	if err != nil || len(members) != 4 {
		t.Fatalf("wanted four members, got %d: %v", len(members), err)
	}
	threshold, faults, err := RabbitVRFThresholdPolicyV1(uint64(len(members)))
	if err != nil || threshold != 3 || faults != 1 {
		t.Fatalf("unexpected policy: threshold=%d faults=%d err=%v", threshold, faults, err)
	}
	for i, member := range members {
		if member.ShareID != uint64(i+1) {
			t.Fatal("invalid share ordering")
		}
		found := false
		for _, seat := range snapshot.Seats[:4] {
			found = found || seat.Participant == member.Participant
		}
		if !found {
			t.Fatal("unavailable member entered committee")
		}
	}
	again, againMembers, err := build()
	if err != nil || again != root || !reflect.DeepEqual(members, againMembers) {
		t.Fatal("committee is not deterministic")
	}
	registry.Hash = common.HexToHash("0x9999")
	changed, _, err := build()
	if err != nil || changed == root {
		t.Fatal("committee does not bind canonical reference hash")
	}
	if !reflect.DeepEqual(snapshot.Seats, original) {
		t.Fatal("historical mining seats were modified")
	}

	// An empty available set must fail instead of inventing a committee.
	for i := range registry.Participants {
		registry.Participants[i].Active = false
	}
	if _, _, err := build(); err == nil {
		t.Fatal("accepted an empty available set")
	}
}
