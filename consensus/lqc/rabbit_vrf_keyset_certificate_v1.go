package lqc

import (
	"errors"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	rabbitvrf "github.com/ethereum/go-ethereum/crypto/rabbitvrf"
	"github.com/ethereum/go-ethereum/rlp"
)

const RabbitVRFKeysetCertificateVersionV1 uint8 = 1

var ErrInvalidRabbitVRFKeysetCertificateV1 = errors.New(
	"invalid rabbit vrf keyset certificate v1",
)

var rabbitVRFKeysetCertificateDomainV1 = []byte(
	"RABBIT-VRF-KEYSET-CERTIFICATE-V1",
)

type RabbitVRFKeysetCertificateV1 struct {
	Version            uint8
	SessionID          common.Hash
	KeysetRoot         common.Hash
	ThresholdPublicKey rabbitvrf.PublicKey
	TranscriptRoot     common.Hash
	Signatures         [][]byte
}

type rabbitVRFKeysetCertificatePayloadV1 struct {
	Domain             []byte
	Version            uint8
	SessionID          common.Hash
	KeysetRoot         common.Hash
	ThresholdPublicKey rabbitvrf.PublicKey
	TranscriptRoot     common.Hash
}

func RabbitVRFKeysetCertificatePayloadHashV1(
	context RabbitVRFDKGSessionContextV1,
	certificate RabbitVRFKeysetCertificateV1,
) (common.Hash, error) {
	if err := ValidateRabbitVRFDKGSessionContextV1(context); err != nil {
		return common.Hash{}, ErrInvalidRabbitVRFKeysetCertificateV1
	}
	sessionID, err := RabbitVRFDKGSessionIDV1(context)
	if err != nil {
		return common.Hash{}, ErrInvalidRabbitVRFKeysetCertificateV1
	}
	if certificate.Version != RabbitVRFKeysetCertificateVersionV1 ||
		certificate.SessionID != sessionID ||
		certificate.KeysetRoot == (common.Hash{}) ||
		certificate.TranscriptRoot == (common.Hash{}) {
		return common.Hash{}, ErrInvalidRabbitVRFKeysetCertificateV1
	}
	if _, err := rabbitvrf.NewVerificationShare(
		1,
		certificate.ThresholdPublicKey,
	); err != nil {
		return common.Hash{}, ErrInvalidRabbitVRFKeysetCertificateV1
	}
	encoded, err := rlp.EncodeToBytes(
		rabbitVRFKeysetCertificatePayloadV1{
			Domain:             rabbitVRFKeysetCertificateDomainV1,
			Version:            certificate.Version,
			SessionID:          certificate.SessionID,
			KeysetRoot:         certificate.KeysetRoot,
			ThresholdPublicKey: certificate.ThresholdPublicKey,
			TranscriptRoot:     certificate.TranscriptRoot,
		},
	)
	if err != nil {
		return common.Hash{}, err
	}
	hash := crypto.Keccak256Hash(encoded)
	if hash == (common.Hash{}) {
		return common.Hash{}, ErrInvalidRabbitVRFKeysetCertificateV1
	}
	return hash, nil
}

func ValidateRabbitVRFKeysetCertificateShapeV1(
	certificate RabbitVRFKeysetCertificateV1,
) error {
	if certificate.Version != RabbitVRFKeysetCertificateVersionV1 ||
		certificate.SessionID == (common.Hash{}) ||
		certificate.KeysetRoot == (common.Hash{}) ||
		certificate.TranscriptRoot == (common.Hash{}) ||
		len(certificate.Signatures) == 0 {
		return ErrInvalidRabbitVRFKeysetCertificateV1
	}

	if _, err := rabbitvrf.NewVerificationShare(
		1,
		certificate.ThresholdPublicKey,
	); err != nil {
		return ErrInvalidRabbitVRFKeysetCertificateV1
	}

	for _, signature := range certificate.Signatures {
		if len(signature) == 0 {
			continue
		}
		if len(signature) != crypto.SignatureLength {
			return ErrInvalidRabbitVRFKeysetCertificateV1
		}
	}

	return nil
}

func ValidateRabbitVRFKeysetCertificateV1(
	context RabbitVRFDKGSessionContextV1,
	members []RabbitVRFCommitteeMemberV1,
	certificate RabbitVRFKeysetCertificateV1,
) (RabbitVRFKeysetCertificateV1, error) {
	if uint64(len(members)) != context.CommitteeSize ||
		uint64(len(certificate.Signatures)) != context.CommitteeSize {
		return RabbitVRFKeysetCertificateV1{},
			ErrInvalidRabbitVRFKeysetCertificateV1
	}

	payloadHash, err := RabbitVRFKeysetCertificatePayloadHashV1(
		context,
		certificate,
	)
	if err != nil {
		return RabbitVRFKeysetCertificateV1{}, err
	}

	canonical := make([][]byte, len(certificate.Signatures))
	var validSignatures uint64

	for index, member := range members {
		expectedShareID := uint64(index) + 1

		if member.ShareID != expectedShareID ||
			member.TicketHash == (common.Hash{}) ||
			member.Participant == (common.Address{}) {
			return RabbitVRFKeysetCertificateV1{},
				ErrInvalidRabbitVRFKeysetCertificateV1
		}

		signature := certificate.Signatures[index]

		// Empty slot means this canonical member did not sign.
		if len(signature) == 0 {
			continue
		}

		if len(signature) != crypto.SignatureLength {
			return RabbitVRFKeysetCertificateV1{},
				ErrInvalidRabbitVRFKeysetCertificateV1
		}

		envelope := RabbitVRFDKGEnvelopeV1{
			Version:       RabbitVRFDKGEnvelopeVersionV1,
			SessionID:     certificate.SessionID,
			MessageType:   RabbitVRFDKGMessageKeysetCertificateV1,
			SenderShareID: expectedShareID,
			Participant:   member.Participant,
			PayloadHash:   payloadHash,
			Signature:     append([]byte(nil), signature...),
		}

		if err := VerifyRabbitVRFDKGEnvelopeV1(
			context,
			member,
			envelope,
		); err != nil {
			return RabbitVRFKeysetCertificateV1{},
				ErrInvalidRabbitVRFKeysetCertificateV1
		}

		canonical[index] =
			append([]byte(nil), signature...)
		validSignatures++
	}

	if validSignatures < context.Threshold {
		return RabbitVRFKeysetCertificateV1{},
			ErrInvalidRabbitVRFKeysetCertificateV1
	}

	certificate.Signatures = canonical
	return certificate, nil
}
