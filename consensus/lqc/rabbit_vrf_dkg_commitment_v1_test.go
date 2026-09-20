package lqc

import (
	"math/big"
	"testing"

	bls12381 "github.com/consensys/gnark-crypto/ecc/bls12-381"

	"github.com/ethereum/go-ethereum/common"
	rabbitvrf "github.com/ethereum/go-ethereum/crypto/rabbitvrf"
)

func rabbitVRFDKGTestCoefficientV1(
	scalar uint64,
) rabbitvrf.DKGCoefficientCommitmentV1 {
	var point bls12381.G1Affine

	if scalar != 0 {
		point.ScalarMultiplicationBase(
			new(big.Int).SetUint64(scalar),
		)
	}

	encoded := point.Bytes()

	var out rabbitvrf.DKGCoefficientCommitmentV1
	copy(out[:], encoded[:])

	return out
}

func rabbitVRFDKGTestCommitmentCoefficientsV1(
	threshold uint64,
) []rabbitvrf.DKGCoefficientCommitmentV1 {
	out := make(
		[]rabbitvrf.DKGCoefficientCommitmentV1,
		threshold,
	)

	for index := range out {
		out[index] = rabbitVRFDKGTestCoefficientV1(
			uint64(index + 1),
		)
	}

	return out
}

func rabbitVRFDKGTestSessionContextV1(
	t *testing.T,
	chainID int64,
	epoch uint64,
) RabbitVRFDKGSessionContextV1 {
	t.Helper()

	context, err := NewRabbitVRFDKGSessionContextV1(
		big.NewInt(chainID),
		epoch,
		common.HexToHash("0xaaaa"),
		32,
	)
	if err != nil {
		t.Fatal(err)
	}

	return context
}

func TestRabbitVRFDKGPolynomialCommitmentV1Deterministic(
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

	first, firstRoot, err :=
		NewRabbitVRFDKGPolynomialCommitmentV1(
			context,
			7,
			coefficients,
		)
	if err != nil {
		t.Fatal(err)
	}

	second, secondRoot, err :=
		NewRabbitVRFDKGPolynomialCommitmentV1(
			context,
			7,
			coefficients,
		)
	if err != nil {
		t.Fatal(err)
	}

	if firstRoot != secondRoot {
		t.Fatal("same polynomial commitment produced different root")
	}

	if first.SessionID != second.SessionID ||
		first.DealerShareID != 7 ||
		len(first.Coefficients) != int(context.Threshold) {
		t.Fatal("deterministic commitment metadata mismatch")
	}

	validatedRoot, err :=
		ValidateRabbitVRFDKGPolynomialCommitmentV1(
			context,
			first,
		)
	if err != nil {
		t.Fatal(err)
	}

	if validatedRoot != firstRoot {
		t.Fatal("validated root differs from constructed root")
	}
}

func TestRabbitVRFDKGPolynomialCommitmentV1BindsOrderAndDealer(
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

	_, baseRoot, err :=
		NewRabbitVRFDKGPolynomialCommitmentV1(
			context,
			7,
			coefficients,
		)
	if err != nil {
		t.Fatal(err)
	}

	reordered := append(
		[]rabbitvrf.DKGCoefficientCommitmentV1(nil),
		coefficients...,
	)
	reordered[1], reordered[2] =
		reordered[2], reordered[1]

	_, reorderedRoot, err :=
		NewRabbitVRFDKGPolynomialCommitmentV1(
			context,
			7,
			reordered,
		)
	if err != nil {
		t.Fatal(err)
	}

	if reorderedRoot == baseRoot {
		t.Fatal("coefficient reordering did not change root")
	}

	_, otherDealerRoot, err :=
		NewRabbitVRFDKGPolynomialCommitmentV1(
			context,
			8,
			coefficients,
		)
	if err != nil {
		t.Fatal(err)
	}

	if otherDealerRoot == baseRoot {
		t.Fatal("dealer ShareID did not bind commitment root")
	}
}

