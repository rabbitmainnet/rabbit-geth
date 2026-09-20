package rabbitvrf

import (
	"bytes"
	"math/big"
	"testing"

	bls12381 "github.com/consensys/gnark-crypto/ecc/bls12-381"
	"github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
)

func dkgEvaluationCommitmentForScalarV1(
	scalar fr.Element,
) DKGCoefficientCommitmentV1 {
	var point bls12381.G1Affine

	if scalar.IsZero() {
		point.SetInfinity()
	} else {
		scalarBig := new(big.Int)
		scalar.ToBigIntRegular(scalarBig)

		point.ScalarMultiplicationBase(
			scalarBig,
		)
	}

	encoded := point.Bytes()

	var out DKGCoefficientCommitmentV1
	copy(out[:], encoded[:])

	return out
}

func dkgEvaluatePolynomialForTestV1(
	coefficients []fr.Element,
	shareID uint64,
) fr.Element {
	var x fr.Element
	x.SetUint64(shareID)

	power := fr.One()

	var result fr.Element

	for index := range coefficients {
		var term fr.Element
		term.Mul(
			&coefficients[index],
			&power,
		)

		result.Add(
			&result,
			&term,
		)

		power.Mul(
			&power,
			&x,
		)
	}

	return result
}

func dkgEvaluationBytesForTestV1(
	scalar fr.Element,
) DKGPolynomialEvaluationV1 {
	encoded := scalar.Bytes()

	out, err :=
		DKGPolynomialEvaluationV1FromBytes(
			encoded[:],
		)
	if err != nil {
		panic(err)
	}

	return out
}

func TestDKGPolynomialEvaluationV1FromBytes(
	t *testing.T,
) {
	var scalar fr.Element
	scalar.SetUint64(12345)

	encoded := scalar.Bytes()

	evaluation, err :=
		DKGPolynomialEvaluationV1FromBytes(
			encoded[:],
		)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(
		evaluation[:],
		encoded[:],
	) {
		t.Fatal("canonical evaluation bytes changed")
	}

	zero := make(
		[]byte,
		DKGPolynomialEvaluationSizeV1,
	)

	if _, err :=
		DKGPolynomialEvaluationV1FromBytes(
			zero,
		); err != nil {
		t.Fatalf(
			"zero dealer evaluation rejected: %v",
			err,
		)
	}

	if _, err :=
		DKGPolynomialEvaluationV1FromBytes(
			encoded[:len(encoded)-1],
		); err == nil {
		t.Fatal("wrong evaluation length accepted")
	}

	nonCanonical := bytes.Repeat(
		[]byte{0xff},
		DKGPolynomialEvaluationSizeV1,
	)

	if _, err :=
		DKGPolynomialEvaluationV1FromBytes(
			nonCanonical,
		); err == nil {
		t.Fatal("non-canonical Fr scalar accepted")
	}
}

func TestVerifyDKGPolynomialEvaluationV1(
	t *testing.T,
) {
	coefficients := make(
		[]fr.Element,
		3,
	)
	coefficients[0].SetUint64(42)
	coefficients[1].SetUint64(7)
	coefficients[2].SetUint64(11)

	commitments := make(
		[]DKGCoefficientCommitmentV1,
		len(coefficients),
	)

	for index := range coefficients {
		commitments[index] =
			dkgEvaluationCommitmentForScalarV1(
				coefficients[index],
			)
	}

	for _, shareID := range []uint64{
		1,
		2,
		5,
		17,
	} {
		scalar :=
			dkgEvaluatePolynomialForTestV1(
				coefficients,
				shareID,
			)

		evaluation :=
			dkgEvaluationBytesForTestV1(
				scalar,
			)

		if err := VerifyDKGPolynomialEvaluationV1(
			shareID,
			evaluation,
			commitments,
		); err != nil {
			t.Fatalf(
				"shareID=%d err=%v",
				shareID,
				err,
			)
		}
	}
}

func TestVerifyDKGPolynomialEvaluationV1AllowsZero(
	t *testing.T,
) {
	coefficients := make(
		[]fr.Element,
		2,
	)

	coefficients[0].SetUint64(1)

	coefficients[1].SetUint64(1)
	coefficients[1].Neg(
		&coefficients[1],
	)

	commitments := []DKGCoefficientCommitmentV1{
		dkgEvaluationCommitmentForScalarV1(
			coefficients[0],
		),
		dkgEvaluationCommitmentForScalarV1(
			coefficients[1],
		),
	}

	scalar :=
		dkgEvaluatePolynomialForTestV1(
			coefficients,
			1,
		)

	if !scalar.IsZero() {
		t.Fatal("test polynomial did not evaluate to zero")
	}

	evaluation :=
		dkgEvaluationBytesForTestV1(
			scalar,
		)

	if err := VerifyDKGPolynomialEvaluationV1(
		1,
		evaluation,
		commitments,
	); err != nil {
		t.Fatalf(
			"valid zero dealer evaluation rejected: %v",
			err,
		)
	}
}

func TestVerifyDKGPolynomialEvaluationV1RejectsTampering(
	t *testing.T,
) {
	coefficients := make(
		[]fr.Element,
		3,
	)
	coefficients[0].SetUint64(42)
	coefficients[1].SetUint64(7)
	coefficients[2].SetUint64(11)

	commitments := make(
		[]DKGCoefficientCommitmentV1,
		len(coefficients),
	)

	for index := range coefficients {
		commitments[index] =
			dkgEvaluationCommitmentForScalarV1(
				coefficients[index],
			)
	}

	scalar :=
		dkgEvaluatePolynomialForTestV1(
			coefficients,
			5,
		)

	evaluation :=
		dkgEvaluationBytesForTestV1(
			scalar,
		)

	wrongScalar := scalar
	var one fr.Element
	one.SetUint64(1)
	wrongScalar.Add(
		&wrongScalar,
		&one,
	)

	wrongEvaluation :=
		dkgEvaluationBytesForTestV1(
			wrongScalar,
		)

	if err := VerifyDKGPolynomialEvaluationV1(
		5,
		wrongEvaluation,
		commitments,
	); err == nil {
		t.Fatal("altered private evaluation accepted")
	}

	alteredCommitments := append(
		[]DKGCoefficientCommitmentV1(nil),
		commitments...,
	)

	var alteredScalar fr.Element
	alteredScalar.SetUint64(99)

	alteredCommitments[1] =
		dkgEvaluationCommitmentForScalarV1(
			alteredScalar,
		)

	if err := VerifyDKGPolynomialEvaluationV1(
		5,
		evaluation,
		alteredCommitments,
	); err == nil {
		t.Fatal("altered public commitment accepted")
	}

	if err := VerifyDKGPolynomialEvaluationV1(
		6,
		evaluation,
		commitments,
	); err == nil {
		t.Fatal("wrong recipient ShareID accepted")
	}

	if err := VerifyDKGPolynomialEvaluationV1(
		0,
		evaluation,
		commitments,
	); err == nil {
		t.Fatal("zero recipient ShareID accepted")
	}
}
