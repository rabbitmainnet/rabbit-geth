package rabbitvrfstate

import (
	"errors"
	"math/big"
	"testing"

	bls12381 "github.com/consensys/gnark-crypto/ecc/bls12-381"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	rabbitvrf "github.com/ethereum/go-ethereum/crypto/rabbitvrf"
)

func testFinalKeysetCoefficientV1(value uint64) rabbitvrf.DKGCoefficientCommitmentV1 {
	var point bls12381.G1Affine
	point.ScalarMultiplicationBase(new(big.Int).SetUint64(value))
	encoded := point.Bytes()
	var out rabbitvrf.DKGCoefficientCommitmentV1
	copy(out[:], encoded[:])
	return out
}

func TestDKGFinalKeysetStoreV1RestartDuplicateConflict(t *testing.T) {
	context, err := lqc.NewRabbitVRFDKGSessionContextV1(
		big.NewInt(9280),
		11,
		common.HexToHash("0xaaaa"),
		32,
	)
	if err != nil {
		t.Fatal(err)
	}

	commitments := make([]lqc.RabbitVRFDKGPolynomialCommitmentV1, context.CommitteeSize)
	for index := range commitments {
		coefficients := make([]rabbitvrf.DKGCoefficientCommitmentV1, context.Threshold)
		for coefficientIndex := range coefficients {
			coefficients[coefficientIndex] = testFinalKeysetCoefficientV1(uint64(coefficientIndex + 1))
		}
		coefficients[0] = testFinalKeysetCoefficientV1(uint64(index + 1))

		commitment, _, err := lqc.NewRabbitVRFDKGPolynomialCommitmentV1(
			context,
			uint64(index+1),
			coefficients,
		)
		if err != nil {
			t.Fatal(err)
		}
		commitments[index] = commitment
	}

	root, thresholdPublicKey, transcriptRoot, verificationShares, err :=
		lqc.RabbitVRFDKGFinalKeysetV1(context, commitments)
	if err != nil {
		t.Fatal(err)
	}

	value := DKGFinalKeysetV1{
		KeysetRoot:         root,
		ThresholdPublicKey: thresholdPublicKey,
		TranscriptRoot:     transcriptRoot,
		VerificationShares: verificationShares,
	}

	dir := t.TempDir()
	store, err := NewDKGFinalKeysetStoreV1(dir)
	if err != nil {
		t.Fatal(err)
	}

	if err := store.Store(context, value); err != nil {
		t.Fatal(err)
	}

	restarted, err := NewDKGFinalKeysetStoreV1(dir)
	if err != nil {
		t.Fatal(err)
	}

	loaded, err := restarted.Load(context)
	if err != nil {
		t.Fatal(err)
	}

	if loaded.KeysetRoot != value.KeysetRoot ||
		loaded.ThresholdPublicKey != value.ThresholdPublicKey ||
		loaded.TranscriptRoot != value.TranscriptRoot ||
		!equalDKGVerificationSharesV1(loaded.VerificationShares, value.VerificationShares) {
		t.Fatal("restart changed final keyset")
	}

	if err := restarted.Store(context, value); err != nil {
		t.Fatalf("duplicate rejected: %v", err)
	}

	conflict := value
	conflict.TranscriptRoot = common.HexToHash("0x1234")

	if err := restarted.Store(context, conflict); !errors.Is(err, ErrInvalidDKGFinalKeysetStoreV1) &&
		!errors.Is(err, ErrDKGFinalKeysetStoreConflictV1) {
		t.Fatalf("conflict error=%v", err)
	}
}
