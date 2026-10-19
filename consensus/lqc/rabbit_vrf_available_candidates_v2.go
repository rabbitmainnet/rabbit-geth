package lqc

import (
	"errors"

	"github.com/ethereum/go-ethereum/common"
)

// Candidate filtering only. A recent canonical heartbeat does not guarantee
// that a participant will complete DKG. The caller must freeze the canonical
// reference and bind this candidate set into the versioned session commitment.
func rabbitVRFAvailableCandidatesV2(
	seats []WorkSeatV1,
	registry *RegistrySnapshot,
	referenceBlock uint64,
	heartbeatWindow uint64,
) ([]WorkSeatV1, error) {
	if registry == nil || registry.Number != referenceBlock || heartbeatWindow == 0 {
		return nil, errors.New("invalid VRF availability reference")
	}

	participants := make(map[common.Address]CanonicalParticipant, len(registry.Participants))
	for _, participant := range registry.Participants {
		if participant.Address == (common.Address{}) {
			return nil, errors.New("zero registry participant")
		}
		if _, exists := participants[participant.Address]; exists {
			return nil, errors.New("duplicate registry participant")
		}
		participants[participant.Address] = participant
	}

	out := make([]WorkSeatV1, 0, len(seats))
	seen := make(map[common.Address]bool, len(seats))
	for _, seat := range seats {
		if seat.Participant == (common.Address{}) || seen[seat.Participant] {
			return nil, errors.New("invalid or duplicate VRF candidate")
		}
		seen[seat.Participant] = true
		participant, exists := participants[seat.Participant]
		if !exists || !participant.Active || participant.JailedUntil > referenceBlock {
			continue
		}
		heartbeat := participant.LastHeartbeat
		if heartbeat == 0 || heartbeat > referenceBlock {
			continue
		}
		if referenceBlock-heartbeat > heartbeatWindow {
			continue
		}
		out = append(out, seat)
	}
	return out, nil
}
