package lqc

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rlp"
)

const (
	LQCHeaderEnvelopeVersionV5               uint8 = 5
	MaxRabbitVRFFinalizationsPerBlockV1            = 32
	MaxRabbitVRFKeysetCertificatesPerBlockV1       = 1
)

var (
	ErrInvalidLQCHeaderExtraV5                    = errors.New("invalid lqc header extra v5")
	ErrUnsupportedLQCHeaderV5                     = errors.New("unsupported lqc header v5")
	ErrLQCHeaderBlockMismatchV5                   = errors.New("lqc header v5 block mismatch")
	ErrTooManyRabbitVRFFinalizationsPerBlock      = errors.New("too many rabbit vrf finalizations per block")
	ErrTooManyRabbitVRFKeysetCertificatesPerBlock = errors.New("too many rabbit vrf keyset certificates per block")
)

type LQCHeaderEnvelopeV5 struct {
	Version                      uint8
	BlockNumber                  uint64
	RegistryRoot                 common.Hash
	WorkStateRoot                common.Hash
	CommitteeClaimRoot           common.Hash
	RegistryOperations           []RegistryOperation
	WorkTickets                  []SignedRandomXWorkTicketV1
	CommitteeParticipationClaims []CommitteeParticipationClaimGroupV1
	RabbitVRFKeysetCertificates  []RabbitVRFKeysetCertificateV1
	RabbitVRFFinalizations       []RabbitVRFFinalizationV1
}

func EncodeLQCHeaderExtraV5(
	blockNumber uint64,
	registryRoot common.Hash,
	workStateRoot common.Hash,
	committeeClaimRoot common.Hash,
	registryOperations []RegistryOperation,
	workTickets []SignedRandomXWorkTicketV1,
	committeeClaims []CommitteeParticipationClaimGroupV1,
	vrfFinalizations []RabbitVRFFinalizationV1,
	maxWorkTickets uint64,
) ([]byte, error) {
	return encodeLQCHeaderExtraV5(blockNumber, registryRoot, workStateRoot, committeeClaimRoot, registryOperations, workTickets, committeeClaims, nil, vrfFinalizations, maxWorkTickets)
}

func EncodeLQCHeaderExtraV5WithKeysetCertificate(
	blockNumber uint64,
	registryRoot common.Hash,
	workStateRoot common.Hash,
	committeeClaimRoot common.Hash,
	registryOperations []RegistryOperation,
	workTickets []SignedRandomXWorkTicketV1,
	committeeClaims []CommitteeParticipationClaimGroupV1,
	certificate RabbitVRFKeysetCertificateV1,
	vrfFinalizations []RabbitVRFFinalizationV1,
	maxWorkTickets uint64,
) ([]byte, error) {
	return encodeLQCHeaderExtraV5(blockNumber, registryRoot, workStateRoot, committeeClaimRoot, registryOperations, workTickets, committeeClaims, []RabbitVRFKeysetCertificateV1{certificate}, vrfFinalizations, maxWorkTickets)
}

func encodeLQCHeaderExtraV5(
	blockNumber uint64,
	registryRoot common.Hash,
	workStateRoot common.Hash,
	committeeClaimRoot common.Hash,
	registryOperations []RegistryOperation,
	workTickets []SignedRandomXWorkTicketV1,
	committeeClaims []CommitteeParticipationClaimGroupV1,
	vrfKeysetCertificates []RabbitVRFKeysetCertificateV1,
	vrfFinalizations []RabbitVRFFinalizationV1,
	maxWorkTickets uint64,
) ([]byte, error) {
	if blockNumber == 0 || registryRoot == (common.Hash{}) ||
		workStateRoot == (common.Hash{}) || committeeClaimRoot == (common.Hash{}) ||
		maxWorkTickets == 0 {
		return nil, ErrInvalidLQCHeaderExtraV5
	}
	if len(vrfFinalizations) > MaxRabbitVRFFinalizationsPerBlockV1 {
		return nil, ErrTooManyRabbitVRFFinalizationsPerBlock
	}
	canonicalRegistry := CanonicalRegistryOperations(registryOperations)
	if err := validateRegistryHeaderOperations(canonicalRegistry); err != nil {
		return nil, err
	}
	canonicalTickets, err := CanonicalWorkTicketsV3(workTickets, maxWorkTickets)
	if err != nil {
		return nil, err
	}
	canonicalClaims, err := CanonicalCommitteeParticipationClaimGroupsV1(blockNumber, committeeClaims)
	if err != nil {
		return nil, err
	}
	if err := validateClaimRegistryOperationCapacityV4(canonicalRegistry, canonicalClaims); err != nil {
		return nil, err
	}
	canonicalFinalizations, err := CanonicalRabbitVRFFinalizationsV1(vrfFinalizations)
	if err != nil {
		return nil, err
	}

	envelope := LQCHeaderEnvelopeV5{
		Version:                      LQCHeaderEnvelopeVersionV5,
		BlockNumber:                  blockNumber,
		RegistryRoot:                 registryRoot,
		WorkStateRoot:                workStateRoot,
		CommitteeClaimRoot:           committeeClaimRoot,
		RegistryOperations:           canonicalRegistry,
		WorkTickets:                  canonicalTickets,
		CommitteeParticipationClaims: canonicalClaims,
		RabbitVRFKeysetCertificates:  append([]RabbitVRFKeysetCertificateV1(nil), vrfKeysetCertificates...),
		RabbitVRFFinalizations:       canonicalFinalizations,
	}
	payload, err := rlp.EncodeToBytes(envelope)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidLQCHeaderExtraV5, err)
	}
	extra := make([]byte, 0, len(registryHeaderMagic)+len(payload)+ProducerSealLength)
	extra = append(extra, registryHeaderMagic...)
	extra = append(extra, payload...)
	extra = appendEmptyProducerSeal(extra)
	if len(extra) > MaxRegistryHeaderExtraSize {
		return nil, fmt.Errorf("%w: %d > %d", ErrInvalidLQCHeaderExtraV5, len(extra), MaxRegistryHeaderExtraSize)
	}
	return extra, nil
}

