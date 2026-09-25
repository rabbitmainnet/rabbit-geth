package rabbitvrf

import (
	"errors"
	"math/big"

	bls12381 "github.com/consensys/gnark-crypto/ecc/bls12-381"
	"github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
)

var ErrInvalidDKGDealerPolynomialV1 = errors.New(
	"invalid Rabbit VRF DKG dealer polynomial v1",
)

// DKGDealerPolynomialV1 owns one dealer's private degree threshold-1 polynomial.
//
// Coefficients are private BLS12-381 Fr scalars ordered a[0]..a[t-1].
//
// The constant and highest-degree coefficients are always non-zero so the
// resulting public commitment satisfies the Rabbit VRF DKG V1 rules.
// Intermediate coefficients may be zero.
type DKGDealerPolynomialV1 struct {
	coefficients []fr.Element
}

// GenerateDKGDealerPolynomialV1 creates a fresh cryptographically-random dealer
// polynomial with exactly threshold coefficients.
func GenerateDKGDealerPolynomialV1(
	threshold uint64,
) (*DKGDealerPolynomialV1, error) {
	if threshold == 0 {
		return nil, ErrInvalidDKGDealerPolynomialV1
	}

	maxInt := int(^uint(0) >> 1)
	if threshold > uint64(maxInt) {
		return nil, ErrInvalidDKGDealerPolynomialV1
	}

	coefficients := make([]fr.Element, int(threshold))

	for index := range coefficients {
		for {
			if _, err := coefficients[index].SetRandom(); err != nil {
				for clearIndex := range coefficients {
					coefficients[clearIndex] = fr.Element{}
				}
				return nil, err
			}

			endpoint := index == 0 || index == len(coefficients)-1
			if !endpoint || !coefficients[index].IsZero() {
				break
			}
		}
	}

	return &DKGDealerPolynomialV1{
		coefficients: coefficients,
	}, nil
}

// DKGDealerPolynomialV1FromBytes restores one canonical private polynomial.
//
// The encoding is exactly threshold concatenated canonical 32-byte Fr scalars.
func DKGDealerPolynomialV1FromBytes(
	encoded []byte,
	threshold uint64,
) (*DKGDealerPolynomialV1, error) {
	if threshold == 0 ||
		len(encoded) == 0 ||
		len(encoded)%SecretKeySize != 0 ||
		uint64(len(encoded)/SecretKeySize) != threshold {
		return nil, ErrInvalidDKGDealerPolynomialV1
	}

	coefficients := make([]fr.Element, int(threshold))

	for index := range coefficients {
		start := index * SecretKeySize
		end := start + SecretKeySize

		if err := coefficients[index].SetBytesCanonical(
			encoded[start:end],
		); err != nil {
			for clearIndex := range coefficients {
				coefficients[clearIndex] = fr.Element{}
			}
			return nil, ErrInvalidDKGDealerPolynomialV1
		}

		endpoint := index == 0 || index == len(coefficients)-1
		if endpoint && coefficients[index].IsZero() {
			for clearIndex := range coefficients {
				coefficients[clearIndex] = fr.Element{}
			}
			return nil, ErrInvalidDKGDealerPolynomialV1
		}
	}

	return &DKGDealerPolynomialV1{
		coefficients: coefficients,
	}, nil
}

// Bytes returns the exact canonical private polynomial encoding.
func (polynomial *DKGDealerPolynomialV1) Bytes() ([]byte, error) {
	if polynomial == nil || len(polynomial.coefficients) == 0 {
		return nil, ErrInvalidDKGDealerPolynomialV1
	}

	encoded := make(
		[]byte,
		len(polynomial.coefficients)*SecretKeySize,
	)

	for index := range polynomial.coefficients {
		endpoint := index == 0 ||
			index == len(polynomial.coefficients)-1

		if endpoint && polynomial.coefficients[index].IsZero() {
			return nil, ErrInvalidDKGDealerPolynomialV1
		}

		scalarBytes := polynomial.coefficients[index].Bytes()
		copy(
			encoded[index*SecretKeySize:(index+1)*SecretKeySize],
			scalarBytes[:],
		)
	}

	return encoded, nil
}

func (polynomial *DKGDealerPolynomialV1) Threshold() uint64 {
	if polynomial == nil {
		return 0
	}
	return uint64(len(polynomial.coefficients))
}

// Commitments derives the canonical public Feldman commitment C[j] = G1*a[j].
func (polynomial *DKGDealerPolynomialV1) Commitments() (
	[]DKGCoefficientCommitmentV1,
	error,
) {
	if polynomial == nil || len(polynomial.coefficients) == 0 {
		return nil, ErrInvalidDKGDealerPolynomialV1
	}

	commitments := make(
		[]DKGCoefficientCommitmentV1,
		len(polynomial.coefficients),
	)

	for index := range polynomial.coefficients {
		scalar := polynomial.coefficients[index]

		endpoint := index == 0 ||
			index == len(polynomial.coefficients)-1

		if endpoint && scalar.IsZero() {
			return nil, ErrInvalidDKGDealerPolynomialV1
		}

		var point bls12381.G1Affine

		if scalar.IsZero() {
			point.SetInfinity()
		} else {
			scalarBig := new(big.Int)
			scalar.ToBigIntRegular(scalarBig)
			point.ScalarMultiplicationBase(scalarBig)
		}

		pointBytes := point.Bytes()
		copy(commitments[index][:], pointBytes[:])

		if err := ValidateDKGCoefficientCommitmentV1(
			commitments[index],
			!endpoint,
		); err != nil {
			return nil, ErrInvalidDKGDealerPolynomialV1
		}
	}

	return commitments, nil
}

// Evaluate returns f(recipientShareID) using Horner-independent canonical Fr
// arithmetic. ShareID zero is invalid in the Rabbit VRF DKG.
func (polynomial *DKGDealerPolynomialV1) Evaluate(
	recipientShareID uint64,
) (
	DKGPolynomialEvaluationV1,
	error,
) {
	var empty DKGPolynomialEvaluationV1

	if polynomial == nil ||
		len(polynomial.coefficients) == 0 ||
		recipientShareID == 0 {
		return empty, ErrInvalidDKGDealerPolynomialV1
	}

	var x fr.Element
	x.SetUint64(recipientShareID)

	power := fr.One()
	var result fr.Element

	for index := range polynomial.coefficients {
		var term fr.Element
		term.Mul(
			&polynomial.coefficients[index],
			&power,
		)
		result.Add(&result, &term)
		power.Mul(&power, &x)
	}

	encoded := result.Bytes()

	evaluation, err := DKGPolynomialEvaluationV1FromBytes(
		encoded[:],
	)
	if err != nil {
		return empty, ErrInvalidDKGDealerPolynomialV1
	}

	return evaluation, nil
}

// Destroy clears the in-memory coefficient slice and releases it.
//
// Callers must still clear any serialized copies returned by Bytes separately.
func (polynomial *DKGDealerPolynomialV1) Destroy() {
	if polynomial == nil {
		return
	}

	for index := range polynomial.coefficients {
		polynomial.coefficients[index] = fr.Element{}
	}
	polynomial.coefficients = nil
}
