package lqc

import (
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
)

const (
	RabbitVRFDKGEnvelopeVersionV1 uint8 = 1

	RabbitVRFDKGMessagePolynomialCommitmentV1 uint8 = 1
	RabbitVRFDKGMessageTransportKeyBindingV1  uint8 = 2
	RabbitVRFDKGMessagePeerRouteV1            uint8 = 3
	RabbitVRFDKGMessageKeysetCertificateV1    uint8 = 4
)

var (
	ErrInvalidRabbitVRFDKGEnvelopeV1 = errors.New(
		"invalid rabbit vrf dkg envelope v1",
	)
	ErrInvalidRabbitVRFDKGEnvelopeSignatureV1 = errors.New(
		"invalid rabbit vrf dkg envelope signature v1",
	)
	ErrRabbitVRFDKGEnvelopeSenderMismatchV1 = errors.New(
		"rabbit vrf dkg envelope sender mismatch v1",
	)
)

var (
	rabbitVRFDKGEnvelopeSignDomainV1 = []byte(
		"RABBIT-VRF-DKG-ENVELOPE-SIGN-V1",
	)
	rabbitVRFDKGEnvelopeSlotDomainV1 = []byte(
		"RABBIT-VRF-DKG-ENVELOPE-SLOT-V1",
	)
	rabbitVRFDKGEnvelopeIDDomainV1 = []byte(
		"RABBIT-VRF-DKG-ENVELOPE-ID-V1",
	)
)

// RabbitVRFDKGEnvelopeV1 authenticates one public DKG protocol object to the
// canonical Participant wallet controlling one immutable VRF committee
// ShareID.
//
// V1 currently permits public polynomial commitments and authenticated
// transport-encryption public-key bindings.
//
// Raw private polynomial evaluations MUST NOT be placed in this public
// envelope. Their later transport requires recipient binding and encryption.
type RabbitVRFDKGEnvelopeV1 struct {
	Version       uint8
	SessionID     common.Hash
	MessageType   uint8
	SenderShareID uint64
	Participant   common.Address
	PayloadHash   common.Hash
	Signature     []byte
}

type rabbitVRFDKGEnvelopeSignPayloadV1 struct {
	Domain        []byte
	Version       uint8
	SessionID     common.Hash
	MessageType   uint8
	SenderShareID uint64
	Participant   common.Address
	PayloadHash   common.Hash
}

type rabbitVRFDKGEnvelopeSlotPayloadV1 struct {
	Domain        []byte
	Version       uint8
	SessionID     common.Hash
	MessageType   uint8
	SenderShareID uint64
}

type rabbitVRFDKGEnvelopeIDPayloadV1 struct {
	Domain      []byte
	SlotID      common.Hash
	Participant common.Address
	PayloadHash common.Hash
}

func validRabbitVRFDKGMessageTypeV1(
	messageType uint8,
) bool {
	return messageType ==
		RabbitVRFDKGMessagePolynomialCommitmentV1 ||
		messageType ==
			RabbitVRFDKGMessageTransportKeyBindingV1 ||
		messageType ==
			RabbitVRFDKGMessagePeerRouteV1 ||
		messageType ==
			RabbitVRFDKGMessageKeysetCertificateV1
}

func validateRabbitVRFDKGEnvelopeContextV1(
	context RabbitVRFDKGSessionContextV1,
	envelope RabbitVRFDKGEnvelopeV1,
) error {
	if err := ValidateRabbitVRFDKGSessionContextV1(
		context,
	); err != nil {
		return ErrInvalidRabbitVRFDKGEnvelopeV1
	}

	sessionID, err :=
		RabbitVRFDKGSessionIDV1(context)
	if err != nil {
		return ErrInvalidRabbitVRFDKGEnvelopeV1
	}

	if envelope.Version !=
		RabbitVRFDKGEnvelopeVersionV1 ||
		envelope.SessionID != sessionID ||
		!validRabbitVRFDKGMessageTypeV1(
			envelope.MessageType,
		) ||
		envelope.SenderShareID == 0 ||
		envelope.SenderShareID >
			context.CommitteeSize ||
		envelope.Participant ==
			(common.Address{}) ||
		envelope.PayloadHash ==
			(common.Hash{}) {
		return ErrInvalidRabbitVRFDKGEnvelopeV1
	}

	return nil
}

