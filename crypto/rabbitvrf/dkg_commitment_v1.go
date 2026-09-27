package rabbitvrf

import (
	"bytes"
	"errors"

	bls12381 "github.com/consensys/gnark-crypto/ecc/bls12-381"
)

var ErrInvalidDKGCoefficientCommitmentV1 = errors.New(
	"invalid Rabbit VRF DKG coefficient commitment v1",
)

// DKGCoefficientCommitmentV1 is one canonical compressed BLS12-381 G1 point.
//
// A zero polynomial coefficient is represented by the canonical compressed G1
// point at infinity. The DKG layer decides which polynomial positions may use
// infinity.
type DKGCoefficientCommitmentV1 [PublicKeySize]byte

// ValidateDKGCoefficientCommitmentV1 validates canonical compressed G1 bytes.
//
// When allowInfinity is false, the commitment must represent a non-zero group
// element. When true, the canonical point-at-infinity encoding is permitted.
func ValidateDKGCoefficientCommitmentV1(
	commitment DKGCoefficientCommitmentV1,
	allowInfinity bool,
) error {
	var point bls12381.G1Affine

	consumed, err := point.SetBytes(commitment[:])
	if err != nil ||
		consumed != PublicKeySize ||
		!point.IsOnCurve() ||
		!point.IsInSubGroup() {
		return ErrInvalidDKGCoefficientCommitmentV1
	}

	if point.IsInfinity() && !allowInfinity {
		return ErrInvalidDKGCoefficientCommitmentV1
	}

	canonical := point.Bytes()
	if !bytes.Equal(canonical[:], commitment[:]) {
		return ErrInvalidDKGCoefficientCommitmentV1
	}

	return nil
}

func AggregateDKGConstantCommitmentsV1(commitments []DKGCoefficientCommitmentV1) (PublicKey, error) {
	var out PublicKey
	if len(commitments) == 0 {
		return out, ErrInvalidDKGCoefficientCommitmentV1
	}
	var aggregate bls12381.G1Affine
	for index, commitment := range commitments {
		if err := ValidateDKGCoefficientCommitmentV1(commitment, false); err != nil {
			return out, err
		}
		var point bls12381.G1Affine
		consumed, err := point.SetBytes(commitment[:])
		if err != nil || consumed != PublicKeySize {
			return out, ErrInvalidDKGCoefficientCommitmentV1
		}
		if index == 0 {
			aggregate = point
		} else {
			var sum bls12381.G1Affine
			sum.Add(&aggregate, &point)
			aggregate = sum
		}
	}
	if aggregate.IsInfinity() || !aggregate.IsOnCurve() || !aggregate.IsInSubGroup() {
		return out, ErrInvalidDKGCoefficientCommitmentV1
	}
	encoded := aggregate.Bytes()
	copy(out[:], encoded[:])
	if _, err := decodePublicKey(out); err != nil {
		return PublicKey{}, err
	}
	return out, nil
}
