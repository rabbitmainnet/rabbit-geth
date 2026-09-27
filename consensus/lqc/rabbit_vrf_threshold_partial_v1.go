package lqc

import (
	"errors"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	rabbitvrf "github.com/ethereum/go-ethereum/crypto/rabbitvrf"
	"github.com/ethereum/go-ethereum/rlp"
)

var (
	rabbitVRFThresholdMessageDomainV1 = []byte("RABBIT-VRF-THRESHOLD-MESSAGE-V1")

	ErrInvalidRabbitVRFThresholdPartialV1 = errors.New("invalid rabbit vrf threshold partial v1")
)

type rabbitVRFThresholdMessagePayloadV1 struct {
	Domain     []byte
	SessionID  common.Hash
	KeysetRoot common.Hash
	RequestID  common.Hash
}

type RabbitVRFThresholdPartialV1 struct {
	SessionID   common.Hash
	KeysetRoot  common.Hash
	RequestID   common.Hash
	MessageHash common.Hash
	ShareID     uint64
	Signature   rabbitvrf.Signature
}

func RabbitVRFThresholdMessageV1(
	context RabbitVRFDKGSessionContextV1,
	keysetRoot common.Hash,
	requestID common.Hash,
) ([]byte, common.Hash, error) {
	if keysetRoot == (common.Hash{}) || requestID == (common.Hash{}) {
		return nil, common.Hash{}, ErrInvalidRabbitVRFThresholdPartialV1
	}

	sessionID, err := RabbitVRFDKGSessionIDV1(context)
	if err != nil || sessionID == (common.Hash{}) {
		return nil, common.Hash{}, ErrInvalidRabbitVRFThresholdPartialV1
	}

	encoded, err := rlp.EncodeToBytes(rabbitVRFThresholdMessagePayloadV1{
		Domain:     append([]byte(nil), rabbitVRFThresholdMessageDomainV1...),
		SessionID:  sessionID,
		KeysetRoot: keysetRoot,
		RequestID:  requestID,
	})
	if err != nil {
		return nil, common.Hash{}, ErrInvalidRabbitVRFThresholdPartialV1
	}

	messageHash := crypto.Keccak256Hash(encoded)
	if messageHash == (common.Hash{}) {
		return nil, common.Hash{}, ErrInvalidRabbitVRFThresholdPartialV1
	}

	return encoded, messageHash, nil
}

func NewRabbitVRFThresholdPartialV1(
	context RabbitVRFDKGSessionContextV1,
	keysetRoot common.Hash,
	requestID common.Hash,
	partial rabbitvrf.PartialSignature,
) (RabbitVRFThresholdPartialV1, error) {
	var out RabbitVRFThresholdPartialV1

	if partial.ShareID == 0 || partial.ShareID > context.CommitteeSize {
		return out, ErrInvalidRabbitVRFThresholdPartialV1
	}

	sessionID, err := RabbitVRFDKGSessionIDV1(context)
	if err != nil {
		return out, ErrInvalidRabbitVRFThresholdPartialV1
	}

	_, messageHash, err := RabbitVRFThresholdMessageV1(
		context,
		keysetRoot,
		requestID,
	)
	if err != nil {
		return out, err
	}

	out.SessionID = sessionID
	out.KeysetRoot = keysetRoot
	out.RequestID = requestID
	out.MessageHash = messageHash
	out.ShareID = partial.ShareID
	out.Signature = partial.Signature

	return out, nil
}

func RabbitVRFThresholdPartialMessageIDV1(
	partial RabbitVRFThresholdPartialV1,
) (common.Hash, error) {
	if partial.SessionID == (common.Hash{}) ||
		partial.KeysetRoot == (common.Hash{}) ||
		partial.RequestID == (common.Hash{}) ||
		partial.MessageHash == (common.Hash{}) ||
		partial.ShareID == 0 {
		return common.Hash{}, ErrInvalidRabbitVRFThresholdPartialV1
	}

	encoded, err := rlp.EncodeToBytes(partial)
	if err != nil {
		return common.Hash{}, ErrInvalidRabbitVRFThresholdPartialV1
	}

	messageID := crypto.Keccak256Hash(encoded)
	if messageID == (common.Hash{}) {
		return common.Hash{}, ErrInvalidRabbitVRFThresholdPartialV1
	}

	return messageID, nil
}
