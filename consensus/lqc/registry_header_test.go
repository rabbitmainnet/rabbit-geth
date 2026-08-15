package lqc

import (
	"bytes"
	"errors"
	"math/big"
	"reflect"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rlp"
)

func TestRegistryHeaderExtraRoundTripAndCanonicalOrder(t *testing.T) {
	chainID := big.NewInt(928)
	keyA := testRegistryKey(t, "0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b")
	keyB := testRegistryKey(t, "0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c")
	operationA := registerOperation(t, chainID, keyA, 1, 200, 1)
	operationB := registerOperation(t, chainID, keyB, 1, 200, 1)
	root := NewCanonicalRegistry().Root()

	extra, err := EncodeRegistryHeaderExtra(100, root, []RegistryOperation{operationB, operationA})
	if err != nil {
		t.Fatal(err)
	}
	if !IsRegistryHeaderExtra(extra) {
		t.Fatal("encoded header was not recognized")
	}
	envelope, err := ValidateRegistryHeaderExtra(chainID, 100, 1, extra)
	if err != nil {
		t.Fatal(err)
	}
	want := CanonicalRegistryOperations([]RegistryOperation{operationA, operationB})
	if envelope.Version != RegistryHeaderEnvelopeVersion || envelope.BlockNumber != 100 || envelope.RegistryRoot != root {
		t.Fatalf("unexpected envelope metadata: %+v", envelope)
	}
	if !reflect.DeepEqual(envelope.Operations, want) {
		t.Fatal("operations did not round-trip in canonical order")
	}
}

func TestRegistryHeaderEncodingIgnoresPoolArrivalOrder(t *testing.T) {
	chainID := big.NewInt(928)
	keyA := testRegistryKey(t, "0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d")
	keyB := testRegistryKey(t, "0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e")
	operationA := registerOperation(t, chainID, keyA, 1, 200, 1)
	operationB := registerOperation(t, chainID, keyB, 1, 200, 1)
	root := NewCanonicalRegistry().Root()

	left, err := EncodeRegistryHeaderExtra(100, root, []RegistryOperation{operationA, operationB})
	if err != nil {
		t.Fatal(err)
	}
	right, err := EncodeRegistryHeaderExtra(100, root, []RegistryOperation{operationB, operationA})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(left, right) {
		t.Fatal("header encoding depends on operation arrival order")
	}
}

func TestDecodeRegistryHeaderRejectsNonCanonicalOrder(t *testing.T) {
	chainID := big.NewInt(928)
	keyA := testRegistryKey(t, "0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f")
	keyB := testRegistryKey(t, "1010101010101010101010101010101010101010101010101010101010101010")
	operations := CanonicalRegistryOperations([]RegistryOperation{
		registerOperation(t, chainID, keyA, 1, 200, 1),
		registerOperation(t, chainID, keyB, 1, 200, 1),
	})
	operations[0], operations[1] = operations[1], operations[0]
	payload, err := rlp.EncodeToBytes(RegistryHeaderEnvelope{
		Version:      RegistryHeaderEnvelopeVersion,
		BlockNumber:  100,
		RegistryRoot: NewCanonicalRegistry().Root(),
		Operations:   operations,
	})
	if err != nil {
		t.Fatal(err)
	}
	extra := appendEmptyProducerSeal(append(bytes.Clone(registryHeaderMagic), payload...))
	if _, err := DecodeRegistryHeaderExtra(extra); !errors.Is(err, ErrNonCanonicalRegistryOperations) {
		t.Fatalf("error = %v, want %v", err, ErrNonCanonicalRegistryOperations)
	}
}

