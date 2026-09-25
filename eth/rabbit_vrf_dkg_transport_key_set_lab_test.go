//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"crypto/ecdsa"
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/crypto"
)

func rabbitVRFDKGCanonicalTransportArtifactV1(
	t *testing.T,
	context lqc.RabbitVRFDKGSessionContextV1,
	member lqc.RabbitVRFCommitteeMemberV1,
	participantKey *ecdsa.PrivateKey,
) rabbitVRFDKGTransportArtifactPacketV1 {
	t.Helper()

	transportKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}

	publicKey, err :=
		lqc.RabbitVRFDKGTransportPublicKeyV1FromBytes(
			crypto.CompressPubkey(
				&transportKey.PublicKey,
			),
		)
	if err != nil {
		t.Fatal(err)
	}

	binding, bindingRoot, err :=
		lqc.NewRabbitVRFDKGTransportKeyBindingV1(
			context,
			member,
			publicKey,
		)
	if err != nil {
		t.Fatal(err)
	}

	envelope, err := lqc.NewRabbitVRFDKGEnvelopeV1(
		context,
		member,
		lqc.RabbitVRFDKGMessageTransportKeyBindingV1,
		bindingRoot,
	)
	if err != nil {
		t.Fatal(err)
	}

	signingHash, err :=
		lqc.RabbitVRFDKGEnvelopeSigningHashV1(
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

	return rabbitVRFDKGTransportArtifactPacketV1{
		Binding:  binding,
		Envelope: envelope,
	}
}

