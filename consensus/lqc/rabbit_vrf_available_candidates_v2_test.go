package lqc

import (
	"reflect"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestRabbitVRFAvailableCandidatesV2(t *testing.T) {
	const head uint64 = 156958
	seats := make([]WorkSeatV1, 36)
	registry := &RegistrySnapshot{Number: head}
	for i := range seats {
		address := common.Address{19: byte(i + 1)}
		seats[i] = WorkSeatV1{Participant: address}
		participant := CanonicalParticipant{
			Address: address, Active: true, LastHeartbeat: 72995,
		}
		if i < 4 {
			participant.LastHeartbeat = head - uint64(i)
		}
		registry.Participants = append(registry.Participants, participant)
	}
	original := append([]WorkSeatV1(nil), seats...)
	candidates, err := rabbitVRFAvailableCandidatesV2(seats, registry, head, 128)
	if err != nil || len(candidates) != 4 {
		t.Fatalf("wanted four recent candidates, got %d: %v", len(candidates), err)
	}
	if !reflect.DeepEqual(seats, original) {
		t.Fatal("mining seats were modified")
	}

	// Registry order must not affect the result.
	for i, j := 0, len(registry.Participants)-1; i < j; i, j = i+1, j-1 {
		registry.Participants[i], registry.Participants[j] =
			registry.Participants[j], registry.Participants[i]
	}
	again, err := rabbitVRFAvailableCandidatesV2(seats, registry, head, 128)
	if err != nil || !reflect.DeepEqual(candidates, again) {
		t.Fatal("candidate result depends on registry iteration order")
	}
}

func TestRabbitVRFAvailableCandidatesV2RejectsUnavailable(t *testing.T) {
	const head uint64 = 1000
	for _, test := range []struct {
		name      string
		exists    bool
		active    bool
		heartbeat uint64
		jailed    uint64
		wanted    int
	}{
		{"missing", false, true, head, 0, 0},
		{"inactive", true, false, head, 0, 0},
		{"zero-heartbeat", true, true, 0, 0, 0},
		{"future-heartbeat", true, true, head + 1, 0, 0},
		{"jailed", true, true, head, head + 1, 0},
		{"window-boundary", true, true, head - 128, 0, 1},
		{"expired", true, true, head - 129, 0, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			address := common.Address{19: 1}
			registry := &RegistrySnapshot{Number: head}
			if test.exists {
				registry.Participants = []CanonicalParticipant{{
					Address: address, Active: test.active,
					LastHeartbeat: test.heartbeat, JailedUntil: test.jailed,
				}}
			}
			got, err := rabbitVRFAvailableCandidatesV2(
				[]WorkSeatV1{{Participant: address}}, registry, head, 128,
			)
			if err != nil || len(got) != test.wanted {
				t.Fatalf("got %d candidates: %v", len(got), err)
			}
		})
	}
}
