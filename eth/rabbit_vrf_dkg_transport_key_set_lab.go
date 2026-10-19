//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"errors"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
)

var errRabbitVRFDKGCanonicalTransportKeySetV1 = errors.New(
	"invalid rabbit vrf dkg canonical transport key set v1",
)

func (n *rabbitVRFDKGTransport) persistCanonicalTransportKeySetV1() (
	bool,
	error,
) {
	if n == nil || n.runtime == nil || n.runtime.engine == nil {
		return false, nil
	}

	root, bindings, envelopes, complete, err :=
		n.canonicalTransportKeySetV1()
	if err != nil {
		return false, err
	}
	if !complete {
		return false, nil
	}
	if len(bindings) == 0 {
		return false, errRabbitVRFDKGCanonicalTransportKeySetV1
	}

	sessionID := bindings[0].SessionID

	n.runtime.mu.RLock()
	if n.runtime.current.SessionID != sessionID {
		n.runtime.mu.RUnlock()
		return false, nil
	}

	members := cloneRabbitVRFDKGMembersV1(
		n.runtime.current.CanonicalMembers,
	)
	if len(members) != len(bindings) {
		n.runtime.mu.RUnlock()
		return false, errRabbitVRFDKGCanonicalTransportKeySetV1
	}

	persisted, _, err :=
		n.runtime.engine.EnsureRabbitVRFDKGTransportKeySetStateV1(
			sessionID,
			members,
			bindings,
			envelopes,
		)
	n.runtime.mu.RUnlock()

	if err != nil {
		return false, err
	}
	if persisted == nil ||
		persisted.SessionID != sessionID ||
		persisted.Root != root {
		return false, errRabbitVRFDKGCanonicalTransportKeySetV1
	}

	if !n.runtime.setCanonicalTransportKeySetV1(
		persisted.SessionID,
		persisted.Root,
		persisted.Members,
		persisted.Bindings,
		persisted.Envelopes,
	) {
		return false, nil
	}

	return true, nil
}

