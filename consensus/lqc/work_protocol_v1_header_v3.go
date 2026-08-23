package lqc

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
)

const LQCHeaderEnvelopeVersionV3 uint8 = 3

var (
	ErrInvalidLQCHeaderExtraV3   = errors.New("invalid lqc header extra v3")
	ErrUnsupportedLQCHeaderV3    = errors.New("unsupported lqc header v3")
	ErrLQCHeaderBlockMismatchV3  = errors.New("lqc header v3 block mismatch")
	ErrInvalidWorkTicketLimitV3  = errors.New("invalid lqc work ticket limit v3")
	ErrTooManyWorkTicketsV3      = errors.New("too many lqc work tickets v3")
	ErrDuplicateWorkTicketV3     = errors.New("duplicate lqc work ticket v3")
	ErrNonCanonicalWorkTicketsV3 = errors.New("non-canonical lqc work tickets v3")
)

// LQCHeaderEnvelopeV3 is an INACTIVE unified header payload.
//
// The existing binary LQC magic and producer-seal suffix are deliberately
// preserved. V3 adds WorkStateRoot and signed RandomX work submissions while
// retaining the canonical registry state and operations.
//
// This file does NOT make V3 active consensus.
type LQCHeaderEnvelopeV3 struct {
	Version            uint8
	BlockNumber        uint64
	RegistryRoot       common.Hash
	WorkStateRoot      common.Hash
	RegistryOperations []RegistryOperation
	WorkTickets        []SignedRandomXWorkTicketV1
}

type workTicketSemanticKeyV3 struct {
	Epoch       uint64
	Participant common.Address
	Nonce       uint64
}

func cloneSignedRandomXWorkTicketV1(
	input SignedRandomXWorkTicketV1,
) SignedRandomXWorkTicketV1 {
	out := input
	out.Signature = append([]byte(nil), input.Signature...)
	return out
}

// validateWorkTicketHeaderShapeV3 performs only CHEAP checks suitable before
// future RandomX recomputation. Full proof + ownership validation remains in
// ValidateRecomputedRandomXWorkV1.
func validateWorkTicketHeaderShapeV3(
	signed SignedRandomXWorkTicketV1,
) error {
	if err := validateRandomXWorkTicketV1(signed.Ticket); err != nil {
		return err
	}
	if len(signed.Signature) != crypto.SignatureLength {
		return ErrInvalidRandomXWorkSignature
	}

	r := new(big.Int).SetBytes(signed.Signature[:32])
	s := new(big.Int).SetBytes(signed.Signature[32:64])
	v := signed.Signature[64]
	if !crypto.ValidateSignatureValues(v, r, s, true) {
		return ErrInvalidRandomXWorkSignature
	}
	return nil
}

// CanonicalWorkTicketsV3 canonicalizes by semantic ticket identity, not pool
// arrival order. A participant cannot submit the same epoch+nonce twice even
// with a different signature.
func CanonicalWorkTicketsV3(
	input []SignedRandomXWorkTicketV1,
	maxWorkTickets uint64,
) ([]SignedRandomXWorkTicketV1, error) {
	if maxWorkTickets == 0 {
		return nil, ErrInvalidWorkTicketLimitV3
	}
	if uint64(len(input)) > maxWorkTickets {
		return nil, ErrTooManyWorkTicketsV3
	}

	out := make([]SignedRandomXWorkTicketV1, len(input))
	seen := make(map[workTicketSemanticKeyV3]struct{}, len(input))

	for index, signed := range input {
		if err := validateWorkTicketHeaderShapeV3(signed); err != nil {
			return nil, err
		}

		key := workTicketSemanticKeyV3{
			Epoch:       signed.Ticket.Epoch,
			Participant: signed.Ticket.Participant,
			Nonce:       signed.Ticket.Nonce,
		}
		if _, exists := seen[key]; exists {
			return nil, ErrDuplicateWorkTicketV3
		}
		seen[key] = struct{}{}
		out[index] = cloneSignedRandomXWorkTicketV1(signed)
	}

	sort.Slice(out, func(i, j int) bool {
		left := out[i].Ticket
		right := out[j].Ticket

		if left.Epoch != right.Epoch {
			return left.Epoch < right.Epoch
		}
		if order := left.Participant.Cmp(right.Participant); order != 0 {
			return order < 0
		}
		if left.Nonce != right.Nonce {
			return left.Nonce < right.Nonce
		}
		return bytes.Compare(out[i].Signature, out[j].Signature) < 0
	})

	return out, nil
}

func signedWorkTicketsEqualV3(
	left,
	right []SignedRandomXWorkTicketV1,
) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].Ticket != right[index].Ticket ||
			!bytes.Equal(left[index].Signature, right[index].Signature) {
			return false
		}
	}
	return true
}