func TestRabbitVRFDKGPolynomialCommitmentV1BindsSession(
	t *testing.T,
) {
	baseContext := rabbitVRFDKGTestSessionContextV1(
		t,
		9280,
		11,
	)

	coefficients :=
		rabbitVRFDKGTestCommitmentCoefficientsV1(
			baseContext.Threshold,
		)

	_, baseRoot, err :=
		NewRabbitVRFDKGPolynomialCommitmentV1(
			baseContext,
			7,
			coefficients,
		)
	if err != nil {
		t.Fatal(err)
	}

	otherChain := rabbitVRFDKGTestSessionContextV1(
		t,
		928,
		11,
	)

	_, otherChainRoot, err :=
		NewRabbitVRFDKGPolynomialCommitmentV1(
			otherChain,
			7,
			coefficients,
		)
	if err != nil {
		t.Fatal(err)
	}

	if otherChainRoot == baseRoot {
		t.Fatal("cross-chain context produced same commitment root")
	}

	otherEpoch := rabbitVRFDKGTestSessionContextV1(
		t,
		9280,
		12,
	)

	_, otherEpochRoot, err :=
		NewRabbitVRFDKGPolynomialCommitmentV1(
			otherEpoch,
			7,
			coefficients,
		)
	if err != nil {
		t.Fatal(err)
	}

	if otherEpochRoot == baseRoot {
		t.Fatal("cross-epoch context produced same commitment root")
	}
}

func TestRabbitVRFDKGPolynomialCommitmentV1AllowsMiddleZero(
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

	coefficients[5] =
		rabbitVRFDKGTestCoefficientV1(0)

	if _, _, err :=
		NewRabbitVRFDKGPolynomialCommitmentV1(
			context,
			7,
			coefficients,
		); err != nil {
		t.Fatalf(
			"valid zero intermediate coefficient rejected: %v",
			err,
		)
	}
}

func TestRabbitVRFDKGPolynomialCommitmentV1RejectsInvalidShape(
	t *testing.T,
) {
	context := rabbitVRFDKGTestSessionContextV1(
		t,
		9280,
		11,
	)

	valid :=
		rabbitVRFDKGTestCommitmentCoefficientsV1(
			context.Threshold,
		)

	short := append(
		[]rabbitvrf.DKGCoefficientCommitmentV1(nil),
		valid[:len(valid)-1]...,
	)

	if _, _, err :=
		NewRabbitVRFDKGPolynomialCommitmentV1(
			context,
			7,
			short,
		); err == nil {
		t.Fatal("wrong coefficient count accepted")
	}

	for _, dealer := range []uint64{
		0,
		context.CommitteeSize + 1,
	} {
		if _, _, err :=
			NewRabbitVRFDKGPolynomialCommitmentV1(
				context,
				dealer,
				valid,
			); err == nil {
			t.Fatalf(
				"invalid dealer ShareID %d accepted",
				dealer,
			)
		}
	}

	zeroConstant := append(
		[]rabbitvrf.DKGCoefficientCommitmentV1(nil),
		valid...,
	)
	zeroConstant[0] =
		rabbitVRFDKGTestCoefficientV1(0)

	if _, _, err :=
		NewRabbitVRFDKGPolynomialCommitmentV1(
			context,
			7,
			zeroConstant,
		); err == nil {
		t.Fatal("zero constant coefficient accepted")
	}

	zeroLeading := append(
		[]rabbitvrf.DKGCoefficientCommitmentV1(nil),
		valid...,
	)
	zeroLeading[len(zeroLeading)-1] =
		rabbitVRFDKGTestCoefficientV1(0)

	if _, _, err :=
		NewRabbitVRFDKGPolynomialCommitmentV1(
			context,
			7,
			zeroLeading,
		); err == nil {
		t.Fatal("zero highest-degree coefficient accepted")
	}

	malformed := append(
		[]rabbitvrf.DKGCoefficientCommitmentV1(nil),
		valid...,
	)
	malformed[3] =
		rabbitvrf.DKGCoefficientCommitmentV1{}

	if _, _, err :=
		NewRabbitVRFDKGPolynomialCommitmentV1(
			context,
			7,
			malformed,
		); err == nil {
		t.Fatal("malformed coefficient commitment accepted")
	}
}

