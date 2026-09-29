package rabbitvrf

import (
	"testing"

	"github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
)

func interpolationSecretShareV1(t *testing.T, id uint64, coefficients []uint64) *SecretShare {
	t.Helper()
	var x fr.Element
	x.SetUint64(id)
	power := fr.One()
	var value fr.Element
	for _, rawCoefficient := range coefficients {
		var coefficient fr.Element
		coefficient.SetUint64(rawCoefficient)
		var term fr.Element
		term.Mul(&coefficient, &power)
		value.Add(&value, &term)
		power.Mul(&power, &x)
	}
	encoded := value.Bytes()
	share, err := SecretShareFromBytes(id, encoded[:])
	if err != nil {
		t.Fatal(err)
	}
	return share
}

func interpolationThresholdPublicKeyV1(t *testing.T, constant uint64) PublicKey {
	t.Helper()
	var scalar fr.Element
	scalar.SetUint64(constant)
	encoded := scalar.Bytes()
	share, err := SecretShareFromBytes(99, encoded[:])
	if err != nil {
		t.Fatal(err)
	}
	publicKey, err := share.PublicKey()
	if err != nil {
		t.Fatal(err)
	}
	return publicKey
}

func TestReconstructVerificationSharesV1Canonical(t *testing.T) {
	const committeeSize = uint64(7)
	const threshold = 3
	coefficients := []uint64{17, 23, 31}
	thresholdPublicKey := interpolationThresholdPublicKeyV1(t, coefficients[0])

	expected := make([]VerificationShare, committeeSize)
	for shareID := uint64(1); shareID <= committeeSize; shareID++ {
		share := interpolationSecretShareV1(t, shareID, coefficients)
		verificationShare, err := share.VerificationShare()
		if err != nil {
			t.Fatal(err)
		}
		expected[shareID-1] = verificationShare
	}

	samples := []VerificationShare{expected[4], expected[0], expected[2]}
	reconstructed, err := ReconstructVerificationSharesV1(
		committeeSize,
		threshold,
		thresholdPublicKey,
		samples,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(reconstructed) != len(expected) {
		t.Fatalf("reconstructed share count=%d want=%d", len(reconstructed), len(expected))
	}
	for index := range expected {
		if reconstructed[index].ShareID() != expected[index].ShareID() ||
			reconstructed[index].PublicKey() != expected[index].PublicKey() {
			t.Fatalf("reconstructed share %d mismatch", index+1)
		}
	}

	reordered, err := ReconstructVerificationSharesV1(
		committeeSize,
		threshold,
		thresholdPublicKey,
		[]VerificationShare{expected[2], expected[4], expected[0]},
	)
	if err != nil {
		t.Fatal(err)
	}
	for index := range reconstructed {
		if reordered[index].ShareID() != reconstructed[index].ShareID() ||
			reordered[index].PublicKey() != reconstructed[index].PublicKey() {
			t.Fatalf("reordered reconstruction changed share %d", index+1)
		}
	}
}

func TestReconstructVerificationSharesV1Adversarial(t *testing.T) {
	coefficients := []uint64{17, 23, 31}
	thresholdPublicKey := interpolationThresholdPublicKeyV1(t, coefficients[0])
	shares := make([]VerificationShare, 8)
	for shareID := uint64(1); shareID <= 8; shareID++ {
		share := interpolationSecretShareV1(t, shareID, coefficients)
		verificationShare, err := share.VerificationShare()
		if err != nil {
			t.Fatal(err)
		}
		shares[shareID-1] = verificationShare
	}

	t.Run("insufficient", func(t *testing.T) {
		if _, err := ReconstructVerificationSharesV1(7, 3, thresholdPublicKey, shares[:2]); err == nil {
			t.Fatal("expected insufficient sample rejection")
		}
	})

	t.Run("duplicate", func(t *testing.T) {
		if _, err := ReconstructVerificationSharesV1(
			7,
			3,
			thresholdPublicKey,
			[]VerificationShare{shares[0], shares[0], shares[2]},
		); err == nil {
			t.Fatal("expected duplicate ShareID rejection")
		}
	})

	t.Run("zero_share_id", func(t *testing.T) {
		zero := shares[0]
		zero.shareID = 0
		if _, err := ReconstructVerificationSharesV1(
			7,
			3,
			thresholdPublicKey,
			[]VerificationShare{zero, shares[1], shares[2]},
		); err == nil {
			t.Fatal("expected zero ShareID rejection")
		}
	})

	t.Run("out_of_range_share_id", func(t *testing.T) {
		if _, err := ReconstructVerificationSharesV1(
			7,
			3,
			thresholdPublicKey,
			[]VerificationShare{shares[0], shares[1], shares[7]},
		); err == nil {
			t.Fatal("expected out-of-range ShareID rejection")
		}
	})

	t.Run("wrong_threshold_public_key", func(t *testing.T) {
		wrongThresholdPublicKey := interpolationThresholdPublicKeyV1(t, 19)
		if _, err := ReconstructVerificationSharesV1(
			7,
			3,
			wrongThresholdPublicKey,
			[]VerificationShare{shares[0], shares[2], shares[4]},
		); err == nil {
			t.Fatal("expected wrong threshold public key rejection")
		}
	})

	t.Run("tampered_sample", func(t *testing.T) {
		tampered := shares[2]
		tampered.publicKey = shares[3].publicKey
		if _, err := ReconstructVerificationSharesV1(
			7,
			3,
			thresholdPublicKey,
			[]VerificationShare{shares[0], tampered, shares[4]},
		); err == nil {
			t.Fatal("expected tampered sample rejection")
		}
	})
}
