package rabbitvrf

import (
	"errors"

	bls12381 "github.com/consensys/gnark-crypto/ecc/bls12-381"

	"github.com/ethereum/go-ethereum/crypto"
)

var (
	ErrInvalidAggregateSignatureV1 = errors.New("invalid Rabbit VRF aggregate signature v1")
	ErrDuplicateAggregateShareV1   = errors.New("duplicate Rabbit VRF aggregate share v1")
	ErrDuplicateAggregateMessageV1 = errors.New("duplicate Rabbit VRF aggregate message v1")
)

// AggregateSignaturesV1 adds independently valid G2 signatures into one
// canonical compressed BLS12-381 signature. It does not verify message/key
// relationships; callers must use VerifyAggregateDistinctMessagesV1 before
// treating the result as authenticated state.
func AggregateSignaturesV1(signatures []Signature) (Signature, error) {
	var out Signature
	if len(signatures) == 0 {
		return out, ErrInvalidAggregateSignatureV1
	}

	points := make([]bls12381.G2Affine, len(signatures))
	for index, signature := range signatures {
		point, err := decodeSignature(signature)
		if err != nil {
			return out, ErrInvalidAggregateSignatureV1
		}
		points[index] = point
	}

	var aggregate bls12381.G2Affine
	aggregate.Set(&points[0])
	for index := 1; index < len(points); index++ {
		var next bls12381.G2Affine
		next.Add(&aggregate, &points[index])
		aggregate = next
	}

	if aggregate.IsInfinity() ||
		!aggregate.IsOnCurve() ||
		!aggregate.IsInSubGroup() {
		return out, ErrInvalidAggregateSignatureV1
	}

	encoded := aggregate.Bytes()
	copy(out[:], encoded[:])
	return out, nil
}

// VerifyAggregateDistinctMessagesV1 verifies one aggregate signature where
// every canonical verification share signs its own non-empty, distinct
// message. Distinct messages are required because this primitive is intended
// for compact participant attestations, not same-message threshold
// reconstruction.
func VerifyAggregateDistinctMessagesV1(
	verificationShares []VerificationShare,
	messages [][]byte,
	signature Signature,
) error {
	if len(verificationShares) == 0 ||
		len(verificationShares) != len(messages) {
		return ErrInvalidAggregateSignatureV1
	}

	aggregate, err := decodeSignature(signature)
	if err != nil {
		return ErrInvalidAggregateSignatureV1
	}

	p := make([]bls12381.G1Affine, 0, len(verificationShares)+1)
	q := make([]bls12381.G2Affine, 0, len(verificationShares)+1)
	seenShares := make(map[uint64]struct{}, len(verificationShares))
	seenMessages := make(map[[32]byte]struct{}, len(messages))

	for index, verificationShare := range verificationShares {
		if verificationShare.shareID == 0 {
			return ErrInvalidShareID
		}
		if _, exists := seenShares[verificationShare.shareID]; exists {
			return ErrDuplicateAggregateShareV1
		}
		seenShares[verificationShare.shareID] = struct{}{}

		message := messages[index]
		if len(message) == 0 {
			return ErrInvalidMessage
		}
		messageHash := crypto.Keccak256Hash(message)
		if _, exists := seenMessages[messageHash]; exists {
			return ErrDuplicateAggregateMessageV1
		}
		seenMessages[messageHash] = struct{}{}

		publicKey, err := decodePublicKey(verificationShare.publicKey)
		if err != nil {
			return ErrInvalidAggregateSignatureV1
		}
		hashed, err := bls12381.HashToG2(message, []byte(HashToG2DSTV1))
		if err != nil || hashed.IsInfinity() || !hashed.IsOnCurve() || !hashed.IsInSubGroup() {
			return ErrInvalidAggregateSignatureV1
		}

		p = append(p, publicKey)
		q = append(q, hashed)
	}

	_, _, generator, _ := bls12381.Generators()
	var negativeGenerator bls12381.G1Affine
	negativeGenerator.Neg(&generator)
	p = append(p, negativeGenerator)
	q = append(q, aggregate)

	ok, err := bls12381.PairingCheck(p, q)
	if err != nil {
		return err
	}
	if !ok {
		return ErrInvalidAggregateSignatureV1
	}
	return nil
}
