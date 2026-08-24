package lqc

// ParticipantSet will be the future deterministic source of LQC participants.
// At this initial stage, it is only the base structure.
// It does not replace RuntimeRegistry yet.
type ParticipantSet struct {
	Participants []HybridParticipant
}

// NewParticipantSet creates an empty set.
func NewParticipantSet() *ParticipantSet {
	return &ParticipantSet{
		Participants: make([]HybridParticipant, 0),
	}
}

// Count returns the number of participants.
func (ps *ParticipantSet) Count() int {
	if ps == nil {
		return 0
	}
	return len(ps.Participants)
}

// Add adds a participant to the set.
func (ps *ParticipantSet) Add(p HybridParticipant) {
	if ps == nil {
		return
	}
	ps.Participants = append(ps.Participants, p)
}

// All returns a copy of the list.
func (ps *ParticipantSet) All() []HybridParticipant {
	if ps == nil {
		return nil
	}
	out := make([]HybridParticipant, len(ps.Participants))
	copy(out, ps.Participants)
	return out
}
