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

const (
	RegistryHeaderEnvelopeVersion uint8 = 2
	MaxRegistryOperationsPerBlock       = 64
	MaxRegistryHeaderExtraSize          = 16 * 1024
)

var (
	registryHeaderMagic = []byte{'L', 'Q', 'C', 0}

	ErrInvalidRegistryHeaderExtra     = errors.New("invalid lqc registry header extra")
	ErrUnsupportedRegistryHeader      = errors.New("unsupported lqc registry header version")
	ErrRegistryHeaderBlockMismatch    = errors.New("lqc registry header block number mismatch")
	ErrInvalidRegistryRoot            = errors.New("invalid lqc registry root")
	ErrTooManyRegistryOperations      = errors.New("too many lqc registry operations")
	ErrDuplicateRegistryOperation     = errors.New("duplicate lqc registry operation")
	ErrNonCanonicalRegistryOperations = errors.New("non-canonical lqc registry operation order")
)

// RegistryHeaderEnvelope is the versioned, canonical wire representation that
// will replace the legacy text Extra after the registry activation fork. Its
// root is a post-block registry commitment. Root verification is intentionally
// performed by the snapshot layer, not by this codec.
type RegistryHeaderEnvelope struct {
	Version      uint8
	BlockNumber  uint64
	RegistryRoot common.Hash
	Operations   []RegistryOperation
}

// IsRegistryHeaderExtra reports whether extra starts with the binary LQC
// registry envelope magic. It does not validate the payload.
func IsRegistryHeaderExtra(extra []byte) bool {
	return len(extra) >= len(registryHeaderMagic) && bytes.Equal(extra[:len(registryHeaderMagic)], registryHeaderMagic)
}

// CanonicalRegistryOperations returns a deep copy sorted independently of pool
// arrival order. Address and sequence are the primary consensus keys.
func CanonicalRegistryOperations(operations []RegistryOperation) []RegistryOperation {
	out := make([]RegistryOperation, len(operations))
	for index, operation := range operations {
		out[index] = operation
		out[index].Signature = bytes.Clone(operation.Signature)
	}
	sort.Slice(out, func(i, j int) bool {
		left, right := out[i], out[j]
		if order := left.Address.Cmp(right.Address); order != 0 {
			return order < 0
		}
		if left.Sequence != right.Sequence {
			return left.Sequence < right.Sequence
		}
		if left.Action != right.Action {
			return left.Action < right.Action
		}
		if left.ValidUntil != right.ValidUntil {
			return left.ValidUntil < right.ValidUntil
		}
		if left.ProofNonce != right.ProofNonce {
			return left.ProofNonce < right.ProofNonce
		}
		return bytes.Compare(left.Signature, right.Signature) < 0
	})
	return out
}

func validateRegistryHeaderOperations(operations []RegistryOperation) error {
	if len(operations) > MaxRegistryOperationsPerBlock {
		return ErrTooManyRegistryOperations
	}
	for index, operation := range operations {
		if operation.Version != RegistryProtocolVersion {
			return fmt.Errorf("operation %d: %w", index, ErrInvalidRegistryVersion)
		}
		if operation.Action < RegistryActionRegister || operation.Action > RegistryActionExit {
			return fmt.Errorf("operation %d: %w", index, ErrInvalidRegistryAction)
		}
		if operation.Address == (common.Address{}) {
			return fmt.Errorf("operation %d: %w", index, ErrInvalidRegistryAddress)
		}
		if operation.Sequence == 0 {
			return fmt.Errorf("operation %d: %w", index, ErrInvalidRegistrySequence)
		}
		if len(operation.Signature) != crypto.SignatureLength {
			return fmt.Errorf("operation %d: %w", index, ErrInvalidRegistrySignature)
		}
		if index > 0 && operations[index-1].Address == operation.Address && operations[index-1].Sequence == operation.Sequence {
			return fmt.Errorf("operation %d: %w", index, ErrDuplicateRegistryOperation)
		}
	}
	return nil
}

