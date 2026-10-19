package lqc

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestRabbitVRFAvailableCommitteeV2Sizes(t *testing.T) {
	for _, count := range []int{1, 2, 3, 4, 32, 128, 129, 1000} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			chainID := big.NewInt(9280)
			snapshot := rabbitVRFCommitteeTestSnapshotV1(t, chainID, count)
			registry := &RegistrySnapshot{
				Number:       1000,
				Hash:         common.HexToHash("0x1234"),
				RegistryRoot: common.HexToHash("0x5678"),
			}
			for _, seat := range snapshot.Seats {
				registry.Participants = append(registry.Participants, CanonicalParticipant{
					Address: seat.Participant, Active: true, LastHeartbeat: 999,
				})
			}
			root, members, err := rabbitVRFAvailableCommitteeV2(
				chainID, snapshot, registry, 1000, 128, 128,
				common.HexToHash("0xabcd"),
			)
			if err != nil {
				t.Fatal(err)
			}
			expected := RabbitVRFCommitteeSizeV1(uint64(count), 32, 128)
			if uint64(len(members)) != expected || len(members) > 128 {
				t.Fatalf("unexpected committee size: %d", len(members))
			}
			session, err := NewRabbitVRFDKGSessionContextV1(
				chainID, 11, root, uint64(len(members)),
			)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("candidates=%d committee=%d threshold=%d faults=%d",
				count, len(members), session.Threshold, session.MaxFaults)
		})
	}
}

func TestRabbitVRFCommitteeV2LargePopulationBound(t *testing.T) {
	// Sizing policy only; this is not a million-node network simulation.
	for _, count := range []uint64{10000, 1000000, 10000000} {
		size := RabbitVRFCommitteeSizeV1(count, 32, 128)
		if size == 0 || size > 128 {
			t.Fatalf("population %d produced committee %d", count, size)
		}
	}
}
