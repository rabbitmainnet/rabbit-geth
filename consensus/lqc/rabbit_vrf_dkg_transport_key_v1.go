package lqc

import (
	"bytes"
	"crypto/ecdsa"
	"errors"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
)

const (
	RabbitVRFDKGTransportKeyVersionV1 uint8 = 1

	RabbitVRFDKGTransportSchemeECIESSecp256k1AES128SHA256V1 uint8 = 1

	RabbitVRFDKGTransportPublicKeySizeV1 = 33
)

var (
	ErrInvalidRabbitVRFDKGTransportKeyV1 = errors.New(
		"invalid rabbit vrf dkg transport key v1",
	)

	ErrRabbitVRFDKGTransportKeySenderMismatchV1 = errors.New(
		"rabbit vrf dkg transport key sender mismatch v1",
	)

	ErrRabbitVRFDKGTransportKeyReusesParticipantKeyV1 = errors.New(
		"rabbit vrf dkg transport key reuses participant wallet key v1",
	)
)

var rabbitVRFDKGTransportKeyDomainV1 = []byte(
	"RABBIT-VRF-DKG-TRANSPORT-KEY-V1",
)

// RabbitVRFDKGTransportPublicKeyV1 is the canonical 33-byte compressed
// secp256k1 public key used for one participant's DKG private transport.
//
// Its corresponding private key is distinct from both:
//   - the Participant wallet private key;
//   - the P2P node private key.
type RabbitVRFDKGTransportPublicKeyV1 [RabbitVRFDKGTransportPublicKeySizeV1]byte

type RabbitVRFDKGTransportKeyBindingV1 struct {
	Version     uint8
	SessionID   common.Hash
	ShareID     uint64
	Participant common.Address
	Scheme      uint8
	PublicKey   RabbitVRFDKGTransportPublicKeyV1
}

type rabbitVRFDKGTransportKeyPayloadV1 struct {
	Domain      []byte
	Version     uint8
	SessionID   common.Hash
	ShareID     uint64
	Participant common.Address
	Scheme      uint8
	PublicKey   RabbitVRFDKGTransportPublicKeyV1
}

func parseRabbitVRFDKGTransportPublicKeyV1(
	publicKey RabbitVRFDKGTransportPublicKeyV1,
) (*ecdsa.PublicKey, error) {
	decoded, err := crypto.DecompressPubkey(
		publicKey[:],
	)
	if err != nil ||
		decoded == nil ||
		decoded.X == nil ||
		decoded.Y == nil ||
		!crypto.S256().IsOnCurve(
			decoded.X,
			decoded.Y,
		) {
		return nil, ErrInvalidRabbitVRFDKGTransportKeyV1
	}

	canonical := crypto.CompressPubkey(decoded)

	if len(canonical) !=
		RabbitVRFDKGTransportPublicKeySizeV1 ||
		!bytes.Equal(
			canonical,
			publicKey[:],
		) {
		return nil, ErrInvalidRabbitVRFDKGTransportKeyV1
	}

	return decoded, nil
}

func RabbitVRFDKGTransportPublicKeyV1FromBytes(
	encoded []byte,
) (
	RabbitVRFDKGTransportPublicKeyV1,
	error,
) {
	var out RabbitVRFDKGTransportPublicKeyV1

	if len(encoded) !=
		RabbitVRFDKGTransportPublicKeySizeV1 {
		return out, ErrInvalidRabbitVRFDKGTransportKeyV1
	}

	copy(out[:], encoded)

	if _, err :=
		parseRabbitVRFDKGTransportPublicKeyV1(
			out,
		); err != nil {
		return RabbitVRFDKGTransportPublicKeyV1{},
			err
	}

	return out, nil
}

func ValidateRabbitVRFDKGTransportPublicKeyV1(
	publicKey RabbitVRFDKGTransportPublicKeyV1,
) error {
	_, err :=
		parseRabbitVRFDKGTransportPublicKeyV1(
			publicKey,
		)
	return err
}

func validateRabbitVRFDKGTransportKeyBindingV1(
	context RabbitVRFDKGSessionContextV1,
	binding RabbitVRFDKGTransportKeyBindingV1,
) (
	*ecdsa.PublicKey,
	error,
) {
	if err :=
		ValidateRabbitVRFDKGSessionContextV1(
			context,
		); err != nil {
		return nil, ErrInvalidRabbitVRFDKGTransportKeyV1
	}

	sessionID, err :=
		RabbitVRFDKGSessionIDV1(context)
	if err != nil {
		return nil, ErrInvalidRabbitVRFDKGTransportKeyV1
	}

	if binding.Version !=
		RabbitVRFDKGTransportKeyVersionV1 ||
		binding.SessionID != sessionID ||
		binding.ShareID == 0 ||
		binding.ShareID >
			context.CommitteeSize ||
		binding.Participant ==
			(common.Address{}) ||
		binding.Scheme !=
			RabbitVRFDKGTransportSchemeECIESSecp256k1AES128SHA256V1 {
		return nil, ErrInvalidRabbitVRFDKGTransportKeyV1
	}

	publicKey, err :=
		parseRabbitVRFDKGTransportPublicKeyV1(
			binding.PublicKey,
		)
	if err != nil {
		return nil, err
	}

	// This prevents direct reuse of the Participant wallet key.
	//
	// P2P node-key separation belongs to the runtime because consensus does
	// not know a node's local P2P private key.
	if crypto.PubkeyToAddress(
		*publicKey,
	) == binding.Participant {
		return nil,
			ErrRabbitVRFDKGTransportKeyReusesParticipantKeyV1
	}

	return publicKey, nil
}

