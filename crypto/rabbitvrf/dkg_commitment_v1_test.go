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

func TestAggregateDKGConstantCommitmentsV1(t *testing.T) {
	makeCommitment := func(value byte) DKGCoefficientCommitmentV1 {
		encoded := make([]byte, SecretKeySize)
		encoded[len(encoded)-1] = value
		secret, err := SecretKeyFromBytes(encoded)
		if err != nil {
			t.Fatal(err)
		}
		publicKey, err := secret.PublicKey()
		if err != nil {
			t.Fatal(err)
		}
		var commitment DKGCoefficientCommitmentV1
		copy(commitment[:], publicKey[:])
		return commitment
	}

	aggregated, err := AggregateDKGConstantCommitmentsV1([]DKGCoefficientCommitmentV1{makeCommitment(2), makeCommitment(3), makeCommitment(5)})
	if err != nil {
		t.Fatal(err)
	}

	expectedBytes := make([]byte, SecretKeySize)
	expectedBytes[len(expectedBytes)-1] = 10
	expectedSecret, err := SecretKeyFromBytes(expectedBytes)
	if err != nil {
		t.Fatal(err)
	}
	expectedPublic, err := expectedSecret.PublicKey()
	if err != nil {
		t.Fatal(err)
	}

	if aggregated != expectedPublic {
		t.Fatal("aggregated constant commitments produced wrong threshold public key")
	}

	if _, err := AggregateDKGConstantCommitmentsV1(nil); err == nil {
		t.Fatal("empty constant commitment set accepted")
	}
}
