package rabbitvrf

import (
	"bytes"
	"math"
	"testing"

	"github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
)

func fuzzShuffleVerifiedPartials(
	input []VerifiedPartialSignature,
	state uint64,
) []VerifiedPartialSignature {
	out := append(
		[]VerifiedPartialSignature(nil),
		input...,
	)

	if state == 0 {
		state = 0x9e3779b97f4a7c15
	}

	for i := len(out) - 1; i > 0; i-- {
		state ^= state << 13
		state ^= state >> 7
		state ^= state << 17

		j := int(
			state % uint64(i+1),
		)

		out[i], out[j] =
			out[j], out[i]
	}

	return out
}

func FuzzThresholdReconstruction(
	f *testing.F,
) {
	f.Add(
		uint8(3),
		uint8(5),
		uint64(1),
		uint64(7),
		[]byte("rabbit-vrf-fuzz-threshold-a"),
	)

	f.Add(
		uint8(0),
		uint8(0),
		uint64(math.MaxUint64),
		uint64(1),
		[]byte("rabbit-vrf-fuzz-threshold-b"),
	)

	f.Add(
		uint8(4),
		uint8(7),
		uint64(9223372036854775808),
		uint64(0x123456789abcdef0),
		[]byte{0x00, 0x01, 0xfe, 0xff},
	)

	f.Fuzz(func(
		t *testing.T,
		thresholdRaw uint8,
		committeeRaw uint8,
		base uint64,
		shuffleSeed uint64,
		message []byte,
	) {
		if len(message) == 0 ||
			len(message) > 256 {
			return
		}

		threshold :=
			int(thresholdRaw%5) + 1

		maxExtra := 8 - threshold

		committee :=
			threshold +
				int(
					committeeRaw%
						uint8(maxExtra+1),
				)

		maxStart :=
			math.MaxUint64 -
				uint64(committee) +
				1

		baseID :=
			uint64(1) +
				base%maxStart

		shareIDs := make(
			[]uint64,
			committee,
		)

		for i := range shareIDs {
			shareIDs[i] =
				baseID + uint64(i)
		}

		polynomialSeed :=
			(base ^ shuffleSeed) %
				1_000_000

		shares, master :=
			propertyThresholdShares(
				t,
				threshold,
				shareIDs,
				polynomialSeed,
			)

		masterPublic, err :=
			master.PublicKey()
		if err != nil {
			t.Fatal(err)
		}

		directSignature, err :=
			master.Sign(message)
		if err != nil {
			t.Fatal(err)
		}

		directRandomness, err :=
			VerifyAndDeriveRandomness(
				masterPublic,
				message,
				directSignature,
			)
		if err != nil {
			t.Fatal(err)
		}

		verified :=
			propertyVerifiedPartials(
				t,
				shares,
				message,
			)

		shuffled :=
			fuzzShuffleVerifiedPartials(
				verified,
				shuffleSeed,
			)

		subset := append(
			[]VerifiedPartialSignature(nil),
			shuffled[:threshold]...,
		)

		propertyAssertReconstruction(
			t,
			masterPublic,
			message,
			shuffled,
			threshold,
			directSignature,
			directRandomness,
		)

		propertyAssertReconstruction(
			t,
			masterPublic,
			message,
			subset,
			threshold,
			directSignature,
			directRandomness,
		)
	})
}

func FuzzVerifyPartialRelabeledShareID(
	f *testing.F,
) {
	f.Add(
		uint64(1),
		uint64(2),
		[]byte("rabbit-vrf-fuzz-relabel-a"),
	)

	f.Add(
		uint64(math.MaxUint64),
		uint64(0),
		[]byte("rabbit-vrf-fuzz-relabel-b"),
	)

	f.Add(
		uint64(77),
		uint64(77),
		[]byte{0x01, 0x02, 0x03},
	)

	f.Fuzz(func(
		t *testing.T,
		originalID uint64,
		label uint64,
		message []byte,
	) {
		if len(message) == 0 ||
			len(message) > 256 {
			return
		}

		if originalID == 0 {
			originalID = 1
		}

		var scalar fr.Element
		scalar.SetUint64(1234567)

		share, err := newSecretShare(
			originalID,
			scalar,
		)
		if err != nil {
			t.Fatal(err)
		}

		verificationShare, err :=
			share.VerificationShare()
		if err != nil {
			t.Fatal(err)
		}

		partial, err :=
			share.SignPartial(message)
		if err != nil {
			t.Fatal(err)
		}

		partial.ShareID = label

		_, err = VerifyPartial(
			verificationShare,
			message,
			partial,
		)

		if label == originalID {
			if err != nil {
				t.Fatalf(
					"valid original ShareID rejected: %v",
					err,
				)
			}

			return
		}

		if err == nil {
			t.Fatalf(
				"relabeled partial accepted: original=%d label=%d",
				originalID,
				label,
			)
		}
	})
}

