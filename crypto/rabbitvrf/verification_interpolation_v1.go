package rabbitvrf

import (
	"sort"

	"github.com/consensys/gnark-crypto/ecc"
	bls12381 "github.com/consensys/gnark-crypto/ecc/bls12-381"
	"github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
)

// PublicKey returns the canonical compressed public key carried by this
// verification share.
func (s VerificationShare) PublicKey() PublicKey {
	return s.publicKey
}

func lagrangeCoefficientAtX(
	sampleID uint64,
	ids []uint64,
	x uint64,
) (fr.Element, error) {
	var zero fr.Element
	if sampleID == 0 || len(ids) == 0 {
		return zero, ErrInvalidShareID
	}

	seen := make(map[uint64]struct{}, len(ids))
	foundSample := false
	for _, id := range ids {
		if id == 0 {
			return zero, ErrInvalidShareID
		}
		if _, exists := seen[id]; exists {
			return zero, ErrDuplicateShareID
		}
		seen[id] = struct{}{}
		if id == sampleID {
			foundSample = true
		}
	}
	if !foundSample {
		return zero, ErrInvalidShareID
	}

	var evaluation fr.Element
	evaluation.SetUint64(x)
	var sample fr.Element
	sample.SetUint64(sampleID)

	numerator := fr.One()
	denominator := fr.One()
	for _, id := range ids {
		if id == sampleID {
			continue
		}

		var other fr.Element
		other.SetUint64(id)

		var numeratorTerm fr.Element
		numeratorTerm.Sub(&evaluation, &other)
		numerator.Mul(&numerator, &numeratorTerm)

		var denominatorTerm fr.Element
		denominatorTerm.Sub(&sample, &other)
		if denominatorTerm.IsZero() {
			return zero, ErrDuplicateShareID
		}
		denominator.Mul(&denominator, &denominatorTerm)
	}

	if denominator.IsZero() {
		return zero, ErrDuplicateShareID
	}
	var inverse fr.Element
	inverse.Inverse(&denominator)
	var coefficient fr.Element
	coefficient.Mul(&numerator, &inverse)
	return coefficient, nil
}

func interpolateVerificationPublicKeyAtV1(
	target uint64,
	ids []uint64,
	points []bls12381.G1Affine,
) (PublicKey, error) {
	var out PublicKey
	if len(ids) == 0 || len(ids) != len(points) {
		return out, ErrInvalidPublicKey
	}

	coefficients := make([]fr.Element, len(ids))
	for index, id := range ids {
		coefficient, err := lagrangeCoefficientAtX(id, ids, target)
		if err != nil {
			return out, err
		}
		coefficients[index] = coefficient
	}

	var interpolated bls12381.G1Affine
	if _, err := interpolated.MultiExp(points, coefficients, ecc.MultiExpConfig{}); err != nil {
		return out, err
	}
	if interpolated.IsInfinity() ||
		!interpolated.IsOnCurve() ||
		!interpolated.IsInSubGroup() {
		return out, ErrInvalidPublicKey
	}

	encoded := interpolated.Bytes()
	copy(out[:], encoded[:])
	if _, err := decodePublicKey(out); err != nil {
		return PublicKey{}, ErrInvalidPublicKey
	}
	return out, nil
}

// ReconstructVerificationSharesV1 deterministically reconstructs the public
// verification share for every ShareID in [1, committeeSize] from exactly one
// threshold-sized canonical sample set. The supplied samples must interpolate
// to the canonical threshold public key at x=0.
//
// This primitive reconstructs public information only; it never reconstructs
// or exposes a secret share.
func ReconstructVerificationSharesV1(
	committeeSize uint64,
	threshold int,
	thresholdPublicKey PublicKey,
	samples []VerificationShare,
) ([]VerificationShare, error) {
	if committeeSize == 0 || threshold <= 0 || uint64(threshold) > committeeSize {
		return nil, ErrInvalidThreshold
	}
	if len(samples) != threshold {
		return nil, ErrInsufficientPartials
	}
	if _, err := decodePublicKey(thresholdPublicKey); err != nil {
		return nil, ErrInvalidPublicKey
	}

	ordered := append([]VerificationShare(nil), samples...)
	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i].shareID < ordered[j].shareID
	})

	ids := make([]uint64, len(ordered))
	points := make([]bls12381.G1Affine, len(ordered))
	for index, sample := range ordered {
		if sample.shareID == 0 || sample.shareID > committeeSize {
			return nil, ErrInvalidShareID
		}
		if index > 0 && ordered[index-1].shareID == sample.shareID {
			return nil, ErrDuplicateShareID
		}
		point, err := decodePublicKey(sample.publicKey)
		if err != nil {
			return nil, ErrInvalidPublicKey
		}
		ids[index] = sample.shareID
		points[index] = point
	}

	interpolatedThresholdKey, err := interpolateVerificationPublicKeyAtV1(0, ids, points)
	if err != nil || interpolatedThresholdKey != thresholdPublicKey {
		return nil, ErrInvalidPublicKey
	}

	out := make([]VerificationShare, 0, committeeSize)
	for shareID := uint64(1); shareID <= committeeSize; shareID++ {
		publicKey, err := interpolateVerificationPublicKeyAtV1(shareID, ids, points)
		if err != nil {
			return nil, err
		}
		verificationShare, err := NewVerificationShare(shareID, publicKey)
		if err != nil {
			return nil, err
		}
		out = append(out, verificationShare)
	}
	return out, nil
}