func newRabbitVRFDKGCanonicalTransportKeySetFixtureV1(
	t *testing.T,
) (
	*rabbitVRFDKGTransport,
	[]lqc.RabbitVRFCommitteeMemberV1,
	[]rabbitVRFDKGTransportArtifactPacketV1,
	[]*ecdsa.PrivateKey,
) {
	t.Helper()

	context, err := lqc.NewRabbitVRFDKGSessionContextV1(
		big.NewInt(9280),
		21,
		crypto.Keccak256Hash(
			[]byte("rabbit-vrf-canonical-transport-key-set-committee"),
		),
		2,
	)
	if err != nil {
		t.Fatal(err)
	}

	sessionID, err := lqc.RabbitVRFDKGSessionIDV1(context)
	if err != nil {
		t.Fatal(err)
	}

	participantKeys := make([]*ecdsa.PrivateKey, 2)
	members := make([]lqc.RabbitVRFCommitteeMemberV1, 2)
	packets := make(
		[]rabbitVRFDKGTransportArtifactPacketV1,
		2,
	)

	for index := range members {
		participantKey, err := crypto.GenerateKey()
		if err != nil {
			t.Fatal(err)
		}

		shareID := uint64(index + 1)
		member := lqc.RabbitVRFCommitteeMemberV1{
			ShareID: shareID,
			TicketHash: crypto.Keccak256Hash(
				[]byte{
					byte(shareID),
					byte(shareID >> 8),
					0x5a,
				},
			),
			Participant: crypto.PubkeyToAddress(
				participantKey.PublicKey,
			),
		}

		participantKeys[index] = participantKey
		members[index] = member
		packets[index] =
			rabbitVRFDKGCanonicalTransportArtifactV1(
				t,
				context,
				member,
				participantKey,
			)
	}

	runtime := &rabbitVRFDKGRuntime{
		current: rabbitVRFDKGLocalContextV1{
			SessionID:        sessionID,
			CanonicalSession: context,
			CanonicalMembers: append(
				[]lqc.RabbitVRFCommitteeMemberV1(nil),
				members...,
			),
			Members: []lqc.RabbitVRFCommitteeMemberV1{
				members[0],
			},
			TransportBindings: []lqc.RabbitVRFDKGTransportKeyBindingV1{
				packets[0].Binding,
			},
			TransportEnvelopes: []lqc.RabbitVRFDKGEnvelopeV1{
				packets[0].Envelope,
			},
		},
	}

	transport, err := newRabbitVRFDKGTransport(
		rabbitVRFDKGTransportConfig{
			ChainID:   big.NewInt(9280),
			NetworkID: 9280,
			Genesis: crypto.Keccak256Hash(
				[]byte("rabbit-vrf-canonical-transport-key-set-genesis"),
			),
			Runtime: runtime,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	return transport, members, packets, participantKeys
}

func TestRabbitVRFDKGTransportV1CanonicalKeySetMissingMemberNotReady(
	t *testing.T,
) {
	transport, _, _, _ :=
		newRabbitVRFDKGCanonicalTransportKeySetFixtureV1(t)

	root, bindings, envelopes, ready, err :=
		transport.canonicalTransportKeySetV1()
	if err != nil {
		t.Fatal(err)
	}
	if ready {
		t.Fatal("incomplete canonical transport key set reported ready")
	}
	if root != (common.Hash{}) ||
		len(bindings) != 0 ||
		len(envelopes) != 0 {
		t.Fatal("incomplete canonical transport key set leaked partial result")
	}
}

func TestRabbitVRFDKGTransportV1CanonicalKeySetLocalAndRemoteComplete(
	t *testing.T,
) {
	transport, members, packets, _ :=
		newRabbitVRFDKGCanonicalTransportKeySetFixtureV1(t)

	inserted, err :=
		transport.storeRemoteTransportArtifactV1(
			packets[1],
		)
	if err != nil {
		t.Fatal(err)
	}
	if !inserted {
		t.Fatal("remote canonical transport artifact was not inserted")
	}

	root, bindings, envelopes, ready, err :=
		transport.canonicalTransportKeySetV1()
	if err != nil {
		t.Fatal(err)
	}
	if !ready {
		t.Fatal("complete canonical transport key set not ready")
	}

	if len(bindings) != 2 ||
		len(envelopes) != 2 {
		t.Fatalf(
			"wrong canonical transport key set size: bindings=%d envelopes=%d",
			len(bindings),
			len(envelopes),
		)
	}

	if bindings[0].ShareID != 1 ||
		bindings[1].ShareID != 2 {
		t.Fatalf(
			"canonical transport key order is wrong: %d %d",
			bindings[0].ShareID,
			bindings[1].ShareID,
		)
	}

	expectedBindings := []lqc.RabbitVRFDKGTransportKeyBindingV1{
		packets[0].Binding,
		packets[1].Binding,
	}
	expectedEnvelopes := []lqc.RabbitVRFDKGEnvelopeV1{
		packets[0].Envelope,
		packets[1].Envelope,
	}

	expectedRoot, err :=
		lqc.RabbitVRFDKGTransportKeySetRootV1(
			transport.runtime.current.CanonicalSession,
			members,
			expectedBindings,
			expectedEnvelopes,
		)
	if err != nil {
		t.Fatal(err)
	}

	if root == (common.Hash{}) ||
		root != expectedRoot {
		t.Fatal("canonical local+remote transport key set root mismatch")
	}
}

func TestRabbitVRFDKGTransportV1CanonicalKeySetRejectsLocalRemoteConflict(
	t *testing.T,
) {
	transport, members, _, participantKeys :=
		newRabbitVRFDKGCanonicalTransportKeySetFixtureV1(t)

	context := transport.runtime.currentContext()

	conflicting :=
		rabbitVRFDKGCanonicalTransportArtifactV1(
			t,
			context.CanonicalSession,
			members[0],
			participantKeys[0],
		)

	inserted, err :=
		transport.storeRemoteTransportArtifactV1(
			conflicting,
		)
	if err != nil {
		t.Fatal(err)
	}
	if !inserted {
		t.Fatal("conflicting valid remote artifact was not inserted")
	}

	_, _, _, ready, err :=
		transport.canonicalTransportKeySetV1()

	if err == nil {
		t.Fatal("local/remote transport key conflict was accepted")
	}
	if !errors.Is(err, errRabbitVRFDKGArtifactConflict) {
		t.Fatalf("wrong conflict error: %v", err)
	}
	if ready {
		t.Fatal("conflicting canonical transport key set reported ready")
	}
}
