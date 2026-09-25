package lqc

import (
	"bytes"
	"errors"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rlp"
)

const RabbitVRFDKGTransportKeySetStateVersionV1 uint8 = 1

var (
	ErrInvalidRabbitVRFDKGTransportKeySetStateV1 = errors.New(
		"invalid rabbit vrf dkg transport key set state v1",
	)
	ErrRabbitVRFDKGTransportKeySetStateConflictV1 = errors.New(
		"rabbit vrf dkg transport key set state conflict v1",
	)
)

var rabbitVRFDKGTransportKeySetStatePrefixV1 = []byte(
	"rabbit-lqc-vrf-dkg-transport-key-set-v1:",
)

var rabbitVRFDKGTransportKeySetStateMu sync.Mutex

type RabbitVRFDKGTransportKeySetStateV1 struct {
	Version   uint8
	SessionID common.Hash
	Root      common.Hash
	Members   []RabbitVRFCommitteeMemberV1
	Bindings  []RabbitVRFDKGTransportKeyBindingV1
	Envelopes []RabbitVRFDKGEnvelopeV1
}

func rabbitVRFDKGTransportKeySetStateKeyV1(sessionID common.Hash) []byte {
	key := make(
		[]byte,
		0,
		len(rabbitVRFDKGTransportKeySetStatePrefixV1)+len(sessionID),
	)
	key = append(key, rabbitVRFDKGTransportKeySetStatePrefixV1...)
	return append(key, sessionID[:]...)
}

func cloneRabbitVRFDKGTransportKeySetStateV1(
	state *RabbitVRFDKGTransportKeySetStateV1,
) *RabbitVRFDKGTransportKeySetStateV1 {
	if state == nil {
		return nil
	}

	out := *state
	out.Members = append(
		[]RabbitVRFCommitteeMemberV1(nil),
		state.Members...,
	)
	out.Bindings = append(
		[]RabbitVRFDKGTransportKeyBindingV1(nil),
		state.Bindings...,
	)
	out.Envelopes = make(
		[]RabbitVRFDKGEnvelopeV1,
		len(state.Envelopes),
	)
	for index := range state.Envelopes {
		out.Envelopes[index] = state.Envelopes[index]
		out.Envelopes[index].Signature = append(
			[]byte(nil),
			state.Envelopes[index].Signature...,
		)
	}
	return &out
}

func newRabbitVRFDKGTransportKeySetStateV1(
	lifecycle *RabbitVRFDKGLifecycleV1,
	members []RabbitVRFCommitteeMemberV1,
	bindings []RabbitVRFDKGTransportKeyBindingV1,
	envelopes []RabbitVRFDKGEnvelopeV1,
) (*RabbitVRFDKGTransportKeySetStateV1, error) {
	if ValidateRabbitVRFDKGLifecycleV1(lifecycle) != nil {
		return nil, ErrInvalidRabbitVRFDKGTransportKeySetStateV1
	}

	root, err := RabbitVRFDKGTransportKeySetRootV1(
		lifecycle.Session,
		members,
		bindings,
		envelopes,
	)
	if err != nil {
		return nil, ErrInvalidRabbitVRFDKGTransportKeySetStateV1
	}

	state := &RabbitVRFDKGTransportKeySetStateV1{
		Version:   RabbitVRFDKGTransportKeySetStateVersionV1,
		SessionID: lifecycle.SessionID,
		Root:      root,
		Members: append(
			[]RabbitVRFCommitteeMemberV1(nil),
			members...,
		),
		Bindings: append(
			[]RabbitVRFDKGTransportKeyBindingV1(nil),
			bindings...,
		),
		Envelopes: make(
			[]RabbitVRFDKGEnvelopeV1,
			len(envelopes),
		),
	}
	for index := range envelopes {
		state.Envelopes[index] = envelopes[index]
		state.Envelopes[index].Signature = append(
			[]byte(nil),
			envelopes[index].Signature...,
		)
	}

	if err := ValidateRabbitVRFDKGTransportKeySetStateV1(
		lifecycle,
		state,
	); err != nil {
		return nil, err
	}

	return state, nil
}

func ValidateRabbitVRFDKGTransportKeySetStateV1(
	lifecycle *RabbitVRFDKGLifecycleV1,
	state *RabbitVRFDKGTransportKeySetStateV1,
) error {
	if ValidateRabbitVRFDKGLifecycleV1(lifecycle) != nil ||
		state == nil ||
		state.Version != RabbitVRFDKGTransportKeySetStateVersionV1 ||
		state.SessionID == (common.Hash{}) ||
		state.SessionID != lifecycle.SessionID ||
		state.Root == (common.Hash{}) ||
		uint64(len(state.Members)) != lifecycle.Session.CommitteeSize ||
		len(state.Bindings) != len(state.Members) ||
		len(state.Envelopes) != len(state.Members) {
		return ErrInvalidRabbitVRFDKGTransportKeySetStateV1
	}

	committee := make([]WorkSeatV1, len(state.Members))
	for index, member := range state.Members {
		if member.ShareID != uint64(index+1) ||
			member.TicketHash == (common.Hash{}) ||
			member.Participant == (common.Address{}) {
			return ErrInvalidRabbitVRFDKGTransportKeySetStateV1
		}
		committee[index] = WorkSeatV1{
			TicketHash:  member.TicketHash,
			Participant: member.Participant,
		}
	}

	committeeRoot, canonicalMembers, err := RabbitVRFCommitteeRootV1(
		lifecycle.Session.ChainID,
		lifecycle.SourceWorkEpoch,
		lifecycle.SelectionRoot,
		lifecycle.CommitteeSeed,
		committee,
	)
	if err != nil ||
		committeeRoot != lifecycle.CommitteeRoot ||
		committeeRoot != lifecycle.Session.CommitteeRoot ||
		len(canonicalMembers) != len(state.Members) {
		return ErrInvalidRabbitVRFDKGTransportKeySetStateV1
	}

	for index := range canonicalMembers {
		if canonicalMembers[index] != state.Members[index] {
			return ErrInvalidRabbitVRFDKGTransportKeySetStateV1
		}
	}

	root, err := RabbitVRFDKGTransportKeySetRootV1(
		lifecycle.Session,
		state.Members,
		state.Bindings,
		state.Envelopes,
	)
	if err != nil || root != state.Root {
		return ErrInvalidRabbitVRFDKGTransportKeySetStateV1
	}

	return nil
}

