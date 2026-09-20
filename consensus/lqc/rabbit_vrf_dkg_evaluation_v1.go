package lqc

import (
	"errors"

	rabbitvrf "github.com/ethereum/go-ethereum/crypto/rabbitvrf"
)

var ErrInvalidRabbitVRFDKGPrivateEvaluationV1 = errors.New(
	"invalid rabbit vrf dkg private evaluation v1",
)

// VerifyRabbitVRFDKGPrivateEvaluationV1 verifies one dealer's private
// polynomial evaluation for one immutable committee recipient.
//
// The supplied public commitment is first validated against the canonical DKG
// session. This binds evaluation verification to the correct chain, target VRF
// epoch, committee, threshold and dealer ShareID.
func VerifyRabbitVRFDKGPrivateEvaluationV1(
	context RabbitVRFDKGSessionContextV1,
	commitment RabbitVRFDKGPolynomialCommitmentV1,
	recipientShareID uint64,
	evaluation rabbitvrf.DKGPolynomialEvaluationV1,
) error {
	if err := ValidateRabbitVRFDKGSessionContextV1(
		context,
	); err != nil {
		return ErrInvalidRabbitVRFDKGPrivateEvaluationV1
	}

	if recipientShareID == 0 ||
		recipientShareID > context.CommitteeSize {
		return ErrInvalidRabbitVRFDKGPrivateEvaluationV1
	}

	if _, err :=
		ValidateRabbitVRFDKGPolynomialCommitmentV1(
			context,
			commitment,
		); err != nil {
		return ErrInvalidRabbitVRFDKGPrivateEvaluationV1
	}

	if err := rabbitvrf.VerifyDKGPolynomialEvaluationV1(
		recipientShareID,
		evaluation,
		commitment.Coefficients,
	); err != nil {
		return ErrInvalidRabbitVRFDKGPrivateEvaluationV1
	}

	return nil
}