func DecodeLQCHeaderExtraV5(extra []byte, maxWorkTickets uint64) (LQCHeaderEnvelopeV5, error) {
	if maxWorkTickets == 0 || len(extra) > MaxRegistryHeaderExtraSize {
		return LQCHeaderEnvelopeV5{}, ErrInvalidLQCHeaderExtraV5
	}
	payloadExtra, _, err := splitProducerSeal(extra)
	if err != nil || !IsRegistryHeaderExtra(payloadExtra) {
		return LQCHeaderEnvelopeV5{}, ErrInvalidLQCHeaderExtraV5
	}
	var envelope LQCHeaderEnvelopeV5
	if err := rlp.DecodeBytes(payloadExtra[len(registryHeaderMagic):], &envelope); err != nil {
		return LQCHeaderEnvelopeV5{}, fmt.Errorf("%w: %v", ErrInvalidLQCHeaderExtraV5, err)
	}
	if envelope.Version != LQCHeaderEnvelopeVersionV5 {
		return LQCHeaderEnvelopeV5{}, ErrUnsupportedLQCHeaderV5
	}
	if envelope.BlockNumber == 0 || envelope.RegistryRoot == (common.Hash{}) ||
		envelope.WorkStateRoot == (common.Hash{}) ||
		envelope.CommitteeClaimRoot == (common.Hash{}) ||
		len(envelope.RabbitVRFKeysetCertificates) > MaxRabbitVRFKeysetCertificatesPerBlockV1 ||
		len(envelope.RabbitVRFFinalizations) > MaxRabbitVRFFinalizationsPerBlockV1 {
		return LQCHeaderEnvelopeV5{}, ErrInvalidLQCHeaderExtraV5
	}
	canonicalRegistry := CanonicalRegistryOperations(envelope.RegistryOperations)
	if err := validateRegistryHeaderOperations(canonicalRegistry); err != nil {
		return LQCHeaderEnvelopeV5{}, err
	}
	if !registryOperationsEqual(canonicalRegistry, envelope.RegistryOperations) {
		return LQCHeaderEnvelopeV5{}, ErrNonCanonicalRegistryOperations
	}
	canonicalTickets, err := CanonicalWorkTicketsV3(envelope.WorkTickets, maxWorkTickets)
	if err != nil {
		return LQCHeaderEnvelopeV5{}, err
	}
	if !signedWorkTicketsEqualV3(canonicalTickets, envelope.WorkTickets) {
		return LQCHeaderEnvelopeV5{}, ErrNonCanonicalWorkTicketsV3
	}
	if err := ValidateCanonicalCommitteeParticipationClaimGroupsV1(
		envelope.BlockNumber,
		envelope.CommitteeParticipationClaims,
	); err != nil {
		return LQCHeaderEnvelopeV5{}, err
	}
	if err := validateClaimRegistryOperationCapacityV4(
		envelope.RegistryOperations,
		envelope.CommitteeParticipationClaims,
	); err != nil {
		return LQCHeaderEnvelopeV5{}, err
	}
	for _, certificate := range envelope.RabbitVRFKeysetCertificates {
		if err := ValidateRabbitVRFKeysetCertificateShapeV1(certificate); err != nil {
			return LQCHeaderEnvelopeV5{}, err
		}
	}
	if err := ValidateCanonicalRabbitVRFFinalizationsV1(envelope.RabbitVRFFinalizations); err != nil {
		return LQCHeaderEnvelopeV5{}, err
	}

	reencoded, err := encodeLQCHeaderExtraV5(
		envelope.BlockNumber,
		envelope.RegistryRoot,
		envelope.WorkStateRoot,
		envelope.CommitteeClaimRoot,
		envelope.RegistryOperations,
		envelope.WorkTickets,
		envelope.CommitteeParticipationClaims,
		envelope.RabbitVRFKeysetCertificates,
		envelope.RabbitVRFFinalizations,
		maxWorkTickets,
	)
	if err != nil {
		return LQCHeaderEnvelopeV5{}, err
	}
	reencodedPayload, _, err := splitProducerSeal(reencoded)
	if err != nil || !bytes.Equal(reencodedPayload, payloadExtra) {
		return LQCHeaderEnvelopeV5{}, ErrInvalidLQCHeaderExtraV5
	}
	return envelope, nil
}

func ValidateLQCHeaderExtraV5(
	blockNumber uint64,
	maxWorkTickets uint64,
	extra []byte,
) (LQCHeaderEnvelopeV5, error) {
	envelope, err := DecodeLQCHeaderExtraV5(extra, maxWorkTickets)
	if err != nil {
		return LQCHeaderEnvelopeV5{}, err
	}
	if envelope.BlockNumber != blockNumber {
		return LQCHeaderEnvelopeV5{}, ErrLQCHeaderBlockMismatchV5
	}
	return envelope, nil
}
