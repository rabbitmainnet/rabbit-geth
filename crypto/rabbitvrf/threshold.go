package rabbitvrf

import (
	"errors"
	"math/big"
	"sort"

	"github.com/consensys/gnark-crypto/ecc"
	bls12381 "github.com/consensys/gnark-crypto/ecc/bls12-381"
	"github.com/consensys/gnark-crypto/ecc/bls12-381/fr"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

var (
	ErrInvalidShareID         = errors.New("invalid Rabbit VRF share id")
	ErrInvalidSecretShare     = errors.New("invalid Rabbit VRF secret share")
	ErrInvalidPartial         = errors.New("invalid Rabbit VRF partial signature")
	ErrPartialMessageMismatch = errors.New("Rabbit VRF partial message mismatch")
	ErrDuplicateShareID       = errors.New("duplicate Rabbit VRF share id")
	ErrInvalidThreshold       = errors.New("invalid Rabbit VRF threshold")
	ErrInsufficientPartials   = errors.New("insufficient Rabbit VRF partial signatures")
)

type SecretShare struct {
	id     uint64
	scalar fr.Element
}

type VerificationShare struct {
	shareID   uint64
	publicKey PublicKey
}

type PartialSignature struct {
	ShareID   uint64
	Signature Signature
}

// VerifiedPartialSignature is local trusted state produced only after
// cryptographic verification. Its fields are private so raw network partials
// cannot be passed directly into threshold reconstruction by external callers.
type VerifiedPartialSignature struct {
	shareID     uint64
	signature   Signature
	messageHash common.Hash
}

func newSecretShare(
	id uint64,
	scalar fr.Element,
) (*SecretShare, error) {
	if id == 0 {
		return nil, ErrInvalidShareID
	}
	if scalar.IsZero() {
		return nil, ErrInvalidSecretShare
	}

	return &SecretShare{
		id:     id,
		scalar: scalar,
	}, nil
}

func (s *SecretShare) ID() uint64 {
	if s == nil {
		return 0
	}

	return s.id
}

func (s *SecretShare) scalarBigInt() (*big.Int, error) {
	if s == nil ||
		s.id == 0 ||
		s.scalar.IsZero() {
		return nil, ErrInvalidSecretShare
	}

	scalar := new(big.Int)
	s.scalar.ToBigIntRegular(scalar)

	if scalar.Sign() <= 0 {
		return nil, ErrInvalidSecretShare
	}

	return scalar, nil
}

func (s *SecretShare) PublicKey() (PublicKey, error) {
	var out PublicKey

	scalar, err := s.scalarBigInt()
	if err != nil {
		return out, err
	}

	var point bls12381.G1Affine
	point.ScalarMultiplicationBase(scalar)

	if point.IsInfinity() ||
		!point.IsOnCurve() ||
		!point.IsInSubGroup() {
		return out, ErrInvalidPublicKey
	}

	encoded := point.Bytes()
	copy(out[:], encoded[:])

	return out, nil
}

func NewVerificationShare(
	shareID uint64,
	publicKey PublicKey,
) (VerificationShare, error) {
	var out VerificationShare

	if shareID == 0 {
		return out, ErrInvalidShareID
	}

	if _, err := decodePublicKey(publicKey); err != nil {
		return out, ErrInvalidPublicKey
	}

	out.shareID = shareID
	out.publicKey = publicKey

	return out, nil
}

func (s *SecretShare) VerificationShare() (VerificationShare, error) {
	var out VerificationShare

	if s == nil || s.id == 0 {
		return out, ErrInvalidSecretShare
	}

	publicKey, err := s.PublicKey()
	if err != nil {
		return out, err
	}

	return NewVerificationShare(
		s.id,
		publicKey,
	)
}

func (s VerificationShare) ShareID() uint64 {
	return s.shareID
}

func (p VerifiedPartialSignature) ShareID() uint64 {
	return p.shareID
}

func (s *SecretShare) SignPartial(
	message []byte,
) (PartialSignature, error) {
	var out PartialSignature

	if len(message) == 0 {
		return out, ErrInvalidMessage
	}

	scalar, err := s.scalarBigInt()
	if err != nil {
		return out, err
	}

	hashed, err := bls12381.HashToG2(
		message,
		[]byte(HashToG2DSTV1),
	)
	if err != nil {
		return out, err
	}

	if hashed.IsInfinity() ||
		!hashed.IsOnCurve() ||
		!hashed.IsInSubGroup() {
		return out, ErrInvalidPartial
	}

	var point bls12381.G2Affine
	point.ScalarMultiplication(
		&hashed,
		scalar,
	)

	if point.IsInfinity() ||
		!point.IsOnCurve() ||
		!point.IsInSubGroup() {
		return out, ErrInvalidPartial
	}

	encoded := point.Bytes()

	out.ShareID = s.id
	copy(out.Signature[:], encoded[:])

	return out, nil
}

func VerifyPartial(
	verificationShare VerificationShare,
	message []byte,
	partial PartialSignature,
) (VerifiedPartialSignature, error) {
	var out VerifiedPartialSignature

	if verificationShare.shareID == 0 ||
		partial.ShareID == 0 ||
		partial.ShareID != verificationShare.shareID {
		return out, ErrInvalidShareID
	}

	if len(message) == 0 {
		return out, ErrInvalidMessage
	}

	if err := Verify(
		verificationShare.publicKey,
		message,
		partial.Signature,
	); err != nil {
		return out, ErrInvalidPartial
	}

	out.shareID = verificationShare.shareID
	out.signature = partial.Signature
	out.messageHash = crypto.Keccak256Hash(message)

	return out, nil
}

func lagrangeCoefficientAtZero(
	target uint64,
	ids []uint64,
) (fr.Element, error) {
	var zero fr.Element

	if target == 0 {
		return zero, ErrInvalidShareID
	}

	seen := make(map[uint64]struct{}, len(ids))
	foundTarget := false

	for _, id := range ids {
		if id == 0 {
			return zero, ErrInvalidShareID
		}

		if _, exists := seen[id]; exists {
			return zero, ErrDuplicateShareID
		}

		seen[id] = struct{}{}

		if id == target {
			foundTarget = true
		}
	}

	if !foundTarget {
		return zero, ErrInvalidShareID
	}

	numerator := fr.One()
	denominator := fr.One()

	var xTarget fr.Element
	xTarget.SetUint64(target)

	for _, id := range ids {
		if id == target {
			continue
		}

		var xOther fr.Element
		xOther.SetUint64(id)

		var negativeOther fr.Element
		negativeOther.Neg(&xOther)
		numerator.Mul(
			&numerator,
			&negativeOther,
		)

		var difference fr.Element
		difference.Sub(
			&xTarget,
			&xOther,
		)

		if difference.IsZero() {
			return zero, ErrDuplicateShareID
		}

		denominator.Mul(
			&denominator,
			&difference,
		)
	}

	if denominator.IsZero() {
		return zero, ErrDuplicateShareID
	}

	var inverse fr.Element
	inverse.Inverse(&denominator)

	var coefficient fr.Element
	coefficient.Mul(
		&numerator,
		&inverse,
	)

	return coefficient, nil
}

func CombineVerifiedPartials(
	thresholdPublicKey PublicKey,
	message []byte,
	partials []VerifiedPartialSignature,
	threshold int,
) (Signature, error) {
	var out Signature

	if threshold <= 0 {
		return out, ErrInvalidThreshold
	}

	if len(message) == 0 {
		return out, ErrInvalidMessage
	}

	if len(partials) < threshold {
		return out, ErrInsufficientPartials
	}

	if _, err := decodePublicKey(
		thresholdPublicKey,
	); err != nil {
		return out, ErrInvalidPublicKey
	}

	expectedMessageHash := crypto.Keccak256Hash(message)

	ordered := append(
		[]VerifiedPartialSignature(nil),
		partials...,
	)

	sort.Slice(
		ordered,
		func(i, j int) bool {
			return ordered[i].shareID < ordered[j].shareID
		},
	)

	for index, partial := range ordered {
		if partial.shareID == 0 {
			return out, ErrInvalidShareID
		}

		if index > 0 &&
			ordered[index-1].shareID == partial.shareID {
			return out, ErrDuplicateShareID
		}

		if partial.messageHash != expectedMessageHash {
			return out, ErrPartialMessageMismatch
		}
	}

	// Only threshold verified shares are needed. Selecting the lowest ShareIDs
	// gives every honest implementation the same bounded reconstruction path.
	selected := ordered[:threshold]

	ids := make([]uint64, len(selected))
	points := make([]bls12381.G2Affine, len(selected))
	coefficients := make([]fr.Element, len(selected))

	for index, partial := range selected {
		point, err := decodeSignature(
			partial.signature,
		)
		if err != nil {
			return out, ErrInvalidPartial
		}

		ids[index] = partial.shareID
		points[index] = point
	}

	for index, id := range ids {
		coefficient, err := lagrangeCoefficientAtZero(
			id,
			ids,
		)
		if err != nil {
			return out, err
		}

		coefficients[index] = coefficient
	}

	var combined bls12381.G2Affine

	if _, err := combined.MultiExp(
		points,
		coefficients,
		ecc.MultiExpConfig{},
	); err != nil {
		return out, err
	}

	if combined.IsInfinity() ||
		!combined.IsOnCurve() ||
		!combined.IsInSubGroup() {
		return out, ErrInvalidSignature
	}

	encoded := combined.Bytes()
	copy(out[:], encoded[:])

	// Never return a reconstructed signature unless it verifies against the
	// canonical epoch threshold public key and exact canonical message.
	if err := Verify(
		thresholdPublicKey,
		message,
		out,
	); err != nil {
		return Signature{}, ErrInvalidSignature
	}

	return out, nil
}
