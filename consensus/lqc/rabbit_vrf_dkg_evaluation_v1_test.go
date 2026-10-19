package lqc

import (
	"testing"

	"github.com/consensys/gnark-crypto/ecc/bls12-381/fr"

	rabbitvrf "github.com/ethereum/go-ethereum/crypto/rabbitvrf"
)

func rabbitVRFDKGEvaluationForTestV1(
	threshold uint64,
	recipientShareID uint64,
) rabbitvrf.DKGPolynomialEvaluationV1 {
	coefficients := make(
		[]fr.Element,
		threshold,
	)

	for index := range coefficients {
		coefficients[index].SetUint64(
			uint64(index + 1),
		)
	}

	var x fr.Element
	x.SetUint64(recipientShareID)

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

	encoded := result.Bytes()

	evaluation, err :=
		rabbitvrf.DKGPolynomialEvaluationV1FromBytes(
			encoded[:],
		)
	if err != nil {
		panic(err)
	}

	return evaluation
}

func TestRabbitVRFDKGPrivateEvaluationV1(
	t *testing.T,
) {
	context := rabbitVRFDKGTestSessionContextV1(
		t,
		9280,
		11,
	)

	coefficients :=
		rabbitVRFDKGTestCommitmentCoefficientsV1(
			context.Threshold,
		)

	commitment, _, err :=
		NewRabbitVRFDKGPolynomialCommitmentV1(
			context,
			7,
			coefficients,
		)
	if err != nil {
		t.Fatal(err)
	}

	for _, recipientShareID := range []uint64{
		1,
		2,
		7,
		19,
		32,
	} {
		evaluation :=
			rabbitVRFDKGEvaluationForTestV1(
				context.Threshold,
				recipientShareID,
			)

		if err :=
			VerifyRabbitVRFDKGPrivateEvaluationV1(
				context,
				commitment,
				recipientShareID,
				evaluation,
			); err != nil {
			t.Fatalf(
				"recipient=%d err=%v",
				recipientShareID,
				err,
			)
		}
	}
}

func TestRabbitVRFDKGPrivateEvaluationV1RejectsRecipientBounds(
	t *testing.T,
) {
	context := rabbitVRFDKGTestSessionContextV1(
		t,
		9280,
		11,
	)

	coefficients :=
		rabbitVRFDKGTestCommitmentCoefficientsV1(
			context.Threshold,
		)

	commitment, _, err :=
		NewRabbitVRFDKGPolynomialCommitmentV1(
			context,
			7,
			coefficients,
		)
	if err != nil {
		t.Fatal(err)
	}

	evaluation :=
		rabbitVRFDKGEvaluationForTestV1(
			context.Threshold,
			1,
		)

	for _, recipientShareID := range []uint64{
		0,
		context.CommitteeSize + 1,
	} {
		if err :=
			VerifyRabbitVRFDKGPrivateEvaluationV1(
				context,
				commitment,
				recipientShareID,
				evaluation,
			); err == nil {
			t.Fatalf(
				"invalid recipient %d accepted",
				recipientShareID,
			)
		}
	}
}

func TestRabbitVRFDKGPrivateEvaluationV1RejectsWrongSession(
	t *testing.T,
) {
	baseContext :=
		rabbitVRFDKGTestSessionContextV1(
			t,
			9280,
			11,
		)

	coefficients :=
		rabbitVRFDKGTestCommitmentCoefficientsV1(
			baseContext.Threshold,
		)

	commitment, _, err :=
		NewRabbitVRFDKGPolynomialCommitmentV1(
			baseContext,
			7,
			coefficients,
		)
	if err != nil {
		t.Fatal(err)
	}

	evaluation :=
		rabbitVRFDKGEvaluationForTestV1(
			baseContext.Threshold,
			5,
		)

	otherContext :=
		rabbitVRFDKGTestSessionContextV1(
			t,
			9280,
			12,
		)

	if err :=
		VerifyRabbitVRFDKGPrivateEvaluationV1(
			otherContext,
			commitment,
			5,
			evaluation,
		); err == nil {
		t.Fatal("cross-session private evaluation accepted")
	}
}

func TestRabbitVRFDKGPrivateEvaluationV1RejectsTampering(
	t *testing.T,
) {
	context :=
		rabbitVRFDKGTestSessionContextV1(
			t,
			9280,
			11,
		)

	coefficients :=
		rabbitVRFDKGTestCommitmentCoefficientsV1(
			context.Threshold,
		)

	commitment, _, err :=
		NewRabbitVRFDKGPolynomialCommitmentV1(
			context,
			7,
			coefficients,
		)
	if err != nil {
		t.Fatal(err)
	}

	evaluation :=
		rabbitVRFDKGEvaluationForTestV1(
			context.Threshold,
			5,
		)

	alteredCommitment := commitment
	alteredCommitment.Coefficients = append(
		[]rabbitvrf.DKGCoefficientCommitmentV1(nil),
		commitment.Coefficients...,
	)
	alteredCommitment.Coefficients[1] =
		rabbitVRFDKGTestCoefficientV1(999)

	if err :=
		VerifyRabbitVRFDKGPrivateEvaluationV1(
			context,
			alteredCommitment,
			5,
			evaluation,
		); err == nil {
		t.Fatal("altered dealer commitment accepted")
	}

	var scalar fr.Element
	if err := scalar.SetBytesCanonical(
		evaluation[:],
	); err != nil {
		t.Fatal(err)
	}

	var one fr.Element
	one.SetUint64(1)
	scalar.Add(
		&scalar,
		&one,
	)

	alteredBytes := scalar.Bytes()

	alteredEvaluation, err :=
		rabbitvrf.DKGPolynomialEvaluationV1FromBytes(
			alteredBytes[:],
		)
	if err != nil {
		t.Fatal(err)
	}

	if err :=
		VerifyRabbitVRFDKGPrivateEvaluationV1(
			context,
			commitment,
			5,
			alteredEvaluation,
		); err == nil {
		t.Fatal("altered private evaluation accepted")
	}
}
