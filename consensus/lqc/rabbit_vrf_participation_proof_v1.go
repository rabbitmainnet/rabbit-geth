package lqc

import (
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
)

const (
	RabbitVRFParticipationProofVersionV1  uint8 = 1
	RabbitVRFParticipationSignatureSizeV1       = crypto.SignatureLength - 1
)

var ErrInvalidRabbitVRFParticipationProofV1 = errors.New("invalid rabbit vrf participation proof v1")

var rabbitVRFParticipationSignDomainV1 = []byte("RABBIT-VRF-PARTICIPATION-SIGN-V1")

type RabbitVRFParticipationProofV1 struct {
	Version           uint8
	Bitmap            []byte
	PartialMessageIDs []common.Hash
	Signatures        [][]byte
}

type rabbitVRFParticipationSignPayloadV1 struct {
	Domain              []byte
	Version             uint8
	SessionID           common.Hash
	TransportKeySetRoot common.Hash
	KeysetRoot          common.Hash
	RequestID           common.Hash
	MessageHash         common.Hash
	PartialMessageID    common.Hash
	ShareID             uint64
	Participant         common.Address
}

func RabbitVRFParticipationSigningHashV1(
	context RabbitVRFDKGSessionContextV1,
	transportKeySetRoot common.Hash,
	keysetRoot common.Hash,
	requestID common.Hash,
	messageHash common.Hash,
	partialMessageID common.Hash,
	member RabbitVRFCommitteeMemberV1,
) (common.Hash, error) {
	if err := ValidateRabbitVRFDKGSessionContextV1(context); err != nil ||
		transportKeySetRoot == (common.Hash{}) ||
		keysetRoot == (common.Hash{}) ||
		requestID == (common.Hash{}) ||
		messageHash == (common.Hash{}) ||
		partialMessageID == (common.Hash{}) ||
		member.ShareID == 0 ||
		member.ShareID > context.CommitteeSize ||
		member.Participant == (common.Address{}) {
		return common.Hash{}, ErrInvalidRabbitVRFParticipationProofV1
	}

	sessionID, err := RabbitVRFDKGSessionIDV1(context)
	if err != nil || sessionID == (common.Hash{}) {
		return common.Hash{}, ErrInvalidRabbitVRFParticipationProofV1
	}

	encoded, err := rlp.EncodeToBytes(rabbitVRFParticipationSignPayloadV1{
		Domain:              append([]byte(nil), rabbitVRFParticipationSignDomainV1...),
		Version:             RabbitVRFParticipationProofVersionV1,
		SessionID:           sessionID,
		TransportKeySetRoot: transportKeySetRoot,
		KeysetRoot:          keysetRoot,
		RequestID:           requestID,
		MessageHash:         messageHash,
		PartialMessageID:    partialMessageID,
		ShareID:             member.ShareID,
		Participant:         member.Participant,
	})
	if err != nil {
		return common.Hash{}, fmt.Errorf("%w: %v", ErrInvalidRabbitVRFParticipationProofV1, err)
	}

	hash := crypto.Keccak256Hash(encoded)
	if hash == (common.Hash{}) {
		return common.Hash{}, ErrInvalidRabbitVRFParticipationProofV1
	}
	return hash, nil
}

func rabbitVRFParticipationBitmapLenV1(committeeSize uint64) int {
	return int((committeeSize + 7) / 8)
}

func rabbitVRFParticipationShareIDsV1(context RabbitVRFDKGSessionContextV1, bitmap []byte) ([]uint64, error) {
	if err := ValidateRabbitVRFDKGSessionContextV1(context); err != nil ||
		len(bitmap) != rabbitVRFParticipationBitmapLenV1(context.CommitteeSize) {
		return nil, ErrInvalidRabbitVRFParticipationProofV1
	}

	if remainder := context.CommitteeSize % 8; remainder != 0 {
		allowed := byte((uint16(1) << remainder) - 1)
		if bitmap[len(bitmap)-1]&^allowed != 0 {
			return nil, ErrInvalidRabbitVRFParticipationProofV1
		}
	}

	shareIDs := make([]uint64, 0, context.Threshold)
	for shareID := uint64(1); shareID <= context.CommitteeSize; shareID++ {
		bit := shareID - 1
		if bitmap[bit/8]&(byte(1)<<uint(bit%8)) != 0 {
			shareIDs = append(shareIDs, shareID)
		}
	}
	if uint64(len(shareIDs)) != context.Threshold {
		return nil, ErrInvalidRabbitVRFParticipationProofV1
	}
	return shareIDs, nil
}

func ValidateRabbitVRFParticipationProofV1(
	context RabbitVRFDKGSessionContextV1,
	transportKeySetRoot common.Hash,
	members []RabbitVRFCommitteeMemberV1,
	bindings []RabbitVRFDKGTransportKeyBindingV1,
	keysetRoot common.Hash,
	requestID common.Hash,
	messageHash common.Hash,
	proof RabbitVRFParticipationProofV1,
) ([]RabbitVRFCommitteeMemberV1, error) {
	if proof.Version != RabbitVRFParticipationProofVersionV1 ||
		transportKeySetRoot == (common.Hash{}) ||
		keysetRoot == (common.Hash{}) ||
		requestID == (common.Hash{}) ||
		messageHash == (common.Hash{}) ||
		uint64(len(members)) != context.CommitteeSize ||
		len(bindings) != len(members) {
		return nil, ErrInvalidRabbitVRFParticipationProofV1
	}

	shareIDs, err := rabbitVRFParticipationShareIDsV1(context, proof.Bitmap)
	if err != nil ||
		len(proof.PartialMessageIDs) != len(shareIDs) ||
		len(proof.Signatures) != len(shareIDs) {
		return nil, ErrInvalidRabbitVRFParticipationProofV1
	}

	participants := make([]RabbitVRFCommitteeMemberV1, len(shareIDs))
	for index, shareID := range shareIDs {
		member := members[shareID-1]
		binding := bindings[shareID-1]
		partialMessageID := proof.PartialMessageIDs[index]
		signature := proof.Signatures[index]

		if member.ShareID != shareID ||
			member.TicketHash == (common.Hash{}) ||
			member.Participant == (common.Address{}) ||
			partialMessageID == (common.Hash{}) ||
			len(signature) != RabbitVRFParticipationSignatureSizeV1 {
			return nil, ErrInvalidRabbitVRFParticipationProofV1
		}
		if _, err := VerifyRabbitVRFDKGTransportKeyBindingV1(context, member, binding); err != nil {
			return nil, ErrInvalidRabbitVRFParticipationProofV1
		}

		signingHash, err := RabbitVRFParticipationSigningHashV1(
			context,
			transportKeySetRoot,
			keysetRoot,
			requestID,
			messageHash,
			partialMessageID,
			member,
		)
		if err != nil {
			return nil, err
		}

		publicKey, err := parseRabbitVRFDKGTransportPublicKeyV1(binding.PublicKey)
		if err != nil || !crypto.VerifySignature(
			crypto.FromECDSAPub(publicKey),
			signingHash[:],
			signature,
		) {
			return nil, ErrInvalidRabbitVRFParticipationProofV1
		}
		participants[index] = member
	}

	return participants, nil
}
