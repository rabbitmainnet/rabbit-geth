package rabbitvrf

import (
	"errors"
	"testing"

	"github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
)

func testThresholdShares(
	t *testing.T,
	count int,
) ([]*SecretShare, *SecretKey) {
	t.Helper()

	if count < 5 {
		t.Fatal("test requires at least five shares")
	}

	// f(x) = 42 + 7x + 11x^2.
	// Deterministic and test-only. Production MUST use DKG.
	var constant fr.Element
	constant.SetUint64(42)

	var linear fr.Element
	linear.SetUint64(7)

	var quadratic fr.Element
	quadratic.SetUint64(11)

	shares := make([]*SecretShare, count)

	for index := 1; index <= count; index++ {
		var x fr.Element
		x.SetUint64(uint64(index))

		var x2 fr.Element
		x2.Mul(&x, &x)

		var linearTerm fr.Element
		linearTerm.Mul(&linear, &x)

		var quadraticTerm fr.Element
		quadraticTerm.Mul(
			&quadratic,
			&x2,
		)

		var value fr.Element
		value.Add(
			&constant,
			&linearTerm,
		)
		value.Add(
			&value,
			&quadraticTerm,
		)

		share, err := newSecretShare(
			uint64(index),
			value,
		)
		if err != nil {
			t.Fatalf(
				"share %d: %v",
				index,
				err,
			)
		}

		shares[index-1] = share
	}

	encoded := constant.Bytes()

	master, err := SecretKeyFromBytes(
		encoded[:],
	)
	if err != nil {
		t.Fatalf(
			"master secret: %v",
			err,
		)
	}

	return shares, master
}

func testVerifiedPartial(
	t *testing.T,
	share *SecretShare,
	message []byte,
) VerifiedPartialSignature {
	t.Helper()

	partial, err := share.SignPartial(message)
	if err != nil {
		t.Fatal(err)
	}

	verificationShare, err := share.VerificationShare()
	if err != nil {
		t.Fatal(err)
	}

	verified, err := VerifyPartial(
		verificationShare,
		message,
		partial,
	)
	if err != nil {
		t.Fatal(err)
	}

	return verified
}

func TestThresholdDifferentSubsetsProduceSameSignature(t *testing.T) {
	shares, master := testThresholdShares(t, 5)
	message := []byte("rabbit-vrf-threshold-subset-test-v1")

	masterPublic, err := master.PublicKey()
	if err != nil {
		t.Fatal(err)
	}

	verified := make(
		[]VerifiedPartialSignature,
		len(shares),
	)

	for index, share := range shares {
		verified[index] = testVerifiedPartial(
			t,
			share,
			message,
		)
	}

	subsets := [][]VerifiedPartialSignature{
		{
			verified[0],
			verified[1],
			verified[2],
		},
		{
			verified[0],
			verified[2],
			verified[4],
		},
		{
			verified[1],
			verified[3],
			verified[4],
		},
	}

	signatures := make([]Signature, len(subsets))

	for index, subset := range subsets {
		combined, err := CombineVerifiedPartials(
			masterPublic,
			message,
			subset,
			3,
		)
		if err != nil {
			t.Fatalf(
				"combine subset %d: %v",
				index,
				err,
			)
		}

		signatures[index] = combined
	}

	if signatures[0] != signatures[1] ||
		signatures[0] != signatures[2] {
		t.Fatal(
			"valid subsets produced different signatures",
		)
	}

	randomA, err := VerifyAndDeriveRandomness(
		masterPublic,
		message,
		signatures[0],
	)
	if err != nil {
		t.Fatal(err)
	}

	randomB, err := VerifyAndDeriveRandomness(
		masterPublic,
		message,
		signatures[1],
	)
	if err != nil {
		t.Fatal(err)
	}

	randomC, err := VerifyAndDeriveRandomness(
		masterPublic,
		message,
		signatures[2],
	)
	if err != nil {
		t.Fatal(err)
	}

	if randomA != randomB ||
		randomA != randomC {
		t.Fatal(
			"valid subsets produced different randomness",
		)
	}
}

