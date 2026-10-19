package lqc

import (
	"bytes"
	"crypto/ecdsa"
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

type rabbitVRFParticipationProofFixtureV1 struct {
	context              RabbitVRFDKGSessionContextV1
	transportKeySetRoot  common.Hash
	keysetRoot           common.Hash
	requestID            common.Hash
	messageHash          common.Hash
	partialMessageIDs    []common.Hash
	members              []RabbitVRFCommitteeMemberV1
	bindings             []RabbitVRFDKGTransportKeyBindingV1
	transportPrivateKeys []*ecdsa.PrivateKey
	proof                RabbitVRFParticipationProofV1
}

func newRabbitVRFParticipationProofFixtureV1(t *testing.T) rabbitVRFParticipationProofFixtureV1 {
	t.Helper()

	context, err := NewRabbitVRFDKGSessionContextV1(
		big.NewInt(9280),
		11,
		crypto.Keccak256Hash([]byte("rabbit-vrf-participation-committee")),
		4,
	)
	if err != nil {
		t.Fatal(err)
	}
	if context.Threshold != 3 {
		t.Fatalf("threshold=%d want=3", context.Threshold)
	}

	fixture := rabbitVRFParticipationProofFixtureV1{
		context:              context,
		transportKeySetRoot:  crypto.Keccak256Hash([]byte("rabbit-vrf-participation-transport-set")),
		keysetRoot:           crypto.Keccak256Hash([]byte("rabbit-vrf-participation-keyset")),
		requestID:            crypto.Keccak256Hash([]byte("rabbit-vrf-participation-request")),
		messageHash:          crypto.Keccak256Hash([]byte("rabbit-vrf-participation-message")),
		partialMessageIDs:    make([]common.Hash, 4),
		members:              make([]RabbitVRFCommitteeMemberV1, 4),
		bindings:             make([]RabbitVRFDKGTransportKeyBindingV1, 4),
		transportPrivateKeys: make([]*ecdsa.PrivateKey, 4),
		proof: RabbitVRFParticipationProofV1{
			Version: RabbitVRFParticipationProofVersionV1,
			Bitmap:  []byte{0x07},
		},
	}

	for index := range fixture.members {
		participantKey, err := crypto.GenerateKey()
		if err != nil {
			t.Fatal(err)
		}
		transportKey, err := crypto.GenerateKey()
		if err != nil {
			t.Fatal(err)
		}
		member := RabbitVRFCommitteeMemberV1{
			ShareID:     uint64(index + 1),
			TicketHash:  crypto.Keccak256Hash([]byte{byte(index + 1), 0x42}),
			Participant: crypto.PubkeyToAddress(participantKey.PublicKey),
		}
		publicKey, err := RabbitVRFDKGTransportPublicKeyV1FromBytes(crypto.CompressPubkey(&transportKey.PublicKey))
		if err != nil {
			t.Fatal(err)
		}
		binding, _, err := NewRabbitVRFDKGTransportKeyBindingV1(context, member, publicKey)
		if err != nil {
			t.Fatal(err)
		}
		fixture.members[index] = member
		fixture.partialMessageIDs[index] = crypto.Keccak256Hash([]byte{byte(index + 1), 0x99})
		fixture.bindings[index] = binding
		fixture.transportPrivateKeys[index] = transportKey
	}

	for index := 0; index < int(context.Threshold); index++ {
		hash, err := RabbitVRFParticipationSigningHashV1(
			context,
			fixture.transportKeySetRoot,
			fixture.keysetRoot,
			fixture.requestID,
			fixture.messageHash,
			fixture.partialMessageIDs[index],
			fixture.members[index],
		)
		if err != nil {
			t.Fatal(err)
		}
		signature, err := crypto.Sign(hash[:], fixture.transportPrivateKeys[index])
		if err != nil {
			t.Fatal(err)
		}
		fixture.proof.PartialMessageIDs = append(fixture.proof.PartialMessageIDs, fixture.partialMessageIDs[index])
		fixture.proof.Signatures = append(
			fixture.proof.Signatures,
			append([]byte(nil), signature[:RabbitVRFParticipationSignatureSizeV1]...),
		)
	}
	return fixture
}

func cloneRabbitVRFParticipationProofV1(proof RabbitVRFParticipationProofV1) RabbitVRFParticipationProofV1 {
	out := proof
	out.Bitmap = append([]byte(nil), proof.Bitmap...)
	out.PartialMessageIDs = append([]common.Hash(nil), proof.PartialMessageIDs...)
	out.Signatures = make([][]byte, len(proof.Signatures))
	for index := range proof.Signatures {
		out.Signatures[index] = append([]byte(nil), proof.Signatures[index]...)
	}
	return out
}

func TestRabbitVRFParticipationProofV1CanonicalAndAdversarial(t *testing.T) {
	fixture := newRabbitVRFParticipationProofFixtureV1(t)

	participants, err := ValidateRabbitVRFParticipationProofV1(
		fixture.context,
		fixture.transportKeySetRoot,
		fixture.members,
		fixture.bindings,
		fixture.keysetRoot,
		fixture.requestID,
		fixture.messageHash,
		fixture.proof,
	)
	if err != nil {
		t.Fatalf("valid proof rejected: %v", err)
	}
	if len(participants) != 3 || participants[0].ShareID != 1 || participants[1].ShareID != 2 || participants[2].ShareID != 3 {
		t.Fatalf("participants=%v want ShareIDs 1,2,3", participants)
	}

	t.Run("deterministic signing hash", func(t *testing.T) {
		first, err := RabbitVRFParticipationSigningHashV1(
			fixture.context, fixture.transportKeySetRoot, fixture.keysetRoot,
			fixture.requestID, fixture.messageHash, fixture.partialMessageIDs[0], fixture.members[0],
		)
		if err != nil {
			t.Fatal(err)
		}
		second, err := RabbitVRFParticipationSigningHashV1(
			fixture.context, fixture.transportKeySetRoot, fixture.keysetRoot,
			fixture.requestID, fixture.messageHash, fixture.partialMessageIDs[0], fixture.members[0],
		)
		if err != nil {
			t.Fatal(err)
		}
		if first != second || first == (common.Hash{}) {
			t.Fatal("participation signing hash is not deterministic")
		}
	})

	cases := map[string]func(*rabbitVRFParticipationProofFixtureV1){
		"wrong_transport_root": func(f *rabbitVRFParticipationProofFixtureV1) { f.transportKeySetRoot[0] ^= 1 },
		"wrong_keyset":         func(f *rabbitVRFParticipationProofFixtureV1) { f.keysetRoot[0] ^= 1 },
		"wrong_request":        func(f *rabbitVRFParticipationProofFixtureV1) { f.requestID[0] ^= 1 },
		"wrong_message":        func(f *rabbitVRFParticipationProofFixtureV1) { f.messageHash[0] ^= 1 },
		"wrong_partial_id":     func(f *rabbitVRFParticipationProofFixtureV1) { f.proof.PartialMessageIDs[0][0] ^= 1 },
		"bad_signature":        func(f *rabbitVRFParticipationProofFixtureV1) { f.proof.Signatures[0][0] ^= 1 },
		"swapped_signatures": func(f *rabbitVRFParticipationProofFixtureV1) {
			f.proof.Signatures[0], f.proof.Signatures[1] = f.proof.Signatures[1], f.proof.Signatures[0]
		},
		"too_few": func(f *rabbitVRFParticipationProofFixtureV1) {
			f.proof.Bitmap = []byte{0x03}
			f.proof.PartialMessageIDs = f.proof.PartialMessageIDs[:2]
			f.proof.Signatures = f.proof.Signatures[:2]
		},
		"too_many": func(f *rabbitVRFParticipationProofFixtureV1) {
			f.proof.Bitmap = []byte{0x0f}
			hash, err := RabbitVRFParticipationSigningHashV1(
				f.context, f.transportKeySetRoot, f.keysetRoot, f.requestID, f.messageHash, f.partialMessageIDs[3], f.members[3],
			)
			if err != nil {
				t.Fatal(err)
			}
			signature, err := crypto.Sign(hash[:], f.transportPrivateKeys[3])
			if err != nil {
				t.Fatal(err)
			}
			f.proof.PartialMessageIDs = append(f.proof.PartialMessageIDs, f.partialMessageIDs[3])
			f.proof.Signatures = append(f.proof.Signatures, append([]byte(nil), signature[:RabbitVRFParticipationSignatureSizeV1]...))
		},
		"signature_count_mismatch": func(f *rabbitVRFParticipationProofFixtureV1) {
			f.proof.Signatures = f.proof.Signatures[:2]
		},
		"partial_id_count_mismatch": func(f *rabbitVRFParticipationProofFixtureV1) {
			f.proof.PartialMessageIDs = f.proof.PartialMessageIDs[:2]
		},
		"noncanonical_bitmap_length": func(f *rabbitVRFParticipationProofFixtureV1) {
			f.proof.Bitmap = append(f.proof.Bitmap, 0)
		},
		"out_of_range_bitmap_bit": func(f *rabbitVRFParticipationProofFixtureV1) {
			f.proof.Bitmap[0] |= 0x80
		},
		"wrong_signature_size": func(f *rabbitVRFParticipationProofFixtureV1) {
			f.proof.Signatures[0] = append(f.proof.Signatures[0], 0)
		},
		"swapped_binding": func(f *rabbitVRFParticipationProofFixtureV1) {
			f.bindings[0], f.bindings[1] = f.bindings[1], f.bindings[0]
		},
		"mutated_participant": func(f *rabbitVRFParticipationProofFixtureV1) {
			f.members[0].Participant = common.HexToAddress("0x1234")
		},
	}

	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			bad := fixture
			bad.members = append([]RabbitVRFCommitteeMemberV1(nil), fixture.members...)
			bad.partialMessageIDs = append([]common.Hash(nil), fixture.partialMessageIDs...)
			bad.bindings = append([]RabbitVRFDKGTransportKeyBindingV1(nil), fixture.bindings...)
			bad.transportPrivateKeys = append([]*ecdsa.PrivateKey(nil), fixture.transportPrivateKeys...)
			bad.proof = cloneRabbitVRFParticipationProofV1(fixture.proof)
			mutate(&bad)

			_, err := ValidateRabbitVRFParticipationProofV1(
				bad.context,
				bad.transportKeySetRoot,
				bad.members,
				bad.bindings,
				bad.keysetRoot,
				bad.requestID,
				bad.messageHash,
				bad.proof,
			)
			if !errors.Is(err, ErrInvalidRabbitVRFParticipationProofV1) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestRabbitVRFParticipationProofV1CanonicalEncodingInputs(t *testing.T) {
	fixture := newRabbitVRFParticipationProofFixtureV1(t)
	proof := cloneRabbitVRFParticipationProofV1(fixture.proof)
	if !bytes.Equal(proof.Bitmap, []byte{0x07}) || len(proof.Signatures) != int(fixture.context.Threshold) {
		t.Fatal("unexpected canonical participation proof shape")
	}
}
