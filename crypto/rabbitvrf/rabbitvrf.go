package rabbitvrf

import (
	"errors"
	"math/big"

	bls12381 "github.com/consensys/gnark-crypto/ecc/bls12-381"
	"github.com/consensys/gnark-crypto/ecc/bls12-381/fr"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

const (
	SecretKeySize = 32
	PublicKeySize = bls12381.SizeOfG1AffineCompressed
	SignatureSize = bls12381.SizeOfG2AffineCompressed
)

var (
	ErrInvalidSecretKey = errors.New("invalid Rabbit VRF secret key")
	ErrInvalidPublicKey = errors.New("invalid Rabbit VRF public key")
	ErrInvalidSignature = errors.New("invalid Rabbit VRF signature")
	ErrInvalidMessage   = errors.New("invalid Rabbit VRF message")
)

var (
	hashToG2DST = []byte(
		"RABBIT-VRF-BLS12381G2_XMD:SHA-256_SSWU_RO_V1",
	)
	randomnessDomain = []byte(
		"RABBIT-VRF-RANDOMNESS-V1",
	)
)

type SecretKey struct {
	scalar fr.Element
}

type PublicKey [PublicKeySize]byte

type Signature [SignatureSize]byte

func GenerateSecretKey() (*SecretKey, error) {
	for {
		var scalar fr.Element
		if _, err := scalar.SetRandom(); err != nil {
			return nil, err
		}
		if scalar.IsZero() {
			continue
		}
		return &SecretKey{scalar: scalar}, nil
	}
}

func SecretKeyFromBytes(encoded []byte) (*SecretKey, error) {
	if len(encoded) != SecretKeySize {
		return nil, ErrInvalidSecretKey
	}

	var scalar fr.Element
	if err := scalar.SetBytesCanonical(encoded); err != nil {
		return nil, ErrInvalidSecretKey
	}
	if scalar.IsZero() {
		return nil, ErrInvalidSecretKey
	}

	return &SecretKey{scalar: scalar}, nil
}

func (sk *SecretKey) Bytes() ([SecretKeySize]byte, error) {
	var zero [SecretKeySize]byte

	if sk == nil || sk.scalar.IsZero() {
		return zero, ErrInvalidSecretKey
	}

	return sk.scalar.Bytes(), nil
}

func (sk *SecretKey) scalarBigInt() (*big.Int, error) {
	if sk == nil || sk.scalar.IsZero() {
		return nil, ErrInvalidSecretKey
	}

	scalar := new(big.Int)
	sk.scalar.ToBigIntRegular(scalar)

	if scalar.Sign() <= 0 {
		return nil, ErrInvalidSecretKey
	}

	return scalar, nil
}

func (sk *SecretKey) PublicKey() (PublicKey, error) {
	var out PublicKey

	scalar, err := sk.scalarBigInt()
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

func (sk *SecretKey) Sign(message []byte) (Signature, error) {
	var out Signature

	if len(message) == 0 {
		return out, ErrInvalidMessage
	}

	scalar, err := sk.scalarBigInt()
	if err != nil {
		return out, err
	}

	hashed, err := bls12381.HashToG2(message, hashToG2DST)
	if err != nil {
		return out, err
	}
	if hashed.IsInfinity() ||
		!hashed.IsOnCurve() ||
		!hashed.IsInSubGroup() {
		return out, ErrInvalidSignature
	}

	var signature bls12381.G2Affine
	signature.ScalarMultiplication(&hashed, scalar)

	if signature.IsInfinity() ||
		!signature.IsOnCurve() ||
		!signature.IsInSubGroup() {
		return out, ErrInvalidSignature
	}

	encoded := signature.Bytes()
	copy(out[:], encoded[:])

	return out, nil
}

func decodePublicKey(encoded PublicKey) (bls12381.G1Affine, error) {
	var point bls12381.G1Affine

	consumed, err := point.SetBytes(encoded[:])
	if err != nil ||
		consumed != PublicKeySize ||
		point.IsInfinity() ||
		!point.IsOnCurve() ||
		!point.IsInSubGroup() {
		return bls12381.G1Affine{}, ErrInvalidPublicKey
	}

	return point, nil
}

func decodeSignature(encoded Signature) (bls12381.G2Affine, error) {
	var point bls12381.G2Affine

	consumed, err := point.SetBytes(encoded[:])
	if err != nil ||
		consumed != SignatureSize ||
		point.IsInfinity() ||
		!point.IsOnCurve() ||
		!point.IsInSubGroup() {
		return bls12381.G2Affine{}, ErrInvalidSignature
	}

	return point, nil
}

func Verify(
	publicKey PublicKey,
	message []byte,
	signature Signature,
) error {
	if len(message) == 0 {
		return ErrInvalidMessage
	}

	pk, err := decodePublicKey(publicKey)
	if err != nil {
		return err
	}

	sig, err := decodeSignature(signature)
	if err != nil {
		return err
	}

	hashed, err := bls12381.HashToG2(message, hashToG2DST)
	if err != nil {
		return err
	}
	if hashed.IsInfinity() ||
		!hashed.IsOnCurve() ||
		!hashed.IsInSubGroup() {
		return ErrInvalidSignature
	}

	_, _, generator, _ := bls12381.Generators()

	var negativeGenerator bls12381.G1Affine
	negativeGenerator.Neg(&generator)

	ok, err := bls12381.PairingCheck(
		[]bls12381.G1Affine{
			pk,
			negativeGenerator,
		},
		[]bls12381.G2Affine{
			hashed,
			sig,
		},
	)
	if err != nil {
		return err
	}
	if !ok {
		return ErrInvalidSignature
	}

	return nil
}

func VerifyAndDeriveRandomness(
	publicKey PublicKey,
	message []byte,
	signature Signature,
) (common.Hash, error) {
	if err := Verify(publicKey, message, signature); err != nil {
		return common.Hash{}, err
	}

	return crypto.Keccak256Hash(
		randomnessDomain,
		message,
		signature[:],
	), nil
}
