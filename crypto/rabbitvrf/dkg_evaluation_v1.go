package rabbitvrf

import (
	"bytes"
	"errors"
	"math/big"

	bls12381 "github.com/consensys/gnark-crypto/ecc/bls12-381"
	"github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
)

const DKGPolynomialEvaluationSizeV1 = SecretKeySize

var ErrInvalidDKGPolynomialEvaluationV1 = errors.New(
	"invalid Rabbit VRF DKG polynomial evaluation v1",
)

// DKGPolynomialEvaluationV1 is one dealer's private scalar contribution
// f(recipientShareID).
//
// Unlike a final aggregated SecretShare, an individual dealer evaluation may
// legitimately be zero.
type DKGPolynomialEvaluationV1 [DKGPolynomialEvaluationSizeV1]byte

func DKGPolynomialEvaluationV1FromBytes(
	encoded []byte,
) (
	DKGPolynomialEvaluationV1,
	error,
) {
	var out DKGPolynomialEvaluationV1

	if len(encoded) != DKGPolynomialEvaluationSizeV1 {
		return out, ErrInvalidDKGPolynomialEvaluationV1
	}

	var scalar fr.Element
	if err := scalar.SetBytesCanonical(encoded); err != nil {
		return out, ErrInvalidDKGPolynomialEvaluationV1
	}

	canonical := scalar.Bytes()
	if !bytes.Equal(canonical[:], encoded) {
		return out, ErrInvalidDKGPolynomialEvaluationV1
	}

	copy(out[:], encoded)

	return out, nil
}

func decodeDKGCoefficientCommitmentV1(
	commitment DKGCoefficientCommitmentV1,
) (
	bls12381.G1Affine,
	error,
) {
	var point bls12381.G1Affine

	consumed, err := point.SetBytes(commitment[:])
	if err != nil ||
		consumed != PublicKeySize ||
		!point.IsOnCurve() ||
		!point.IsInSubGroup() {
		return bls12381.G1Affine{},
			ErrInvalidDKGCoefficientCommitmentV1
	}

	canonical := point.Bytes()
	if !bytes.Equal(canonical[:], commitment[:]) {
		return bls12381.G1Affine{},
			ErrInvalidDKGCoefficientCommitmentV1
	}

	return point, nil
}

// VerifyDKGPolynomialEvaluationV1 verifies:
//
//	G1 * evaluation
//	    ==
//	C[0] + x*C[1] + x^2*C[2] + ... + x^(n-1)*C[n-1]
//
// where x is the immutable non-zero recipient ShareID.
//
// Committee bounds and DKG session binding belong to the consensus wrapper.
func VerifyDKGPolynomialEvaluationV1(
	recipientShareID uint64,
	evaluation DKGPolynomialEvaluationV1,
	coefficients []DKGCoefficientCommitmentV1,
) error {
	if recipientShareID == 0 ||
		len(coefficients) == 0 {
		return ErrInvalidDKGPolynomialEvaluationV1
	}

	var evaluationScalar fr.Element
	if err := evaluationScalar.SetBytesCanonical(
		evaluation[:],
	); err != nil {
		return ErrInvalidDKGPolynomialEvaluationV1
	}

	evaluationCanonical := evaluationScalar.Bytes()
	if !bytes.Equal(
		evaluationCanonical[:],
		evaluation[:],
	) {
		return ErrInvalidDKGPolynomialEvaluationV1
	}

	var lhs bls12381.G1Affine

	if evaluationScalar.IsZero() {
		lhs.SetInfinity()
	} else {
		evaluationBig := new(big.Int)
		evaluationScalar.ToBigIntRegular(
			evaluationBig,
		)
		lhs.ScalarMultiplicationBase(
			evaluationBig,
		)
	}

	var x fr.Element
	x.SetUint64(recipientShareID)

	power := fr.One()

	var rhs bls12381.G1Affine
	rhs.SetInfinity()

	for _, encodedCommitment := range coefficients {
		commitment, err :=
			decodeDKGCoefficientCommitmentV1(
				encodedCommitment,
			)
		if err != nil {
			return ErrInvalidDKGPolynomialEvaluationV1
		}

		if !commitment.IsInfinity() {
			powerBig := new(big.Int)
			power.ToBigIntRegular(powerBig)

			var term bls12381.G1Affine
			term.ScalarMultiplication(
				&commitment,
				powerBig,
			)

			var sum bls12381.G1Affine
			sum.Add(&rhs, &term)
			rhs = sum
		}

		power.Mul(&power, &x)
	}

	lhsBytes := lhs.Bytes()
	rhsBytes := rhs.Bytes()

	if !bytes.Equal(lhsBytes[:], rhsBytes[:]) {
		return ErrInvalidDKGPolynomialEvaluationV1
	}

	return nil
}

func AggregateDKGPolynomialEvaluationsV1(recipientShareID uint64, evaluations []DKGPolynomialEvaluationV1) (*SecretShare, error) {
	if recipientShareID == 0 || len(evaluations) == 0 {
		return nil, ErrInvalidSecretShare
	}
	var aggregate fr.Element
	for _, evaluation := range evaluations {
		var scalar fr.Element
		if err := scalar.SetBytesCanonical(evaluation[:]); err != nil {
			return nil, ErrInvalidDKGPolynomialEvaluationV1
		}
		aggregate.Add(&aggregate, &scalar)
	}
	return newSecretShare(recipientShareID, aggregate)
}

func DKGVerificationPublicKeyV1(recipientShareID uint64, dealerCommitments [][]DKGCoefficientCommitmentV1) (PublicKey, error) {
	var out PublicKey
	if recipientShareID == 0 || len(dealerCommitments) == 0 {
		return out, ErrInvalidDKGPolynomialEvaluationV1
	}
	var x fr.Element
	x.SetUint64(recipientShareID)
	var aggregate bls12381.G1Affine
	aggregate.SetInfinity()
	for _, coefficients := range dealerCommitments {
		if len(coefficients) == 0 {
			return out, ErrInvalidDKGPolynomialEvaluationV1
		}
		power := fr.One()
		var dealerPoint bls12381.G1Affine
		dealerPoint.SetInfinity()
		for _, encodedCommitment := range coefficients {
			commitment, err := decodeDKGCoefficientCommitmentV1(encodedCommitment)
			if err != nil {
				return out, ErrInvalidDKGPolynomialEvaluationV1
			}
			if !commitment.IsInfinity() {
				powerBig := new(big.Int)
				power.ToBigIntRegular(powerBig)
				var term bls12381.G1Affine
				term.ScalarMultiplication(&commitment, powerBig)
				var sum bls12381.G1Affine
				sum.Add(&dealerPoint, &term)
				dealerPoint = sum
			}
			power.Mul(&power, &x)
		}
		var sum bls12381.G1Affine
		sum.Add(&aggregate, &dealerPoint)
		aggregate = sum
	}
	if aggregate.IsInfinity() || !aggregate.IsOnCurve() || !aggregate.IsInSubGroup() {
		return out, ErrInvalidPublicKey
	}
	encoded := aggregate.Bytes()
	copy(out[:], encoded[:])
	if _, err := decodePublicKey(out); err != nil {
		return PublicKey{}, err
	}
	return out, nil
}
