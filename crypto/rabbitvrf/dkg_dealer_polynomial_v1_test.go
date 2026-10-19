package rabbitvrf

import (
	"bytes"
	"testing"
)

func TestDKGDealerPolynomialV1RoundTrip(
	t *testing.T,
) {
	polynomial, err := GenerateDKGDealerPolynomialV1(4)
	if err != nil {
		t.Fatal(err)
	}
	defer polynomial.Destroy()

	encoded, err := polynomial.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		for index := range encoded {
			encoded[index] = 0
		}
	}()

	restored, err := DKGDealerPolynomialV1FromBytes(
		encoded,
		4,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Destroy()

	restoredBytes, err := restored.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		for index := range restoredBytes {
			restoredBytes[index] = 0
		}
	}()

	if !bytes.Equal(encoded, restoredBytes) {
		t.Fatal("restored polynomial bytes changed")
	}

	commitments, err := polynomial.Commitments()
	if err != nil {
		t.Fatal(err)
	}

	restoredCommitments, err := restored.Commitments()
	if err != nil {
		t.Fatal(err)
	}

	if len(commitments) != len(restoredCommitments) {
		t.Fatal("restored commitment count changed")
	}

	for index := range commitments {
		if commitments[index] != restoredCommitments[index] {
			t.Fatal("restored commitment changed")
		}
	}

	for _, shareID := range []uint64{1, 2, 5, 17} {
		evaluation, err := polynomial.Evaluate(shareID)
		if err != nil {
			t.Fatal(err)
		}

		restoredEvaluation, err := restored.Evaluate(shareID)
		if err != nil {
			t.Fatal(err)
		}

		if evaluation != restoredEvaluation {
			t.Fatal("restored evaluation changed")
		}

		if err := VerifyDKGPolynomialEvaluationV1(
			shareID,
			evaluation,
			commitments,
		); err != nil {
			t.Fatalf(
				"evaluation verification failed for share %d: %v",
				shareID,
				err,
			)
		}
	}
}

func TestDKGDealerPolynomialV1RejectsInvalidEncoding(
	t *testing.T,
) {
	polynomial, err := GenerateDKGDealerPolynomialV1(3)
	if err != nil {
		t.Fatal(err)
	}
	defer polynomial.Destroy()

	encoded, err := polynomial.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		for index := range encoded {
			encoded[index] = 0
		}
	}()

	if _, err := DKGDealerPolynomialV1FromBytes(
		encoded,
		2,
	); err == nil {
		t.Fatal("wrong threshold accepted")
	}

	zeroConstant := append([]byte(nil), encoded...)
	for index := 0; index < SecretKeySize; index++ {
		zeroConstant[index] = 0
	}

	if _, err := DKGDealerPolynomialV1FromBytes(
		zeroConstant,
		3,
	); err == nil {
		t.Fatal("zero constant coefficient accepted")
	}

	zeroHighest := append([]byte(nil), encoded...)
	start := 2 * SecretKeySize
	for index := start; index < start+SecretKeySize; index++ {
		zeroHighest[index] = 0
	}

	if _, err := DKGDealerPolynomialV1FromBytes(
		zeroHighest,
		3,
	); err == nil {
		t.Fatal("zero highest-degree coefficient accepted")
	}

	if _, err := polynomial.Evaluate(0); err == nil {
		t.Fatal("zero recipient share id accepted")
	}
}

func TestDKGDealerPolynomialV1Destroy(
	t *testing.T,
) {
	polynomial, err := GenerateDKGDealerPolynomialV1(3)
	if err != nil {
		t.Fatal(err)
	}

	polynomial.Destroy()

	if polynomial.Threshold() != 0 {
		t.Fatal("destroyed polynomial retained threshold")
	}

	if _, err := polynomial.Bytes(); err == nil {
		t.Fatal("destroyed polynomial remained usable")
	}

	if _, err := polynomial.Commitments(); err == nil {
		t.Fatal("destroyed polynomial commitments remained usable")
	}

	if _, err := polynomial.Evaluate(1); err == nil {
		t.Fatal("destroyed polynomial evaluation remained usable")
	}
}