func FuzzVerifyPartialMutatedSignature(
	f *testing.F,
) {
	f.Add(
		uint64(1),
		uint16(0),
		byte(1),
		[]byte("rabbit-vrf-fuzz-signature-a"),
	)

	f.Add(
		uint64(math.MaxUint64),
		uint16(SignatureSize-1),
		byte(0xff),
		[]byte("rabbit-vrf-fuzz-signature-b"),
	)

	f.Fuzz(func(
		t *testing.T,
		shareID uint64,
		index uint16,
		delta byte,
		message []byte,
	) {
		if len(message) == 0 ||
			len(message) > 256 {
			return
		}

		if shareID == 0 {
			shareID = 1
		}

		if delta == 0 {
			delta = 1
		}

		var scalar fr.Element
		scalar.SetUint64(7654321)

		share, err := newSecretShare(
			shareID,
			scalar,
		)
		if err != nil {
			t.Fatal(err)
		}

		verificationShare, err :=
			share.VerificationShare()
		if err != nil {
			t.Fatal(err)
		}

		partial, err :=
			share.SignPartial(message)
		if err != nil {
			t.Fatal(err)
		}

		position :=
			int(index) % SignatureSize

		partial.Signature[position] ^= delta

		if _, err := VerifyPartial(
			verificationShare,
			message,
			partial,
		); err == nil {
			t.Fatalf(
				"mutated signature accepted at byte %d",
				position,
			)
		}
	})
}

func FuzzDecodePublicKeyCanonical(
	f *testing.F,
) {
	secretBytes := make(
		[]byte,
		SecretKeySize,
	)

	secretBytes[SecretKeySize-1] = 1

	secret, err :=
		SecretKeyFromBytes(secretBytes)
	if err != nil {
		f.Fatal(err)
	}

	publicKey, err :=
		secret.PublicKey()
	if err != nil {
		f.Fatal(err)
	}

	f.Add(publicKey[:])
	f.Add(make([]byte, PublicKeySize))

	allFF := make(
		[]byte,
		PublicKeySize,
	)

	for i := range allFF {
		allFF[i] = 0xff
	}

	f.Add(allFF)

	f.Fuzz(func(
		t *testing.T,
		raw []byte,
	) {
		if len(raw) != PublicKeySize {
			return
		}

		var encoded PublicKey
		copy(encoded[:], raw)

		point, err :=
			decodePublicKey(encoded)

		if err != nil {
			return
		}

		canonical := point.Bytes()

		if !bytes.Equal(
			canonical[:],
			encoded[:],
		) {
			t.Fatal(
				"accepted public key is not canonical",
			)
		}

		if point.IsInfinity() ||
			!point.IsOnCurve() ||
			!point.IsInSubGroup() {
			t.Fatal(
				"accepted public key violates point validation",
			)
		}
	})
}

func FuzzDecodeSignatureCanonical(
	f *testing.F,
) {
	secretBytes := make(
		[]byte,
		SecretKeySize,
	)

	secretBytes[SecretKeySize-1] = 1

	secret, err :=
		SecretKeyFromBytes(secretBytes)
	if err != nil {
		f.Fatal(err)
	}

	signature, err :=
		secret.Sign(
			[]byte("rabbit-vrf-fuzz-signature-seed"),
		)
	if err != nil {
		f.Fatal(err)
	}

	f.Add(signature[:])
	f.Add(make([]byte, SignatureSize))

	allFF := make(
		[]byte,
		SignatureSize,
	)

	for i := range allFF {
		allFF[i] = 0xff
	}

	f.Add(allFF)

	f.Fuzz(func(
		t *testing.T,
		raw []byte,
	) {
		if len(raw) != SignatureSize {
			return
		}

		var encoded Signature
		copy(encoded[:], raw)

		point, err :=
			decodeSignature(encoded)

		if err != nil {
			return
		}

		canonical := point.Bytes()

		if !bytes.Equal(
			canonical[:],
			encoded[:],
		) {
			t.Fatal(
				"accepted signature is not canonical",
			)
		}

		if point.IsInfinity() ||
			!point.IsOnCurve() ||
			!point.IsInSubGroup() {
			t.Fatal(
				"accepted signature violates point validation",
			)
		}
	})
}
