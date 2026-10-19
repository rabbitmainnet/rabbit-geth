package lqc

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

func rabbitVRFDKGTransportKeySetFixtureV1(
	t *testing.T,
) (
	RabbitVRFDKGSessionContextV1,
	[]RabbitVRFCommitteeMemberV1,
	[]RabbitVRFDKGTransportKeyBindingV1,
	[]RabbitVRFDKGEnvelopeV1,
) {
	t.Helper()

	context := rabbitVRFDKGTestSessionContextV1(
		t,
		9280,
		11,
	)

	members := make(
		[]RabbitVRFCommitteeMemberV1,
		0,
		context.CommitteeSize,
	)
	bindings := make(
		[]RabbitVRFDKGTransportKeyBindingV1,
		0,
		context.CommitteeSize,
	)
	envelopes := make(
		[]RabbitVRFDKGEnvelopeV1,
		0,
		context.CommitteeSize,
	)

	for shareID := uint64(1); shareID <= context.CommitteeSize; shareID++ {
		participantKey, err := crypto.GenerateKey()
		if err != nil {
			t.Fatal(err)
		}

		transportKey, err := crypto.GenerateKey()
		if err != nil {
			t.Fatal(err)
		}

		member := RabbitVRFCommitteeMemberV1{
			ShareID: shareID,
			TicketHash: crypto.Keccak256Hash(
				[]byte{
					byte(shareID),
					byte(shareID >> 8),
					0xa5,
				},
			),
			Participant: crypto.PubkeyToAddress(
				participantKey.PublicKey,
			),
		}

		publicKey, err :=
			RabbitVRFDKGTransportPublicKeyV1FromBytes(
				crypto.CompressPubkey(
					&transportKey.PublicKey,
				),
			)
		if err != nil {
			t.Fatal(err)
		}

		binding, bindingRoot, err :=
			NewRabbitVRFDKGTransportKeyBindingV1(
				context,
				member,
				publicKey,
			)
		if err != nil {
			t.Fatal(err)
		}

		envelope, err := NewRabbitVRFDKGEnvelopeV1(
			context,
			member,
			RabbitVRFDKGMessageTransportKeyBindingV1,
			bindingRoot,
		)
		if err != nil {
			t.Fatal(err)
		}

		signingHash, err :=
			RabbitVRFDKGEnvelopeSigningHashV1(
				context,
				envelope,
			)
		if err != nil {
			t.Fatal(err)
		}

		envelope.Signature, err = crypto.Sign(
			signingHash[:],
			participantKey,
		)
		if err != nil {
			t.Fatal(err)
		}

		members = append(members, member)
		bindings = append(bindings, binding)
		envelopes = append(envelopes, envelope)
	}

	return context, members, bindings, envelopes
}

func reverseRabbitVRFDKGTransportKeySetFixtureV1[T any](
	input []T,
) []T {
	out := append([]T(nil), input...)
	for left, right := 0, len(out)-1; left < right; left, right = left+1, right-1 {
		out[left], out[right] = out[right], out[left]
	}
	return out
}

func TestRabbitVRFDKGTransportKeySetV1DeterministicAcrossInputOrder(
	t *testing.T,
) {
	context, members, bindings, envelopes :=
		rabbitVRFDKGTransportKeySetFixtureV1(t)

	root, err := RabbitVRFDKGTransportKeySetRootV1(
		context,
		members,
		bindings,
		envelopes,
	)
	if err != nil {
		t.Fatal(err)
	}
	if root == (common.Hash{}) {
		t.Fatal("canonical transport key set root is zero")
	}

	reversedRoot, err := RabbitVRFDKGTransportKeySetRootV1(
		context,
		reverseRabbitVRFDKGTransportKeySetFixtureV1(
			members,
		),
		reverseRabbitVRFDKGTransportKeySetFixtureV1(
			bindings,
		),
		reverseRabbitVRFDKGTransportKeySetFixtureV1(
			envelopes,
		),
	)
	if err != nil {
		t.Fatal(err)
	}

	if reversedRoot != root {
		t.Fatal("transport key set root depends on input order")
	}
}

func TestRabbitVRFDKGTransportKeySetV1RejectsIncompleteSet(
	t *testing.T,
) {
	context, members, bindings, envelopes :=
		rabbitVRFDKGTransportKeySetFixtureV1(t)

	members = members[:len(members)-1]
	bindings = bindings[:len(bindings)-1]
	envelopes = envelopes[:len(envelopes)-1]

	if _, err := RabbitVRFDKGTransportKeySetRootV1(
		context,
		members,
		bindings,
		envelopes,
	); err == nil {
		t.Fatal("incomplete transport key set accepted")
	}
}

func TestRabbitVRFDKGTransportKeySetV1RejectsDuplicateShareID(
	t *testing.T,
) {
	context, members, bindings, envelopes :=
		rabbitVRFDKGTransportKeySetFixtureV1(t)

	members = append(
		[]RabbitVRFCommitteeMemberV1(nil),
		members...,
	)
	members[1].ShareID = members[0].ShareID

	if _, err := RabbitVRFDKGTransportKeySetRootV1(
		context,
		members,
		bindings,
		envelopes,
	); err == nil {
		t.Fatal("duplicate ShareID accepted")
	}
}

func TestRabbitVRFDKGTransportKeySetV1RejectsDuplicateParticipant(
	t *testing.T,
) {
	context, members, bindings, envelopes :=
		rabbitVRFDKGTransportKeySetFixtureV1(t)

	members = append(
		[]RabbitVRFCommitteeMemberV1(nil),
		members...,
	)
	members[1].Participant = members[0].Participant

	if _, err := RabbitVRFDKGTransportKeySetRootV1(
		context,
		members,
		bindings,
		envelopes,
	); err == nil {
		t.Fatal("duplicate participant accepted")
	}
}

func TestRabbitVRFDKGTransportKeySetV1RejectsSwappedBinding(
	t *testing.T,
) {
	context, members, bindings, envelopes :=
		rabbitVRFDKGTransportKeySetFixtureV1(t)

	bindings = append(
		[]RabbitVRFDKGTransportKeyBindingV1(nil),
		bindings...,
	)
	bindings[0], bindings[1] =
		bindings[1], bindings[0]

	if _, err := RabbitVRFDKGTransportKeySetRootV1(
		context,
		members,
		bindings,
		envelopes,
	); err == nil {
		t.Fatal("binding assigned to wrong canonical member accepted")
	}
}
