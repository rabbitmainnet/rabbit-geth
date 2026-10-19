//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/crypto"
	rabbitvrf "github.com/ethereum/go-ethereum/crypto/rabbitvrf"
)

func TestRabbitVRFDKGTransportV1AcceptsPolynomialCommitmentSmoke(t *testing.T) {
	participantKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}

	context, err := lqc.NewRabbitVRFDKGSessionContextV1(
		big.NewInt(9280),
		11,
		crypto.Keccak256Hash([]byte("rabbit-vrf-polynomial-smoke")),
		32,
	)
	if err != nil {
		t.Fatal(err)
	}

	member := lqc.RabbitVRFCommitteeMemberV1{
		ShareID: 7,
		TicketHash: crypto.Keccak256Hash(
			[]byte("rabbit-vrf-polynomial-smoke-ticket"),
		),
		Participant: crypto.PubkeyToAddress(participantKey.PublicKey),
	}

	coefficients := make(
		[]rabbitvrf.DKGCoefficientCommitmentV1,
		context.Threshold,
	)
	for i := range coefficients {
		secret, err := rabbitvrf.GenerateSecretKey()
		if err != nil {
			t.Fatal(err)
		}
		publicKey, err := secret.PublicKey()
		if err != nil {
			t.Fatal(err)
		}
		copy(coefficients[i][:], publicKey[:])
	}

	commitment, root, err := lqc.NewRabbitVRFDKGPolynomialCommitmentV1(
		context,
		member.ShareID,
		coefficients,
	)
	if err != nil {
		t.Fatal(err)
	}

	envelope, err := lqc.NewRabbitVRFDKGEnvelopeV1(
		context,
		member,
		lqc.RabbitVRFDKGMessagePolynomialCommitmentV1,
		root,
	)
	if err != nil {
		t.Fatal(err)
	}

	signingHash, err := lqc.RabbitVRFDKGEnvelopeSigningHashV1(
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

	sessionID, err := lqc.RabbitVRFDKGSessionIDV1(context)
	if err != nil {
		t.Fatal(err)
	}

	runtime := &rabbitVRFDKGRuntime{
		current: rabbitVRFDKGLocalContextV1{
			SessionID:        sessionID,
			CanonicalSession: context,
			CanonicalMembers: []lqc.RabbitVRFCommitteeMemberV1{
				member,
			},
		},
	}

	transport, err := newRabbitVRFDKGTransport(
		rabbitVRFDKGTransportConfig{
			ChainID:   big.NewInt(9280),
			NetworkID: 9280,
			Genesis: crypto.Keccak256Hash(
				[]byte("rabbit-vrf-polynomial-smoke-genesis"),
			),
			Runtime: runtime,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	packet := rabbitVRFDKGPolynomialCommitmentPacketV1{
		Commitment: commitment,
		Envelope:   envelope,
	}
	if err := transport.validatePolynomialCommitmentV1(packet); err != nil {
		t.Fatalf("valid polynomial commitment rejected: %v", err)
	}
}