func TestThresholdOrderAndMasterSignatureUniqueness(t *testing.T) {
	shares, master := testThresholdShares(t, 5)
	message := []byte("rabbit-vrf-threshold-order-test-v1")

	masterPublic, err := master.PublicKey()
	if err != nil {
		t.Fatal(err)
	}

	masterSignature, err := master.Sign(message)
	if err != nil {
		t.Fatal(err)
	}

	a := testVerifiedPartial(
		t,
		shares[0],
		message,
	)
	b := testVerifiedPartial(
		t,
		shares[1],
		message,
	)
	c := testVerifiedPartial(
		t,
		shares[2],
		message,
	)

	forward, err := CombineVerifiedPartials(
		masterPublic,
		message,
		[]VerifiedPartialSignature{
			a,
			b,
			c,
		},
		3,
	)
	if err != nil {
		t.Fatal(err)
	}

	reverse, err := CombineVerifiedPartials(
		masterPublic,
		message,
		[]VerifiedPartialSignature{
			c,
			b,
			a,
		},
		3,
	)
	if err != nil {
		t.Fatal(err)
	}

	if forward != reverse {
		t.Fatal(
			"partial ordering changed threshold signature",
		)
	}

	if forward != masterSignature {
		t.Fatal(
			"threshold signature differs from direct master signature",
		)
	}
}

func TestThresholdMoreThanThresholdUsesCanonicalSubset(t *testing.T) {
	shares, master := testThresholdShares(t, 5)
	message := []byte("rabbit-vrf-threshold-canonical-subset-v1")

	masterPublic, err := master.PublicKey()
	if err != nil {
		t.Fatal(err)
	}

	all := make(
		[]VerifiedPartialSignature,
		0,
		len(shares),
	)

	for _, share := range shares {
		all = append(
			all,
			testVerifiedPartial(
				t,
				share,
				message,
			),
		)
	}

	combined, err := CombineVerifiedPartials(
		masterPublic,
		message,
		[]VerifiedPartialSignature{
			all[4],
			all[2],
			all[0],
			all[3],
			all[1],
		},
		3,
	)
	if err != nil {
		t.Fatal(err)
	}

	masterSignature, err := master.Sign(message)
	if err != nil {
		t.Fatal(err)
	}

	if combined != masterSignature {
		t.Fatal(
			"canonical threshold subset produced wrong signature",
		)
	}
}

func TestThresholdRejectsDuplicateAndInsufficientShares(t *testing.T) {
	shares, master := testThresholdShares(t, 5)
	message := []byte("rabbit-vrf-threshold-negative-test")

	masterPublic, err := master.PublicKey()
	if err != nil {
		t.Fatal(err)
	}

	a := testVerifiedPartial(
		t,
		shares[0],
		message,
	)
	b := testVerifiedPartial(
		t,
		shares[1],
		message,
	)

	if _, err := CombineVerifiedPartials(
		masterPublic,
		message,
		[]VerifiedPartialSignature{
			a,
			a,
			b,
		},
		3,
	); !errors.Is(err, ErrDuplicateShareID) {
		t.Fatalf(
			"duplicate share accepted: %v",
			err,
		)
	}

	if _, err := CombineVerifiedPartials(
		masterPublic,
		message,
		[]VerifiedPartialSignature{
			a,
			b,
		},
		3,
	); !errors.Is(err, ErrInsufficientPartials) {
		t.Fatalf(
			"insufficient shares accepted: %v",
			err,
		)
	}

	if _, err := CombineVerifiedPartials(
		masterPublic,
		message,
		[]VerifiedPartialSignature{
			a,
		},
		0,
	); !errors.Is(err, ErrInvalidThreshold) {
		t.Fatalf(
			"invalid threshold accepted: %v",
			err,
		)
	}
}

func TestThresholdRejectsWrongPartialMessage(t *testing.T) {
	shares, _ := testThresholdShares(t, 5)

	partial, err := shares[0].SignPartial(
		[]byte("rabbit-vrf-message-a"),
	)
	if err != nil {
		t.Fatal(err)
	}

	verificationShare, err := shares[0].VerificationShare()
	if err != nil {
		t.Fatal(err)
	}

	if _, err := VerifyPartial(
		verificationShare,
		[]byte("rabbit-vrf-message-b"),
		partial,
	); !errors.Is(err, ErrInvalidPartial) {
		t.Fatalf(
			"wrong message accepted: %v",
			err,
		)
	}
}

