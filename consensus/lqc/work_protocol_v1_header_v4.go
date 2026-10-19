package lqc

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rlp"
)

const (
	LQCHeaderEnvelopeVersionV4                 uint8 = 4
	MaxRegistryOperationsWithCommitteeClaimsV1       = 48
)

var (
	ErrInvalidLQCHeaderExtraV4          = errors.New("invalid lqc header extra v4")
	ErrUnsupportedLQCHeaderV4           = errors.New("unsupported lqc header v4")
	ErrLQCHeaderBlockMismatchV4         = errors.New("lqc header v4 block mismatch")
	ErrTooManyClaimRegistryOperationsV4 = errors.New("too many registry operations with committee claims v4")
)

// LQCHeaderEnvelopeV4 is an INACTIVE extension of V3. Historical V3 encoding
// remains unchanged. V4 adds delayed, compact committee participation claims.
type LQCHeaderEnvelopeV4 struct {
	Version                      uint8
	BlockNumber                  uint64
	RegistryRoot                 common.Hash
	WorkStateRoot                common.Hash
	CommitteeClaimRoot           common.Hash
	RegistryOperations           []RegistryOperation
	WorkTickets                  []SignedRandomXWorkTicketV1
	CommitteeParticipationClaims []CommitteeParticipationClaimGroupV1
}

func validateClaimRegistryOperationCapacityV4(
	operations []RegistryOperation,
	claims []CommitteeParticipationClaimGroupV1,
) error {
	if len(claims) > 0 && len(operations) > MaxRegistryOperationsWithCommitteeClaimsV1 {
		return ErrTooManyClaimRegistryOperationsV4
	}
	return nil
}

func EncodeLQCHeaderExtraV4(
	blockNumber uint64,
	registryRoot common.Hash,
	workStateRoot common.Hash,
	committeeClaimRoot common.Hash,
	registryOperations []RegistryOperation,
	workTickets []SignedRandomXWorkTicketV1,
	committeeClaims []CommitteeParticipationClaimGroupV1,
	maxWorkTickets uint64,
) ([]byte, error) {
	if blockNumber == 0 || registryRoot == (common.Hash{}) ||
		workStateRoot == (common.Hash{}) || committeeClaimRoot == (common.Hash{}) ||
		maxWorkTickets == 0 {
		return nil, ErrInvalidLQCHeaderExtraV4
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
	envelope := LQCHeaderEnvelopeV4{
		Version:                      LQCHeaderEnvelopeVersionV4,
		BlockNumber:                  blockNumber,
		RegistryRoot:                 registryRoot,
		WorkStateRoot:                workStateRoot,
		CommitteeClaimRoot:           committeeClaimRoot,
		RegistryOperations:           canonicalRegistry,
		WorkTickets:                  canonicalTickets,
		CommitteeParticipationClaims: canonicalClaims,
	}
	payload, err := rlp.EncodeToBytes(envelope)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidLQCHeaderExtraV4, err)
	}
	extra := make([]byte, 0, len(registryHeaderMagic)+len(payload)+ProducerSealLength)
	extra = append(extra, registryHeaderMagic...)
	extra = append(extra, payload...)
	extra = appendEmptyProducerSeal(extra)
	if len(extra) > MaxRegistryHeaderExtraSize {
		return nil, fmt.Errorf("%w: %d > %d", ErrInvalidLQCHeaderExtraV4, len(extra), MaxRegistryHeaderExtraSize)
	}
	return extra, nil
}

func DecodeLQCHeaderExtraV4(extra []byte, maxWorkTickets uint64) (LQCHeaderEnvelopeV4, error) {
	if maxWorkTickets == 0 || len(extra) > MaxRegistryHeaderExtraSize {
		return LQCHeaderEnvelopeV4{}, ErrInvalidLQCHeaderExtraV4
	}
	payloadExtra, _, err := splitProducerSeal(extra)
	if err != nil || !IsRegistryHeaderExtra(payloadExtra) {
		return LQCHeaderEnvelopeV4{}, ErrInvalidLQCHeaderExtraV4
	}
	var envelope LQCHeaderEnvelopeV4
	if err := rlp.DecodeBytes(payloadExtra[len(registryHeaderMagic):], &envelope); err != nil {
		return LQCHeaderEnvelopeV4{}, fmt.Errorf("%w: %v", ErrInvalidLQCHeaderExtraV4, err)
	}
	if envelope.Version != LQCHeaderEnvelopeVersionV4 {
		return LQCHeaderEnvelopeV4{}, ErrUnsupportedLQCHeaderV4
	}
	if envelope.BlockNumber == 0 || envelope.RegistryRoot == (common.Hash{}) ||
		envelope.WorkStateRoot == (common.Hash{}) ||
		envelope.CommitteeClaimRoot == (common.Hash{}) {
		return LQCHeaderEnvelopeV4{}, ErrInvalidLQCHeaderExtraV4
	}
	canonicalRegistry := CanonicalRegistryOperations(envelope.RegistryOperations)
	if err := validateRegistryHeaderOperations(canonicalRegistry); err != nil {
		return LQCHeaderEnvelopeV4{}, err
	}
	if !registryOperationsEqual(canonicalRegistry, envelope.RegistryOperations) {
		return LQCHeaderEnvelopeV4{}, ErrNonCanonicalRegistryOperations
	}
	canonicalTickets, err := CanonicalWorkTicketsV3(envelope.WorkTickets, maxWorkTickets)
	if err != nil {
		return LQCHeaderEnvelopeV4{}, err
	}
	if !signedWorkTicketsEqualV3(canonicalTickets, envelope.WorkTickets) {
		return LQCHeaderEnvelopeV4{}, ErrNonCanonicalWorkTicketsV3
	}
	if err := ValidateCanonicalCommitteeParticipationClaimGroupsV1(
		envelope.BlockNumber,
		envelope.CommitteeParticipationClaims,
	); err != nil {
		return LQCHeaderEnvelopeV4{}, err
	}
	if err := validateClaimRegistryOperationCapacityV4(
		envelope.RegistryOperations,
		envelope.CommitteeParticipationClaims,
	); err != nil {
		return LQCHeaderEnvelopeV4{}, err
	}
	reencoded, err := EncodeLQCHeaderExtraV4(
		envelope.BlockNumber,
		envelope.RegistryRoot,
		envelope.WorkStateRoot,
		envelope.CommitteeClaimRoot,
		envelope.RegistryOperations,
		envelope.WorkTickets,
		envelope.CommitteeParticipationClaims,
		maxWorkTickets,
	)
	if err != nil {
		return LQCHeaderEnvelopeV4{}, err
	}
	reencodedPayload, _, err := splitProducerSeal(reencoded)
	if err != nil || !bytes.Equal(reencodedPayload, payloadExtra) {
		return LQCHeaderEnvelopeV4{}, ErrInvalidLQCHeaderExtraV4
	}
	return envelope, nil
}

func ValidateLQCHeaderExtraV4(
	blockNumber uint64,
	maxWorkTickets uint64,
	extra []byte,
) (LQCHeaderEnvelopeV4, error) {
	envelope, err := DecodeLQCHeaderExtraV4(extra, maxWorkTickets)
	if err != nil {
		return LQCHeaderEnvelopeV4{}, err
	}
	if envelope.BlockNumber != blockNumber {
		return LQCHeaderEnvelopeV4{}, ErrLQCHeaderBlockMismatchV4
	}
	return envelope, nil
}