// NewRabbitVRFDKGEnvelopeV1 constructs an unsigned canonical envelope.
//
// The member supplied here must be the canonical member associated with the
// committee committed by the DKG session.
func NewRabbitVRFDKGEnvelopeV1(
	context RabbitVRFDKGSessionContextV1,
	sender RabbitVRFCommitteeMemberV1,
	messageType uint8,
	payloadHash common.Hash,
) (
	RabbitVRFDKGEnvelopeV1,
	error,
) {
	var out RabbitVRFDKGEnvelopeV1

	if err := ValidateRabbitVRFDKGSessionContextV1(
		context,
	); err != nil {
		return out, ErrInvalidRabbitVRFDKGEnvelopeV1
	}

	if sender.ShareID == 0 ||
		sender.ShareID > context.CommitteeSize ||
		sender.TicketHash == (common.Hash{}) ||
		sender.Participant == (common.Address{}) ||
		!validRabbitVRFDKGMessageTypeV1(
			messageType,
		) ||
		payloadHash == (common.Hash{}) {
		return out, ErrInvalidRabbitVRFDKGEnvelopeV1
	}

	sessionID, err :=
		RabbitVRFDKGSessionIDV1(context)
	if err != nil {
		return out, ErrInvalidRabbitVRFDKGEnvelopeV1
	}

	out = RabbitVRFDKGEnvelopeV1{
		Version:       RabbitVRFDKGEnvelopeVersionV1,
		SessionID:     sessionID,
		MessageType:   messageType,
		SenderShareID: sender.ShareID,
		Participant:   sender.Participant,
		PayloadHash:   payloadHash,
	}

	return out, nil
}

// RabbitVRFDKGEnvelopeSigningDataV1 returns the exact canonical RLP bytes
// supplied to accounts.Wallet.SignData.
//
// Wallet.SignData hashes these bytes once with Keccak256 before signing.
func RabbitVRFDKGEnvelopeSigningDataV1(
	context RabbitVRFDKGSessionContextV1,
	envelope RabbitVRFDKGEnvelopeV1,
) (
	[]byte,
	error,
) {
	if err := validateRabbitVRFDKGEnvelopeContextV1(
		context,
		envelope,
	); err != nil {
		return nil, err
	}

	return rlp.EncodeToBytes(
		rabbitVRFDKGEnvelopeSignPayloadV1{
			Domain:        rabbitVRFDKGEnvelopeSignDomainV1,
			Version:       envelope.Version,
			SessionID:     envelope.SessionID,
			MessageType:   envelope.MessageType,
			SenderShareID: envelope.SenderShareID,
			Participant:   envelope.Participant,
			PayloadHash:   envelope.PayloadHash,
		},
	)
}

// RabbitVRFDKGEnvelopeSigningHashV1 matches the hash produced by
// accounts.Wallet.SignData over RabbitVRFDKGEnvelopeSigningDataV1.
func RabbitVRFDKGEnvelopeSigningHashV1(
	context RabbitVRFDKGSessionContextV1,
	envelope RabbitVRFDKGEnvelopeV1,
) (
	common.Hash,
	error,
) {
	data, err :=
		RabbitVRFDKGEnvelopeSigningDataV1(
			context,
			envelope,
		)
	if err != nil {
		return common.Hash{}, err
	}

	return crypto.Keccak256Hash(data), nil
}

// RabbitVRFDKGEnvelopeSlotIDV1 identifies the unique singleton public-message
// slot for one sender in one DKG session.
//
// PayloadHash and Participant are deliberately excluded.
//
// Therefore two different payloads claiming the same
// (session, messageType, ShareID) produce the same SlotID and can later be
// treated as objective equivocation evidence.
func RabbitVRFDKGEnvelopeSlotIDV1(
	context RabbitVRFDKGSessionContextV1,
	envelope RabbitVRFDKGEnvelopeV1,
) (
	common.Hash,
	error,
) {
	if err := validateRabbitVRFDKGEnvelopeContextV1(
		context,
		envelope,
	); err != nil {
		return common.Hash{}, err
	}

	encoded, err := rlp.EncodeToBytes(
		rabbitVRFDKGEnvelopeSlotPayloadV1{
			Domain:        rabbitVRFDKGEnvelopeSlotDomainV1,
			Version:       envelope.Version,
			SessionID:     envelope.SessionID,
			MessageType:   envelope.MessageType,
			SenderShareID: envelope.SenderShareID,
		},
	)
	if err != nil {
		return common.Hash{}, err
	}

	slotID := crypto.Keccak256Hash(encoded)
	if slotID == (common.Hash{}) {
		return common.Hash{},
			ErrInvalidRabbitVRFDKGEnvelopeV1
	}

	return slotID, nil
}