func TestThresholdRejectsRelabeledShareID(t *testing.T) {
	shares, _ := testThresholdShares(t, 5)
	message := []byte("rabbit-vrf-share-id-binding-test")

	partial, err := shares[0].SignPartial(message)
	if err != nil {
		t.Fatal(err)
	}

	verificationShare, err := shares[0].VerificationShare()
	if err != nil {
		t.Fatal(err)
	}

	partial.ShareID = shares[1].ID()

	if _, err := VerifyPartial(
		verificationShare,
		message,
		partial,
	); !errors.Is(err, ErrInvalidShareID) {
		t.Fatalf(
			"relabeled partial accepted: %v",
			err,
		)
	}
}

func TestThresholdRejectsMixedMessages(t *testing.T) {
	shares, master := testThresholdShares(t, 5)

	messageA := []byte("rabbit-vrf-round-a")
	messageB := []byte("rabbit-vrf-round-b")

	masterPublic, err := master.PublicKey()
	if err != nil {
		t.Fatal(err)
	}

	a := testVerifiedPartial(
		t,
		shares[0],
		messageA,
	)
	b := testVerifiedPartial(
		t,
		shares[1],
		messageA,
	)
	c := testVerifiedPartial(
		t,
		shares[2],
		messageB,
	)

	if _, err := CombineVerifiedPartials(
		masterPublic,
		messageA,
		[]VerifiedPartialSignature{
			a,
			b,
			c,
		},
		3,
	); !errors.Is(err, ErrPartialMessageMismatch) {
		t.Fatalf(
			"mixed messages accepted: %v",
			err,
		)
	}
}

func TestThresholdRejectsZeroVerifiedPartial(t *testing.T) {
	shares, master := testThresholdShares(t, 5)
	message := []byte("rabbit-vrf-zero-verified-test")

	masterPublic, err := master.PublicKey()
	if err != nil {
		t.Fatal(err)
	}

	a := testVerifiedPartial(
		t,
		shares[0],
		message,
	)
	b := testVerifiedPartial(
		t,
		shares[1],
		message,
	)

	var zero VerifiedPartialSignature

	if _, err := CombineVerifiedPartials(
		masterPublic,
		message,
		[]VerifiedPartialSignature{
			a,
			b,
			zero,
		},
		3,
	); !errors.Is(err, ErrInvalidShareID) {
		t.Fatalf(
			"zero verified partial accepted: %v",
			err,
		)
	}
}

func TestThresholdRejectsWrongThresholdPublicKey(t *testing.T) {
	shares, _ := testThresholdShares(t, 5)
	message := []byte("rabbit-vrf-wrong-threshold-key-test")

	wrongMaster := fixedSecretKey(t, 43)

	wrongPublic, err := wrongMaster.PublicKey()
	if err != nil {
		t.Fatal(err)
	}

	a := testVerifiedPartial(
		t,
		shares[0],
		message,
	)
	b := testVerifiedPartial(
		t,
		shares[1],
		message,
	)
	c := testVerifiedPartial(
		t,
		shares[2],
		message,
	)

	if _, err := CombineVerifiedPartials(
		wrongPublic,
		message,
		[]VerifiedPartialSignature{
			a,
			b,
			c,
		},
		3,
	); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf(
			"wrong threshold public key accepted: %v",
			err,
		)
	}
}

func TestVerificationShareValidation(t *testing.T) {
	shares, _ := testThresholdShares(t, 5)

	publicKey, err := shares[0].PublicKey()
	if err != nil {
		t.Fatal(err)
	}

	if _, err := NewVerificationShare(
		0,
		publicKey,
	); !errors.Is(err, ErrInvalidShareID) {
		t.Fatalf(
			"zero verification share id accepted: %v",
			err,
		)
	}

	var invalidPublic PublicKey

	if _, err := NewVerificationShare(
		1,
		invalidPublic,
	); !errors.Is(err, ErrInvalidPublicKey) {
		t.Fatalf(
			"invalid verification public key accepted: %v",
			err,
		)
	}
}