// EncodeLQCHeaderExtraV3 builds the unified V3 payload using the SAME LQC magic,
// SAME global 16 KiB extra-data bound, and SAME producer-seal suffix convention
// already used by Registry Header V2.
//
// maxWorkTickets is deliberately supplied by the caller. This foundation does
// not freeze Rabbit mainnet's verification-capacity policy before that separate
// benchmark/gate is completed.
func EncodeLQCHeaderExtraV3(
	blockNumber uint64,
	registryRoot common.Hash,
	workStateRoot common.Hash,
	registryOperations []RegistryOperation,
	workTickets []SignedRandomXWorkTicketV1,
	maxWorkTickets uint64,
) ([]byte, error) {
	if blockNumber == 0 ||
		registryRoot == (common.Hash{}) ||
		workStateRoot == (common.Hash{}) {
		return nil, ErrInvalidLQCHeaderExtraV3
	}
	if maxWorkTickets == 0 {
		return nil, ErrInvalidWorkTicketLimitV3
	}

	canonicalRegistry := CanonicalRegistryOperations(registryOperations)
	if err := validateRegistryHeaderOperations(canonicalRegistry); err != nil {
		return nil, err
	}

	canonicalTickets, err := CanonicalWorkTicketsV3(
		workTickets,
		maxWorkTickets,
	)
	if err != nil {
		return nil, err
	}

	envelope := LQCHeaderEnvelopeV3{
		Version:            LQCHeaderEnvelopeVersionV3,
		BlockNumber:        blockNumber,
		RegistryRoot:       registryRoot,
		WorkStateRoot:      workStateRoot,
		RegistryOperations: canonicalRegistry,
		WorkTickets:        canonicalTickets,
	}

	payload, err := rlp.EncodeToBytes(envelope)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidLQCHeaderExtraV3, err)
	}

	extra := make(
		[]byte,
		0,
		len(registryHeaderMagic)+len(payload)+ProducerSealLength,
	)
	extra = append(extra, registryHeaderMagic...)
	extra = append(extra, payload...)
	extra = appendEmptyProducerSeal(extra)

	if len(extra) > MaxRegistryHeaderExtraSize {
		return nil, fmt.Errorf(
			"%w: %d > %d",
			ErrInvalidLQCHeaderExtraV3,
			len(extra),
			MaxRegistryHeaderExtraSize,
		)
	}
	return extra, nil
}

// DecodeLQCHeaderExtraV3 validates bounded/canonical V3 syntax while ignoring
// the producer-seal BYTES for payload canonicality. Producer seal validity stays
// in the existing producer-seal verifier.
func DecodeLQCHeaderExtraV3(
	extra []byte,
	maxWorkTickets uint64,
) (LQCHeaderEnvelopeV3, error) {
	if maxWorkTickets == 0 {
		return LQCHeaderEnvelopeV3{}, ErrInvalidWorkTicketLimitV3
	}
	if len(extra) > MaxRegistryHeaderExtraSize {
		return LQCHeaderEnvelopeV3{}, ErrInvalidLQCHeaderExtraV3
	}

	payloadExtra, _, err := splitProducerSeal(extra)
	if err != nil {
		return LQCHeaderEnvelopeV3{}, ErrInvalidLQCHeaderExtraV3
	}

	if !IsRegistryHeaderExtra(payloadExtra) {
		return LQCHeaderEnvelopeV3{}, ErrInvalidLQCHeaderExtraV3
	}

	var envelope LQCHeaderEnvelopeV3
	if err := rlp.DecodeBytes(
		payloadExtra[len(registryHeaderMagic):],
		&envelope,
	); err != nil {
		return LQCHeaderEnvelopeV3{}, fmt.Errorf(
			"%w: %v",
			ErrInvalidLQCHeaderExtraV3,
			err,
		)
	}

	if envelope.Version != LQCHeaderEnvelopeVersionV3 {
		return LQCHeaderEnvelopeV3{}, ErrUnsupportedLQCHeaderV3
	}

	if envelope.BlockNumber == 0 ||
		envelope.RegistryRoot == (common.Hash{}) ||
		envelope.WorkStateRoot == (common.Hash{}) {
		return LQCHeaderEnvelopeV3{}, ErrInvalidLQCHeaderExtraV3
	}

	canonicalRegistry := CanonicalRegistryOperations(
		envelope.RegistryOperations,
	)
	if err := validateRegistryHeaderOperations(canonicalRegistry); err != nil {
		return LQCHeaderEnvelopeV3{}, err
	}
	if !registryOperationsEqual(
		canonicalRegistry,
		envelope.RegistryOperations,
	) {
		return LQCHeaderEnvelopeV3{}, ErrNonCanonicalRegistryOperations
	}

	canonicalTickets, err := CanonicalWorkTicketsV3(
		envelope.WorkTickets,
		maxWorkTickets,
	)
	if err != nil {
		return LQCHeaderEnvelopeV3{}, err
	}
	if !signedWorkTicketsEqualV3(
		canonicalTickets,
		envelope.WorkTickets,
	) {
		return LQCHeaderEnvelopeV3{}, ErrNonCanonicalWorkTicketsV3
	}

	reencoded, err := EncodeLQCHeaderExtraV3(
		envelope.BlockNumber,
		envelope.RegistryRoot,
		envelope.WorkStateRoot,
		envelope.RegistryOperations,
		envelope.WorkTickets,
		maxWorkTickets,
	)
	if err != nil {
		return LQCHeaderEnvelopeV3{}, err
	}

	reencodedPayload, _, err := splitProducerSeal(reencoded)
	if err != nil ||
		!bytes.Equal(reencodedPayload, payloadExtra) {
		return LQCHeaderEnvelopeV3{}, ErrInvalidLQCHeaderExtraV3
	}

	return envelope, nil
}

func ValidateLQCHeaderExtraV3(
	blockNumber uint64,
	maxWorkTickets uint64,
	extra []byte,
) (LQCHeaderEnvelopeV3, error) {
	envelope, err := DecodeLQCHeaderExtraV3(
		extra,
		maxWorkTickets,
	)
	if err != nil {
		return LQCHeaderEnvelopeV3{}, err
	}
	if envelope.BlockNumber != blockNumber {
		return LQCHeaderEnvelopeV3{}, ErrLQCHeaderBlockMismatchV3
	}
	return envelope, nil
}