// RabbitVRFDKGEnvelopeIDV1 identifies the exact authenticated message content.
//
// Signature bytes are deliberately excluded from the identity.
func RabbitVRFDKGEnvelopeIDV1(
	context RabbitVRFDKGSessionContextV1,
	envelope RabbitVRFDKGEnvelopeV1,
) (
	common.Hash,
	error,
) {
	slotID, err :=
		RabbitVRFDKGEnvelopeSlotIDV1(
			context,
			envelope,
		)
	if err != nil {
		return common.Hash{}, err
	}

	encoded, err := rlp.EncodeToBytes(
		rabbitVRFDKGEnvelopeIDPayloadV1{
			Domain:      rabbitVRFDKGEnvelopeIDDomainV1,
			SlotID:      slotID,
			Participant: envelope.Participant,
			PayloadHash: envelope.PayloadHash,
		},
	)
	if err != nil {
		return common.Hash{}, err
	}

	envelopeID := crypto.Keccak256Hash(encoded)
	if envelopeID == (common.Hash{}) {
		return common.Hash{},
			ErrInvalidRabbitVRFDKGEnvelopeV1
	}

	return envelopeID, nil
}

// VerifyRabbitVRFDKGEnvelopeV1 authenticates an envelope against the canonical
// committee member identified by SenderShareID.
//
// expected MUST come from the exact committee committed by context.CommitteeRoot.
// A P2P node key is never accepted as a substitute for expected.Participant.
func VerifyRabbitVRFDKGEnvelopeV1(
	context RabbitVRFDKGSessionContextV1,
	expected RabbitVRFCommitteeMemberV1,
	envelope RabbitVRFDKGEnvelopeV1,
) error {
	if err := validateRabbitVRFDKGEnvelopeContextV1(
		context,
		envelope,
	); err != nil {
		return err
	}

	if expected.ShareID == 0 ||
		expected.ShareID > context.CommitteeSize ||
		expected.TicketHash == (common.Hash{}) ||
		expected.Participant == (common.Address{}) ||
		expected.ShareID != envelope.SenderShareID ||
		expected.Participant != envelope.Participant {
		return ErrRabbitVRFDKGEnvelopeSenderMismatchV1
	}

	if len(envelope.Signature) !=
		crypto.SignatureLength {
		return ErrInvalidRabbitVRFDKGEnvelopeSignatureV1
	}

	r := new(big.Int).SetBytes(
		envelope.Signature[:32],
	)
	s := new(big.Int).SetBytes(
		envelope.Signature[32:64],
	)

	if !crypto.ValidateSignatureValues(
		envelope.Signature[64],
		r,
		s,
		true,
	) {
		return ErrInvalidRabbitVRFDKGEnvelopeSignatureV1
	}

	signingHash, err :=
		RabbitVRFDKGEnvelopeSigningHashV1(
			context,
			envelope,
		)
	if err != nil {
		return err
	}

	publicKey, err := crypto.SigToPub(
		signingHash[:],
		envelope.Signature,
	)
	if err != nil {
		return ErrInvalidRabbitVRFDKGEnvelopeSignatureV1
	}

	signer := crypto.PubkeyToAddress(
		*publicKey,
	)

	if signer != expected.Participant {
		return ErrInvalidRabbitVRFDKGEnvelopeSignatureV1
	}

	return nil
}

// RabbitVRFDKGPolynomialCommitmentPayloadHashV1 validates one dealer public
// polynomial commitment and returns its canonical commitment root for use as
// the authenticated envelope PayloadHash.
func RabbitVRFDKGPolynomialCommitmentPayloadHashV1(
	context RabbitVRFDKGSessionContextV1,
	commitment RabbitVRFDKGPolynomialCommitmentV1,
) (
	common.Hash,
	error,
) {
	root, err :=
		ValidateRabbitVRFDKGPolynomialCommitmentV1(
			context,
			commitment,
		)
	if err != nil {
		return common.Hash{},
			ErrInvalidRabbitVRFDKGEnvelopeV1
	}

	return root, nil
}
