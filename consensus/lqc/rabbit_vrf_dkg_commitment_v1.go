package lqc

import (
	"errors"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	rabbitvrf "github.com/ethereum/go-ethereum/crypto/rabbitvrf"
	"github.com/ethereum/go-ethereum/rlp"
)

const RabbitVRFDKGPolynomialCommitmentVersionV1 uint8 = 1

var ErrInvalidRabbitVRFDKGPolynomialCommitmentV1 = errors.New(
	"invalid rabbit vrf dkg polynomial commitment v1",
)

var rabbitVRFDKGPolynomialCommitmentDomainV1 = []byte(
	"RABBIT-VRF-DKG-POLY-COMMITMENT-V1",
)

// RabbitVRFDKGPolynomialCommitmentV1 is one committee member's public Feldman-
// style commitment to a degree threshold-1 polynomial.
//
// Coefficients are ordered:
//
//	C[j] = g1 * a[j]
//
// No private scalar coefficient appears in this structure.
type RabbitVRFDKGPolynomialCommitmentV1 struct {
	Version       uint8
	SessionID     common.Hash
	DealerShareID uint64
	Coefficients  []rabbitvrf.DKGCoefficientCommitmentV1
}

type rabbitVRFDKGPolynomialCommitmentPayloadV1 struct {
	Domain        []byte
	Version       uint8
	SessionID     common.Hash
	DealerShareID uint64
	Coefficients  []rabbitvrf.DKGCoefficientCommitmentV1
}

// NewRabbitVRFDKGPolynomialCommitmentV1 validates and canonicalizes one public
// polynomial commitment.
//
// V1 requires exactly threshold coefficient commitments, corresponding to a
// polynomial of exact degree threshold-1.
//
// The constant coefficient and highest-degree coefficient must be non-zero.
// Intermediate coefficients may be zero and therefore may use the canonical G1
// point-at-infinity encoding.
func NewRabbitVRFDKGPolynomialCommitmentV1(
	context RabbitVRFDKGSessionContextV1,
	dealerShareID uint64,
	coefficients []rabbitvrf.DKGCoefficientCommitmentV1,
) (
	RabbitVRFDKGPolynomialCommitmentV1,
	common.Hash,
	error,
) {
	var out RabbitVRFDKGPolynomialCommitmentV1

	if err := ValidateRabbitVRFDKGSessionContextV1(
		context,
	); err != nil {
		return out,
			common.Hash{},
			ErrInvalidRabbitVRFDKGPolynomialCommitmentV1
	}

	if dealerShareID == 0 ||
		dealerShareID > context.CommitteeSize ||
		uint64(len(coefficients)) != context.Threshold {
		return out,
			common.Hash{},
			ErrInvalidRabbitVRFDKGPolynomialCommitmentV1
	}

	if len(coefficients) == 0 {
		return out,
			common.Hash{},
			ErrInvalidRabbitVRFDKGPolynomialCommitmentV1
	}

	canonical := append(
		[]rabbitvrf.DKGCoefficientCommitmentV1(nil),
		coefficients...,
	)

	last := len(canonical) - 1

	for index, coefficient := range canonical {
		allowInfinity := index > 0 && index < last

		if err := rabbitvrf.ValidateDKGCoefficientCommitmentV1(
			coefficient,
			allowInfinity,
		); err != nil {
			return out,
				common.Hash{},
				ErrInvalidRabbitVRFDKGPolynomialCommitmentV1
		}
	}

	sessionID, err := RabbitVRFDKGSessionIDV1(
		context,
	)
	if err != nil {
		return out,
			common.Hash{},
			ErrInvalidRabbitVRFDKGPolynomialCommitmentV1
	}

	out = RabbitVRFDKGPolynomialCommitmentV1{
		Version:       RabbitVRFDKGPolynomialCommitmentVersionV1,
		SessionID:     sessionID,
		DealerShareID: dealerShareID,
		Coefficients:  canonical,
	}

	encoded, err := rlp.EncodeToBytes(
		rabbitVRFDKGPolynomialCommitmentPayloadV1{
			Domain:        rabbitVRFDKGPolynomialCommitmentDomainV1,
			Version:       out.Version,
			SessionID:     out.SessionID,
			DealerShareID: out.DealerShareID,
			Coefficients:  out.Coefficients,
		},
	)
	if err != nil {
		return RabbitVRFDKGPolynomialCommitmentV1{},
			common.Hash{},
			err
	}

	root := crypto.Keccak256Hash(encoded)
	if root == (common.Hash{}) {
		return RabbitVRFDKGPolynomialCommitmentV1{},
			common.Hash{},
			ErrInvalidRabbitVRFDKGPolynomialCommitmentV1
	}

	return out, root, nil
}