func TestRabbitVRFDKGPolynomialCommitmentsV1CanonicalOrder(
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

	makeCommitment := func(
		dealer uint64,
	) RabbitVRFDKGPolynomialCommitmentV1 {
		commitment, _, err :=
			NewRabbitVRFDKGPolynomialCommitmentV1(
				context,
				dealer,
				coefficients,
			)
		if err != nil {
			t.Fatal(err)
		}

		return commitment
	}

	input := []RabbitVRFDKGPolynomialCommitmentV1{
		makeCommitment(7),
		makeCommitment(2),
		makeCommitment(5),
	}

	canonical, roots, err :=
		CanonicalRabbitVRFDKGPolynomialCommitmentsV1(
			context,
			input,
		)
	if err != nil {
		t.Fatal(err)
	}

	if len(canonical) != 3 ||
		len(roots) != 3 {
		t.Fatal("canonical commitment collection length mismatch")
	}

	want := []uint64{2, 5, 7}

	for index, dealer := range want {
		if canonical[index].DealerShareID != dealer {
			t.Fatalf(
				"canonical[%d].dealer=%d want=%d",
				index,
				canonical[index].DealerShareID,
				dealer,
			)
		}

		root, err :=
			ValidateRabbitVRFDKGPolynomialCommitmentV1(
				context,
				canonical[index],
			)
		if err != nil {
			t.Fatal(err)
		}

		if roots[index] != root {
			t.Fatalf(
				"canonical root mismatch at index %d",
				index,
			)
		}
	}

	input[0].Coefficients[0] =
		rabbitVRFDKGTestCoefficientV1(99)

	if canonical[2].Coefficients[0] ==
		input[0].Coefficients[0] {
		t.Fatal(
			"canonical commitment aliases mutable input coefficients",
		)
	}
}

func TestRabbitVRFDKGPolynomialCommitmentsV1ArrivalOrderIndependent(
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

	makeCommitment := func(
		dealer uint64,
	) RabbitVRFDKGPolynomialCommitmentV1 {
		commitment, _, err :=
			NewRabbitVRFDKGPolynomialCommitmentV1(
				context,
				dealer,
				coefficients,
			)
		if err != nil {
			t.Fatal(err)
		}

		return commitment
	}

	firstInput := []RabbitVRFDKGPolynomialCommitmentV1{
		makeCommitment(9),
		makeCommitment(3),
		makeCommitment(6),
	}

	secondInput := []RabbitVRFDKGPolynomialCommitmentV1{
		makeCommitment(6),
		makeCommitment(9),
		makeCommitment(3),
	}

	first, firstRoots, err :=
		CanonicalRabbitVRFDKGPolynomialCommitmentsV1(
			context,
			firstInput,
		)
	if err != nil {
		t.Fatal(err)
	}

	second, secondRoots, err :=
		CanonicalRabbitVRFDKGPolynomialCommitmentsV1(
			context,
			secondInput,
		)
	if err != nil {
		t.Fatal(err)
	}

	if len(first) != len(second) ||
		len(firstRoots) != len(secondRoots) {
		t.Fatal("canonical collection lengths differ")
	}

	for index := range first {
		if first[index].DealerShareID !=
			second[index].DealerShareID {
			t.Fatal(
				"network arrival order changed canonical dealer order",
			)
		}

		if firstRoots[index] != secondRoots[index] {
			t.Fatal(
				"network arrival order changed canonical commitment roots",
			)
		}
	}
}

func TestRabbitVRFDKGPolynomialCommitmentsV1RejectsDuplicateDealer(
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

	first, _, err :=
		NewRabbitVRFDKGPolynomialCommitmentV1(
			context,
			7,
			coefficients,
		)
	if err != nil {
		t.Fatal(err)
	}

	secondCoefficients := append(
		[]rabbitvrf.DKGCoefficientCommitmentV1(nil),
		coefficients...,
	)
	secondCoefficients[1] =
		rabbitVRFDKGTestCoefficientV1(777)

	second, _, err :=
		NewRabbitVRFDKGPolynomialCommitmentV1(
			context,
			7,
			secondCoefficients,
		)
	if err != nil {
		t.Fatal(err)
	}

	if _, _, err :=
		CanonicalRabbitVRFDKGPolynomialCommitmentsV1(
			context,
			[]RabbitVRFDKGPolynomialCommitmentV1{
				first,
				second,
			},
		); err == nil {
		t.Fatal("duplicate dealer commitment accepted")
	}
}

func TestRabbitVRFDKGPolynomialCommitmentsV1RejectsEmpty(
	t *testing.T,
) {
	context := rabbitVRFDKGTestSessionContextV1(
		t,
		9280,
		11,
	)

	if _, _, err :=
		CanonicalRabbitVRFDKGPolynomialCommitmentsV1(
			context,
			nil,
		); err == nil {
		t.Fatal("empty commitment collection accepted")
	}
}

func TestRabbitVRFDKGPolynomialCommitmentV1RejectsWrongSession(
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

	commitment.SessionID =
		common.HexToHash("0x1234")

	if _, err :=
		ValidateRabbitVRFDKGPolynomialCommitmentV1(
			context,
			commitment,
		); err == nil {
		t.Fatal("wrong session ID accepted")
	}
}