func TestRegistryHeaderRejectsDuplicateAndTooManyOperations(t *testing.T) {
	chainID := big.NewInt(928)
	key := testRegistryKey(t, "1111111111111111111111111111111111111111111111111111111111111111")
	operation := registerOperation(t, chainID, key, 1, 200, 1)
	root := NewCanonicalRegistry().Root()

	if _, err := EncodeRegistryHeaderExtra(100, root, []RegistryOperation{operation, operation}); !errors.Is(err, ErrDuplicateRegistryOperation) {
		t.Fatalf("duplicate error = %v, want %v", err, ErrDuplicateRegistryOperation)
	}
	tooMany := make([]RegistryOperation, MaxRegistryOperationsPerBlock+1)
	for index := range tooMany {
		tooMany[index] = operation
		tooMany[index].Address = common.BigToAddress(big.NewInt(int64(index + 1)))
	}
	if _, err := EncodeRegistryHeaderExtra(100, root, tooMany); !errors.Is(err, ErrTooManyRegistryOperations) {
		t.Fatalf("limit error = %v, want %v", err, ErrTooManyRegistryOperations)
	}
}

func TestRegistryHeaderRejectsInvalidMetadataAndSignature(t *testing.T) {
	chainID := big.NewInt(928)
	key := testRegistryKey(t, "1212121212121212121212121212121212121212121212121212121212121212")
	operation := registerOperation(t, chainID, key, 1, 200, 1)
	root := NewCanonicalRegistry().Root()

	if _, err := EncodeRegistryHeaderExtra(100, common.Hash{}, nil); !errors.Is(err, ErrInvalidRegistryRoot) {
		t.Fatalf("root error = %v, want %v", err, ErrInvalidRegistryRoot)
	}
	badSignature := operation
	badSignature.Signature = badSignature.Signature[:64]
	if _, err := EncodeRegistryHeaderExtra(100, root, []RegistryOperation{badSignature}); !errors.Is(err, ErrInvalidRegistrySignature) {
		t.Fatalf("signature length error = %v, want %v", err, ErrInvalidRegistrySignature)
	}
	extra, err := EncodeRegistryHeaderExtra(100, root, []RegistryOperation{operation})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateRegistryHeaderExtra(chainID, 101, 1, extra); !errors.Is(err, ErrRegistryHeaderBlockMismatch) {
		t.Fatalf("block error = %v, want %v", err, ErrRegistryHeaderBlockMismatch)
	}
	wrongAddress := operation
	wrongAddress.Address = common.BigToAddress(big.NewInt(999))
	extra, err = EncodeRegistryHeaderExtra(100, root, []RegistryOperation{wrongAddress})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateRegistryHeaderExtra(chainID, 100, 1, extra); !errors.Is(err, ErrInvalidRegistrySignature) {
		t.Fatalf("cryptographic signature error = %v, want %v", err, ErrInvalidRegistrySignature)
	}
}

func TestRegistryHeaderRejectsMalformedOversizedAndUnsupportedPayloads(t *testing.T) {
	oversized := make([]byte, MaxRegistryHeaderExtraSize+1)
	copy(oversized, registryHeaderMagic)
	if _, err := DecodeRegistryHeaderExtra(oversized); !errors.Is(err, ErrInvalidRegistryHeaderExtra) {
		t.Fatalf("oversized error = %v, want %v", err, ErrInvalidRegistryHeaderExtra)
	}
	if _, err := DecodeRegistryHeaderExtra([]byte("LQC:1:100")); !errors.Is(err, ErrInvalidRegistryHeaderExtra) {
		t.Fatalf("legacy error = %v, want %v", err, ErrInvalidRegistryHeaderExtra)
	}
	payload, err := rlp.EncodeToBytes(RegistryHeaderEnvelope{
		Version:      RegistryHeaderEnvelopeVersion + 1,
		BlockNumber:  100,
		RegistryRoot: NewCanonicalRegistry().Root(),
	})
	if err != nil {
		t.Fatal(err)
	}
	extra := appendEmptyProducerSeal(append(bytes.Clone(registryHeaderMagic), payload...))
	if _, err := DecodeRegistryHeaderExtra(extra); !errors.Is(err, ErrUnsupportedRegistryHeader) {
		t.Fatalf("version error = %v, want %v", err, ErrUnsupportedRegistryHeader)
	}
}
