package rabbitvrf

import (
	"bytes"
	"errors"
	"testing"

	bls12381 "github.com/consensys/gnark-crypto/ecc/bls12-381"
)

func fixedSecretKey(t *testing.T, value byte) *SecretKey {
	t.Helper()

	encoded := make([]byte, SecretKeySize)
	encoded[len(encoded)-1] = value

	key, err := SecretKeyFromBytes(encoded)
	if err != nil {
		t.Fatalf("fixed secret key: %v", err)
	}

	return key
}

func TestBLSSignVerifyAndRandomness(t *testing.T) {
	secret := fixedSecretKey(t, 42)

	publicKey, err := secret.PublicKey()
	if err != nil {
		t.Fatalf("public key: %v", err)
	}

	message := []byte("rabbit-vrf-test-message-v1")

	signatureA, err := secret.Sign(message)
	if err != nil {
		t.Fatalf("sign A: %v", err)
	}

	signatureB, err := secret.Sign(message)
	if err != nil {
		t.Fatalf("sign B: %v", err)
	}

	if signatureA != signatureB {
		t.Fatal("BLS signature is not unique for the same key and message")
	}

	if err := Verify(publicKey, message, signatureA); err != nil {
		t.Fatalf("verify: %v", err)
	}

	randomnessA, err := VerifyAndDeriveRandomness(
		publicKey,
		message,
		signatureA,
	)
	if err != nil {
		t.Fatalf("derive randomness A: %v", err)
	}

	randomnessB, err := VerifyAndDeriveRandomness(
		publicKey,
		message,
		signatureB,
	)
	if err != nil {
		t.Fatalf("derive randomness B: %v", err)
	}

	if randomnessA != randomnessB {
		t.Fatal("same proof produced different randomness")
	}
	if bytes.Equal(randomnessA[:], make([]byte, len(randomnessA))) {
		t.Fatal("derived zero randomness")
	}
}

func TestBLSRejectsWrongMessageAndKey(t *testing.T) {
	secretA := fixedSecretKey(t, 42)
	secretB := fixedSecretKey(t, 43)

	publicA, err := secretA.PublicKey()
	if err != nil {
		t.Fatal(err)
	}

	publicB, err := secretB.PublicKey()
	if err != nil {
		t.Fatal(err)
	}

	message := []byte("rabbit-vrf-message-a")

	signature, err := secretA.Sign(message)
	if err != nil {
		t.Fatal(err)
	}

	if err := Verify(
		publicA,
		[]byte("rabbit-vrf-message-b"),
		signature,
	); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("wrong message accepted or wrong error: %v", err)
	}

	if err := Verify(
		publicB,
		message,
		signature,
	); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("wrong public key accepted or wrong error: %v", err)
	}
}

func TestSecretKeyCanonicalRoundTrip(t *testing.T) {
	secret := fixedSecretKey(t, 42)

	encoded, err := secret.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	restored, err := SecretKeyFromBytes(encoded[:])
	if err != nil {
		t.Fatal(err)
	}

	publicA, err := secret.PublicKey()
	if err != nil {
		t.Fatal(err)
	}

	publicB, err := restored.PublicKey()
	if err != nil {
		t.Fatal(err)
	}

	if publicA != publicB {
		t.Fatal("secret key round trip changed public key")
	}
}

func TestRejectsInvalidEncodings(t *testing.T) {
	zeroSecret := make([]byte, SecretKeySize)
	if _, err := SecretKeyFromBytes(zeroSecret); !errors.Is(
		err,
		ErrInvalidSecretKey,
	) {
		t.Fatalf("zero secret accepted: %v", err)
	}

	nonCanonicalSecret := bytes.Repeat([]byte{0xff}, SecretKeySize)
	if _, err := SecretKeyFromBytes(nonCanonicalSecret); !errors.Is(
		err,
		ErrInvalidSecretKey,
	) {
		t.Fatalf("non-canonical secret accepted: %v", err)
	}

	var zeroPublic PublicKey
	var zeroSignature Signature

	if err := Verify(
		zeroPublic,
		[]byte("rabbit-vrf-invalid-encoding"),
		zeroSignature,
	); err == nil {
		t.Fatal("invalid public key/signature encoding accepted")
	}

	secret := fixedSecretKey(t, 42)
	publicKey, err := secret.PublicKey()
	if err != nil {
		t.Fatal(err)
	}

	if _, err := secret.Sign(nil); !errors.Is(err, ErrInvalidMessage) {
		t.Fatalf("empty message accepted for signing: %v", err)
	}

	if err := Verify(
		publicKey,
		nil,
		zeroSignature,
	); !errors.Is(err, ErrInvalidMessage) {
		t.Fatalf("empty message accepted for verification: %v", err)
	}
}

func TestGenerateSecretKey(t *testing.T) {
	secret, err := GenerateSecretKey()
	if err != nil {
		t.Fatalf("generate secret: %v", err)
	}

	encoded, err := secret.Bytes()
	if err != nil {
		t.Fatalf("encode secret: %v", err)
	}

	if bytes.Equal(encoded[:], make([]byte, SecretKeySize)) {
		t.Fatal("generated zero secret")
	}

	publicKey, err := secret.PublicKey()
	if err != nil {
		t.Fatalf("public key: %v", err)
	}

	message := []byte("rabbit-vrf-generated-key-test")
	signature, err := secret.Sign(message)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	if err := Verify(publicKey, message, signature); err != nil {
		t.Fatalf("verify generated key: %v", err)
	}
}

func TestRejectsInfinityPoints(t *testing.T) {
	secret := fixedSecretKey(t, 42)

	publicKey, err := secret.PublicKey()
	if err != nil {
		t.Fatal(err)
	}

	message := []byte("rabbit-vrf-infinity-test")

	signature, err := secret.Sign(message)
	if err != nil {
		t.Fatal(err)
	}

	var infinityG1 bls12381.G1Affine
	infinityG1.SetInfinity()
	encodedG1 := infinityG1.Bytes()

	var infinityPublic PublicKey
	copy(infinityPublic[:], encodedG1[:])

	if err := Verify(
		infinityPublic,
		message,
		signature,
	); !errors.Is(err, ErrInvalidPublicKey) {
		t.Fatalf("G1 infinity was not rejected: %v", err)
	}

	var infinityG2 bls12381.G2Affine
	infinityG2.SetInfinity()
	encodedG2 := infinityG2.Bytes()

	var infinitySignature Signature
	copy(infinitySignature[:], encodedG2[:])

	if err := Verify(
		publicKey,
		message,
		infinitySignature,
	); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("G2 infinity was not rejected: %v", err)
	}
}