func (n *rabbitVRFDKGTransport) canonicalTransportKeySetV1() (
	common.Hash,
	[]lqc.RabbitVRFDKGTransportKeyBindingV1,
	[]lqc.RabbitVRFDKGEnvelopeV1,
	bool,
	error,
) {
	if n == nil || n.runtime == nil {
		return common.Hash{},
			nil,
			nil,
			false,
			errRabbitVRFDKGCanonicalTransportKeySetV1
	}

	context := n.runtime.currentContext()

	if context.SessionID == (common.Hash{}) ||
		len(context.CanonicalMembers) == 0 {
		return common.Hash{}, nil, nil, false, nil
	}

	if uint64(len(context.CanonicalMembers)) !=
		context.CanonicalSession.CommitteeSize {
		return common.Hash{},
			nil,
			nil,
			false,
			errRabbitVRFDKGCanonicalTransportKeySetV1
	}

	expectedByShare := make(
		map[uint64]lqc.RabbitVRFCommitteeMemberV1,
		len(context.CanonicalMembers),
	)
	seenParticipants := make(
		map[common.Address]struct{},
		len(context.CanonicalMembers),
	)

	for _, member := range context.CanonicalMembers {
		if member.ShareID == 0 ||
			member.ShareID > context.CanonicalSession.CommitteeSize ||
			member.TicketHash == (common.Hash{}) ||
			member.Participant == (common.Address{}) {
			return common.Hash{},
				nil,
				nil,
				false,
				errRabbitVRFDKGCanonicalTransportKeySetV1
		}

		if _, exists := expectedByShare[member.ShareID]; exists {
			return common.Hash{},
				nil,
				nil,
				false,
				errRabbitVRFDKGCanonicalTransportKeySetV1
		}

		if _, exists := seenParticipants[member.Participant]; exists {
			return common.Hash{},
				nil,
				nil,
				false,
				errRabbitVRFDKGCanonicalTransportKeySetV1
		}

		expectedByShare[member.ShareID] = member
		seenParticipants[member.Participant] = struct{}{}
	}

	packets := make(
		map[uint64]rabbitVRFDKGTransportArtifactPacketV1,
		len(context.CanonicalMembers),
	)

	addPacket := func(
		expected lqc.RabbitVRFCommitteeMemberV1,
		packet rabbitVRFDKGTransportArtifactPacketV1,
	) error {
		if packet.Binding.ShareID != expected.ShareID ||
			packet.Binding.Participant != expected.Participant ||
			packet.Binding.SessionID != context.SessionID ||
			packet.Envelope.SessionID != context.SessionID {
			return errRabbitVRFDKGCanonicalTransportKeySetV1
		}

		if err := lqc.VerifyRabbitVRFDKGTransportKeyEnvelopeV1(
			context.CanonicalSession,
			expected,
			packet.Binding,
			packet.Envelope,
		); err != nil {
			return err
		}

		if existing, ok := packets[expected.ShareID]; ok {
			if existing.Binding != packet.Binding {
				return errRabbitVRFDKGArtifactConflict
			}
			return nil
		}

		packets[expected.ShareID] =
			cloneRabbitVRFDKGTransportArtifactPacketV1(packet)

		return nil
	}

	if len(context.TransportBindings) !=
		len(context.TransportEnvelopes) {
		return common.Hash{},
			nil,
			nil,
			false,
			errRabbitVRFDKGCanonicalTransportKeySetV1
	}

	if len(context.TransportBindings) != 0 {
		if len(context.TransportBindings) != len(context.Members) {
			return common.Hash{},
				nil,
				nil,
				false,
				errRabbitVRFDKGCanonicalTransportKeySetV1
		}

		for index, member := range context.Members {
			expected, ok := expectedByShare[member.ShareID]
			if !ok ||
				expected.Participant != member.Participant ||
				expected.TicketHash != member.TicketHash {
				return common.Hash{},
					nil,
					nil,
					false,
					errRabbitVRFDKGCanonicalTransportKeySetV1
			}

			if err := addPacket(
				expected,
				rabbitVRFDKGTransportArtifactPacketV1{
					Binding:  context.TransportBindings[index],
					Envelope: context.TransportEnvelopes[index],
				},
			); err != nil {
				return common.Hash{}, nil, nil, false, err
			}
		}
	}

	n.mu.RLock()
	if n.closed {
		n.mu.RUnlock()
		return common.Hash{},
			nil,
			nil,
			false,
			errRabbitVRFDKGProtocolClosed
	}

	remote := make(
		[]rabbitVRFDKGTransportArtifactPacketV1,
		0,
		len(n.remoteArtifacts),
	)

	if n.remoteSession == context.SessionID {
		for _, packet := range n.remoteArtifacts {
			remote = append(
				remote,
				cloneRabbitVRFDKGTransportArtifactPacketV1(packet),
			)
		}
	}
	n.mu.RUnlock()

	for _, packet := range remote {
		expected, ok := expectedByShare[packet.Binding.ShareID]
		if !ok ||
			expected.Participant != packet.Binding.Participant {
			return common.Hash{},
				nil,
				nil,
				false,
				errRabbitVRFDKGCanonicalTransportKeySetV1
		}

		if err := addPacket(expected, packet); err != nil {
			return common.Hash{}, nil, nil, false, err
		}
	}

	bindings := make(
		[]lqc.RabbitVRFDKGTransportKeyBindingV1,
		0,
		len(context.CanonicalMembers),
	)
	envelopes := make(
		[]lqc.RabbitVRFDKGEnvelopeV1,
		0,
		len(context.CanonicalMembers),
	)

	for _, member := range context.CanonicalMembers {
		packet, ok := packets[member.ShareID]
		if !ok {
			current := n.runtime.currentContext()
			if current.SessionID != context.SessionID {
				return common.Hash{}, nil, nil, false, nil
			}

			return common.Hash{}, nil, nil, false, nil
		}

		bindings = append(bindings, packet.Binding)

		envelope := packet.Envelope
		envelope.Signature = append(
			[]byte(nil),
			envelope.Signature...,
		)
		envelopes = append(envelopes, envelope)
	}

	current := n.runtime.currentContext()
	if current.SessionID != context.SessionID {
		return common.Hash{}, nil, nil, false, nil
	}

	root, err := lqc.RabbitVRFDKGTransportKeySetRootV1(
		context.CanonicalSession,
		context.CanonicalMembers,
		bindings,
		envelopes,
	)
	if err != nil {
		return common.Hash{}, nil, nil, false, err
	}

	current = n.runtime.currentContext()
	if current.SessionID != context.SessionID {
		return common.Hash{}, nil, nil, false, nil
	}

	return root, bindings, envelopes, true, nil
}