// EncodeRegistryHeaderExtra canonicalizes operations and encodes a bounded RLP
// envelope prefixed by a binary LQC magic. A fixed zeroed producer-seal suffix
// is reserved and replaced only after the final state root is known.
func EncodeRegistryHeaderExtra(blockNumber uint64, registryRoot common.Hash, operations []RegistryOperation) ([]byte, error) {
	if registryRoot == (common.Hash{}) {
		return nil, ErrInvalidRegistryRoot
	}
	canonical := CanonicalRegistryOperations(operations)
	if err := validateRegistryHeaderOperations(canonical); err != nil {
		return nil, err
	}
	envelope := RegistryHeaderEnvelope{
		Version:      RegistryHeaderEnvelopeVersion,
		BlockNumber:  blockNumber,
		RegistryRoot: registryRoot,
		Operations:   canonical,
	}
	payload, err := rlp.EncodeToBytes(envelope)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidRegistryHeaderExtra, err)
	}
	extra := make([]byte, 0, len(registryHeaderMagic)+len(payload))
	extra = append(extra, registryHeaderMagic...)
	extra = append(extra, payload...)
	extra = appendEmptyProducerSeal(extra)
	if len(extra) > MaxRegistryHeaderExtraSize {
		return nil, fmt.Errorf("%w: %d > %d", ErrInvalidRegistryHeaderExtra, len(extra), MaxRegistryHeaderExtraSize)
	}
	return extra, nil
}

// DecodeRegistryHeaderExtra rejects malformed, oversized, unsupported and
// non-canonical payloads. Cryptographic validation is available through
// ValidateRegistryHeaderExtra.
func DecodeRegistryHeaderExtra(extra []byte) (RegistryHeaderEnvelope, error) {
	if !IsRegistryHeaderExtra(extra) || len(extra) > MaxRegistryHeaderExtraSize {
		return RegistryHeaderEnvelope{}, ErrInvalidRegistryHeaderExtra
	}
	payloadExtra, _, err := splitProducerSeal(extra)
	if err != nil || !IsRegistryHeaderExtra(payloadExtra) {
		return RegistryHeaderEnvelope{}, ErrInvalidRegistryHeaderExtra
	}
	var envelope RegistryHeaderEnvelope
	if err := rlp.DecodeBytes(payloadExtra[len(registryHeaderMagic):], &envelope); err != nil {
		return RegistryHeaderEnvelope{}, fmt.Errorf("%w: %v", ErrInvalidRegistryHeaderExtra, err)
	}
	if envelope.Version != RegistryHeaderEnvelopeVersion {
		return RegistryHeaderEnvelope{}, ErrUnsupportedRegistryHeader
	}
	if envelope.RegistryRoot == (common.Hash{}) {
		return RegistryHeaderEnvelope{}, ErrInvalidRegistryRoot
	}
	if err := validateRegistryHeaderOperations(envelope.Operations); err != nil {
		return RegistryHeaderEnvelope{}, err
	}
	canonical := CanonicalRegistryOperations(envelope.Operations)
	if !registryOperationsEqual(canonical, envelope.Operations) {
		return RegistryHeaderEnvelope{}, ErrNonCanonicalRegistryOperations
	}
	reencoded, err := EncodeRegistryHeaderExtra(envelope.BlockNumber, envelope.RegistryRoot, envelope.Operations)
	if err != nil {
		return RegistryHeaderEnvelope{}, err
	}
	reencodedPayload, _, err := splitProducerSeal(reencoded)
	if err != nil || !bytes.Equal(reencodedPayload, payloadExtra) {
		return RegistryHeaderEnvelope{}, ErrInvalidRegistryHeaderExtra
	}
	return envelope, nil
}

// ValidateRegistryHeaderExtra performs the context-dependent operation checks.
// The stage-three snapshot layer will additionally recompute RegistryRoot.
func ValidateRegistryHeaderExtra(chainID *big.Int, blockNumber, proofDifficulty uint64, extra []byte) (RegistryHeaderEnvelope, error) {
	envelope, err := DecodeRegistryHeaderExtra(extra)
	if err != nil {
		return RegistryHeaderEnvelope{}, err
	}
	if envelope.BlockNumber != blockNumber {
		return RegistryHeaderEnvelope{}, ErrRegistryHeaderBlockMismatch
	}
	for index, operation := range envelope.Operations {
		if err := ValidateRegistryOperation(chainID, blockNumber, proofDifficulty, operation); err != nil {
			return RegistryHeaderEnvelope{}, fmt.Errorf("operation %d: %w", index, err)
		}
	}
	return envelope, nil
}

func registryOperationsEqual(left, right []RegistryOperation) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].Version != right[index].Version ||
			left[index].Action != right[index].Action ||
			left[index].Address != right[index].Address ||
			left[index].Sequence != right[index].Sequence ||
			left[index].ValidUntil != right[index].ValidUntil ||
			left[index].ProofNonce != right[index].ProofNonce ||
			!bytes.Equal(left[index].Signature, right[index].Signature) {
			return false
		}
	}
	return true
}
