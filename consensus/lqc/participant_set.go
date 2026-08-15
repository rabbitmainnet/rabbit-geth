package lqc

// ParticipantSet será a futura fonte determinística dos participantes do LQC.
// Nesta primeira etapa ele é apenas a estrutura base.
// Ainda não substitui o RuntimeRegistry.
type ParticipantSet struct {
	Participants []HybridParticipant
}

// NewParticipantSet cria um conjunto vazio.
func NewParticipantSet() *ParticipantSet {
	return &ParticipantSet{
		Participants: make([]HybridParticipant, 0),
	}
}

// Count retorna a quantidade de participantes.
func (ps *ParticipantSet) Count() int {
	if ps == nil {
		return 0
	}
	return len(ps.Participants)
}

// Add adiciona um participante ao conjunto.
func (ps *ParticipantSet) Add(p HybridParticipant) {
	if ps == nil {
		return
	}
	ps.Participants = append(ps.Participants, p)
}

// All retorna uma cópia da lista.
func (ps *ParticipantSet) All() []HybridParticipant {
	if ps == nil {
		return nil
	}
	out := make([]HybridParticipant, len(ps.Participants))
	copy(out, ps.Participants)
	return out
}
