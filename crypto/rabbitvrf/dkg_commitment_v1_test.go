package rabbitvrf

import (
	"math/big"
	"testing"

	bls12381 "github.com/consensys/gnark-crypto/ecc/bls12-381"
)

func dkgCoefficientCommitmentForTestV1(
	scalar uint64,
) DKGCoefficientCommitmentV1 {
	var point bls12381.G1Affine

	if scalar != 0 {
		point.ScalarMultiplicationBase(
			new(big.Int).SetUint64(scalar),
		)
	}

	encoded := point.Bytes()

	var out DKGCoefficientCommitmentV1
	copy(out[:], encoded[:])

	return out
}

func TestValidateDKGCoefficientCommitmentV1(
	t *testing.T,
) {
	nonZero := dkgCoefficientCommitmentForTestV1(7)

	if err := ValidateDKGCoefficientCommitmentV1(
		nonZero,
		false,
	); err != nil {
		t.Fatal(err)
	}

	infinity := dkgCoefficientCommitmentForTestV1(0)

	if err := ValidateDKGCoefficientCommitmentV1(
		infinity,
		true,
	); err != nil {
		t.Fatalf(
			"canonical infinity rejected when allowed: %v",
			err,
		)
	}

	if err := ValidateDKGCoefficientCommitmentV1(
		infinity,
		false,
	); err == nil {
		t.Fatal("infinity accepted when forbidden")
	}

	var malformed DKGCoefficientCommitmentV1

	if err := ValidateDKGCoefficientCommitmentV1(
		malformed,
		true,
	); err == nil {
		t.Fatal("malformed G1 encoding accepted")
	}
}