// ValidateRabbitVRFDKGPolynomialCommitmentV1 verifies that a received public
// commitment belongs to the supplied canonical DKG session and obeys the V1
// polynomial-shape rules.
// CanonicalRabbitVRFDKGPolynomialCommitmentsV1 validates a collection of
// public dealer commitments, rejects duplicate immutable DealerShareIDs and
// returns the collection ordered by DealerShareID.
//
// Network arrival order MUST NOT affect the canonical order.
//
// Returned commitments own independent coefficient slices so later mutation of
// an input slice cannot modify the canonical result.
func CanonicalRabbitVRFDKGPolynomialCommitmentsV1(
	context RabbitVRFDKGSessionContextV1,
	commitments []RabbitVRFDKGPolynomialCommitmentV1,
) (
	[]RabbitVRFDKGPolynomialCommitmentV1,
	[]common.Hash,
	error,
) {
	if err := ValidateRabbitVRFDKGSessionContextV1(
		context,
	); err != nil {
		return nil,
			nil,
			ErrInvalidRabbitVRFDKGPolynomialCommitmentV1
	}

	if len(commitments) == 0 ||
		uint64(len(commitments)) > context.CommitteeSize {
		return nil,
			nil,
			ErrInvalidRabbitVRFDKGPolynomialCommitmentV1
	}

	canonical := make(
		[]RabbitVRFDKGPolynomialCommitmentV1,
		len(commitments),
	)

	for index, commitment := range commitments {
		canonical[index] = RabbitVRFDKGPolynomialCommitmentV1{
			Version:       commitment.Version,
			SessionID:     commitment.SessionID,
			DealerShareID: commitment.DealerShareID,
			Coefficients: append(
				[]rabbitvrf.DKGCoefficientCommitmentV1(nil),
				commitment.Coefficients...,
			),
		}
	}

	sort.Slice(
		canonical,
		func(i, j int) bool {
			return canonical[i].DealerShareID <
				canonical[j].DealerShareID
		},
	)

	roots := make(
		[]common.Hash,
		len(canonical),
	)

	var previousDealer uint64

	for index, commitment := range canonical {
		if index > 0 &&
			commitment.DealerShareID == previousDealer {
			return nil,
				nil,
				ErrInvalidRabbitVRFDKGPolynomialCommitmentV1
		}

		root, err :=
			ValidateRabbitVRFDKGPolynomialCommitmentV1(
				context,
				commitment,
			)
		if err != nil {
			return nil, nil, err
		}

		roots[index] = root
		previousDealer = commitment.DealerShareID
	}

	return canonical, roots, nil
}

func ValidateRabbitVRFDKGPolynomialCommitmentV1(
	context RabbitVRFDKGSessionContextV1,
	commitment RabbitVRFDKGPolynomialCommitmentV1,
) (
	common.Hash,
	error,
) {
	if commitment.Version !=
		RabbitVRFDKGPolynomialCommitmentVersionV1 {
		return common.Hash{},
			ErrInvalidRabbitVRFDKGPolynomialCommitmentV1
	}

	rebuilt, root, err :=
		NewRabbitVRFDKGPolynomialCommitmentV1(
			context,
			commitment.DealerShareID,
			commitment.Coefficients,
		)
	if err != nil {
		return common.Hash{}, err
	}

	if rebuilt.SessionID != commitment.SessionID {
		return common.Hash{},
			ErrInvalidRabbitVRFDKGPolynomialCommitmentV1
	}

	return root, nil
}

func RabbitVRFDKGThresholdPublicKeyV1(context RabbitVRFDKGSessionContextV1, commitments []RabbitVRFDKGPolynomialCommitmentV1) (rabbitvrf.PublicKey, []RabbitVRFDKGPolynomialCommitmentV1, []common.Hash, error) {
	var zero rabbitvrf.PublicKey
	canonical, roots, err := CanonicalRabbitVRFDKGPolynomialCommitmentsV1(context, commitments)
	if err != nil {
		return zero, nil, nil, err
	}
	if uint64(len(canonical)) != context.CommitteeSize {
		return zero, nil, nil, ErrInvalidRabbitVRFDKGPolynomialCommitmentV1
	}
	constants := make([]rabbitvrf.DKGCoefficientCommitmentV1, len(canonical))
	for index, commitment := range canonical {
		if len(commitment.Coefficients) == 0 {
			return zero, nil, nil, ErrInvalidRabbitVRFDKGPolynomialCommitmentV1
		}
		constants[index] = commitment.Coefficients[0]
	}
	publicKey, err := rabbitvrf.AggregateDKGConstantCommitmentsV1(constants)
	if err != nil {
		return zero, nil, nil, ErrInvalidRabbitVRFDKGPolynomialCommitmentV1
	}
	return publicKey, canonical, roots, nil
}

var rabbitVRFDKGTranscriptRootDomainV1 = []byte("RABBIT-VRF-DKG-TRANSCRIPT-ROOT-V1")

type rabbitVRFDKGTranscriptRootPayloadV1 struct {
	Domain          []byte
	SessionID       common.Hash
	CommitmentRoots []common.Hash
}

func RabbitVRFDKGTranscriptRootV1(context RabbitVRFDKGSessionContextV1, commitmentRoots []common.Hash) (common.Hash, error) {
	if err := ValidateRabbitVRFDKGSessionContextV1(context); err != nil {
		return common.Hash{}, ErrInvalidRabbitVRFDKGPolynomialCommitmentV1
	}
	if uint64(len(commitmentRoots)) != context.CommitteeSize {
		return common.Hash{}, ErrInvalidRabbitVRFDKGPolynomialCommitmentV1
	}
	for _, root := range commitmentRoots {
		if root == (common.Hash{}) {
			return common.Hash{}, ErrInvalidRabbitVRFDKGPolynomialCommitmentV1
		}
	}
	sessionID, err := RabbitVRFDKGSessionIDV1(context)
	if err != nil {
		return common.Hash{}, ErrInvalidRabbitVRFDKGPolynomialCommitmentV1
	}
	encoded, err := rlp.EncodeToBytes(rabbitVRFDKGTranscriptRootPayloadV1{Domain: rabbitVRFDKGTranscriptRootDomainV1, SessionID: sessionID, CommitmentRoots: append([]common.Hash(nil), commitmentRoots...)})
	if err != nil {
		return common.Hash{}, err
	}
	root := crypto.Keccak256Hash(encoded)
	if root == (common.Hash{}) {
		return common.Hash{}, ErrInvalidRabbitVRFDKGPolynomialCommitmentV1
	}
	return root, nil
}