func encodeRabbitVRFDKGTransportKeySetStateV1(
	lifecycle *RabbitVRFDKGLifecycleV1,
	state *RabbitVRFDKGTransportKeySetStateV1,
) ([]byte, error) {
	if err := ValidateRabbitVRFDKGTransportKeySetStateV1(
		lifecycle,
		state,
	); err != nil {
		return nil, err
	}
	return rlp.EncodeToBytes(state)
}

func decodeRabbitVRFDKGTransportKeySetStateV1(
	lifecycle *RabbitVRFDKGLifecycleV1,
	blob []byte,
) (*RabbitVRFDKGTransportKeySetStateV1, error) {
	var state RabbitVRFDKGTransportKeySetStateV1

	if err := rlp.DecodeBytes(blob, &state); err != nil {
		return nil, ErrInvalidRabbitVRFDKGTransportKeySetStateV1
	}
	if err := ValidateRabbitVRFDKGTransportKeySetStateV1(
		lifecycle,
		&state,
	); err != nil {
		return nil, err
	}

	canonical, err := rlp.EncodeToBytes(&state)
	if err != nil || !bytes.Equal(canonical, blob) {
		return nil, ErrInvalidRabbitVRFDKGTransportKeySetStateV1
	}

	return cloneRabbitVRFDKGTransportKeySetStateV1(&state), nil
}

func (l *LQC) LoadRabbitVRFDKGTransportKeySetStateV1(
	sessionID common.Hash,
) (*RabbitVRFDKGTransportKeySetStateV1, error) {
	if l == nil ||
		l.db == nil ||
		sessionID == (common.Hash{}) {
		return nil, ErrInvalidRabbitVRFDKGTransportKeySetStateV1
	}

	lifecycle, err := l.LoadRabbitVRFDKGLifecycleV1(sessionID)
	if err != nil {
		return nil, err
	}

	blob, err := l.db.Get(
		rabbitVRFDKGTransportKeySetStateKeyV1(sessionID),
	)
	if err != nil {
		return nil, err
	}

	state, err := decodeRabbitVRFDKGTransportKeySetStateV1(
		lifecycle,
		blob,
	)
	if err != nil {
		return nil, err
	}

	if state.SessionID != sessionID {
		return nil, ErrInvalidRabbitVRFDKGTransportKeySetStateV1
	}

	return state, nil
}

func (l *LQC) EnsureRabbitVRFDKGTransportKeySetStateV1(
	sessionID common.Hash,
	members []RabbitVRFCommitteeMemberV1,
	bindings []RabbitVRFDKGTransportKeyBindingV1,
	envelopes []RabbitVRFDKGEnvelopeV1,
) (
	*RabbitVRFDKGTransportKeySetStateV1,
	bool,
	error,
) {
	if l == nil ||
		l.db == nil ||
		sessionID == (common.Hash{}) {
		return nil,
			false,
			ErrInvalidRabbitVRFDKGTransportKeySetStateV1
	}

	lifecycle, err := l.LoadRabbitVRFDKGLifecycleV1(sessionID)
	if err != nil {
		return nil, false, err
	}

	expected, err := newRabbitVRFDKGTransportKeySetStateV1(
		lifecycle,
		members,
		bindings,
		envelopes,
	)
	if err != nil {
		return nil, false, err
	}

	rabbitVRFDKGTransportKeySetStateMu.Lock()
	defer rabbitVRFDKGTransportKeySetStateMu.Unlock()

	key := rabbitVRFDKGTransportKeySetStateKeyV1(sessionID)

	has, err := l.db.Has(key)
	if err != nil {
		return nil, false, err
	}

	if has {
		existing, err :=
			l.LoadRabbitVRFDKGTransportKeySetStateV1(sessionID)
		if err != nil {
			return nil, false, err
		}

		existingBlob, err :=
			encodeRabbitVRFDKGTransportKeySetStateV1(
				lifecycle,
				existing,
			)
		if err != nil {
			return nil, false, err
		}

		expectedBlob, err :=
			encodeRabbitVRFDKGTransportKeySetStateV1(
				lifecycle,
				expected,
			)
		if err != nil {
			return nil, false, err
		}

		if !bytes.Equal(existingBlob, expectedBlob) {
			return nil,
				false,
				ErrRabbitVRFDKGTransportKeySetStateConflictV1
		}

		return existing, false, nil
	}

	blob, err := encodeRabbitVRFDKGTransportKeySetStateV1(
		lifecycle,
		expected,
	)
	if err != nil {
		return nil, false, err
	}

	if err := l.db.Put(key, blob); err != nil {
		return nil, false, err
	}

	return cloneRabbitVRFDKGTransportKeySetStateV1(expected),
		true,
		nil
}