func NewRabbitVRFDKGTransportKeyBindingV1(
	context RabbitVRFDKGSessionContextV1,
	member RabbitVRFCommitteeMemberV1,
	publicKey RabbitVRFDKGTransportPublicKeyV1,
) (
	RabbitVRFDKGTransportKeyBindingV1,
	common.Hash,
	error,
) {
	var out RabbitVRFDKGTransportKeyBindingV1

	if err :=
		ValidateRabbitVRFDKGSessionContextV1(
			context,
		); err != nil {
		return out,
			common.Hash{},
			ErrInvalidRabbitVRFDKGTransportKeyV1
	}

	if member.ShareID == 0 ||
		member.ShareID >
			context.CommitteeSize ||
		member.TicketHash ==
			(common.Hash{}) ||
		member.Participant ==
			(common.Address{}) {
		return out,
			common.Hash{},
			ErrInvalidRabbitVRFDKGTransportKeyV1
	}

	sessionID, err :=
		RabbitVRFDKGSessionIDV1(context)
	if err != nil {
		return out,
			common.Hash{},
			ErrInvalidRabbitVRFDKGTransportKeyV1
	}

	out = RabbitVRFDKGTransportKeyBindingV1{
		Version:     RabbitVRFDKGTransportKeyVersionV1,
		SessionID:   sessionID,
		ShareID:     member.ShareID,
		Participant: member.Participant,
		Scheme:      RabbitVRFDKGTransportSchemeECIESSecp256k1AES128SHA256V1,
		PublicKey:   publicKey,
	}

	root, err :=
		RabbitVRFDKGTransportKeyRootV1(
			context,
			out,
		)
	if err != nil {
		return RabbitVRFDKGTransportKeyBindingV1{},
			common.Hash{},
			err
	}

	return out, root, nil
}

func RabbitVRFDKGTransportKeyRootV1(
	context RabbitVRFDKGSessionContextV1,
	binding RabbitVRFDKGTransportKeyBindingV1,
) (
	common.Hash,
	error,
) {
	if _, err :=
		validateRabbitVRFDKGTransportKeyBindingV1(
			context,
			binding,
		); err != nil {
		return common.Hash{}, err
	}

	encoded, err :=
		rlp.EncodeToBytes(
			rabbitVRFDKGTransportKeyPayloadV1{
				Domain:      rabbitVRFDKGTransportKeyDomainV1,
				Version:     binding.Version,
				SessionID:   binding.SessionID,
				ShareID:     binding.ShareID,
				Participant: binding.Participant,
				Scheme:      binding.Scheme,
				PublicKey:   binding.PublicKey,
			},
		)
	if err != nil {
		return common.Hash{},
			ErrInvalidRabbitVRFDKGTransportKeyV1
	}

	root := crypto.Keccak256Hash(encoded)

	if root == (common.Hash{}) {
		return common.Hash{},
			ErrInvalidRabbitVRFDKGTransportKeyV1
	}

	return root, nil
}

func VerifyRabbitVRFDKGTransportKeyBindingV1(
	context RabbitVRFDKGSessionContextV1,
	expected RabbitVRFCommitteeMemberV1,
	binding RabbitVRFDKGTransportKeyBindingV1,
) (
	common.Hash,
	error,
) {
	if expected.ShareID == 0 ||
		expected.ShareID >
			context.CommitteeSize ||
		expected.TicketHash ==
			(common.Hash{}) ||
		expected.Participant ==
			(common.Address{}) ||
		expected.ShareID !=
			binding.ShareID ||
		expected.Participant !=
			binding.Participant {
		return common.Hash{},
			ErrRabbitVRFDKGTransportKeySenderMismatchV1
	}

	return RabbitVRFDKGTransportKeyRootV1(
		context,
		binding,
	)
}

func VerifyRabbitVRFDKGTransportKeyEnvelopeV1(
	context RabbitVRFDKGSessionContextV1,
	expected RabbitVRFCommitteeMemberV1,
	binding RabbitVRFDKGTransportKeyBindingV1,
	envelope RabbitVRFDKGEnvelopeV1,
) error {
	root, err :=
		VerifyRabbitVRFDKGTransportKeyBindingV1(
			context,
			expected,
			binding,
		)
	if err != nil {
		return err
	}

	if envelope.MessageType !=
		RabbitVRFDKGMessageTransportKeyBindingV1 ||
		envelope.SenderShareID !=
			binding.ShareID ||
		envelope.Participant !=
			binding.Participant ||
		envelope.PayloadHash != root {
		return ErrInvalidRabbitVRFDKGTransportKeyV1
	}

	return VerifyRabbitVRFDKGEnvelopeV1(
		context,
		expected,
		envelope,
	)
}
