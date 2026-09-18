package rabbitvrf

import (
	"errors"
	"math"
	"testing"

	"github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
)

func propertyPolynomial(
	threshold int,
	seed uint64,
) []fr.Element {
	coefficients := make(
		[]fr.Element,
		threshold,
	)

	coefficients[0].SetUint64(
		42 + seed,
	)

	for i := 1; i < threshold; i++ {
		coefficients[i].SetUint64(
			7 + seed + uint64(i)*11,
		)
	}

	return coefficients
}

func propertyEvaluatePolynomial(
	coefficients []fr.Element,
	shareID uint64,
) fr.Element {
	var x fr.Element
	x.SetUint64(shareID)

	value := coefficients[len(coefficients)-1]

	for i := len(coefficients) - 2; i >= 0; i-- {
		var next fr.Element

		next.Mul(
			&value,
			&x,
		)

		next.Add(
			&next,
			&coefficients[i],
		)

		value = next
	}

	return value
}

func propertyThresholdShares(
	t *testing.T,
	threshold int,
	shareIDs []uint64,
	seed uint64,
) ([]*SecretShare, *SecretKey) {
	t.Helper()

	if threshold <= 0 {
		t.Fatal("invalid test threshold")
	}

	if len(shareIDs) < threshold {
		t.Fatal("test committee smaller than threshold")
	}

	seen := make(map[uint64]struct{})

	for _, shareID := range shareIDs {
		if shareID == 0 {
			t.Fatal("zero ShareID in property test")
		}

		if _, ok := seen[shareID]; ok {
			t.Fatalf(
				"duplicate ShareID in property test: %d",
				shareID,
			)
		}

		seen[shareID] = struct{}{}
	}

	coefficients := propertyPolynomial(
		threshold,
		seed,
	)

	masterBytes := coefficients[0].Bytes()

	master, err := SecretKeyFromBytes(
		masterBytes[:],
	)
	if err != nil {
		t.Fatalf(
			"master secret: %v",
			err,
		)
	}

	shares := make(
		[]*SecretShare,
		0,
		len(shareIDs),
	)

	for _, shareID := range shareIDs {
		scalar := propertyEvaluatePolynomial(
			coefficients,
			shareID,
		)

		share, err := newSecretShare(
			shareID,
			scalar,
		)
		if err != nil {
			t.Fatalf(
				"share %d: %v",
				shareID,
				err,
			)
		}

		shares = append(
			shares,
			share,
		)
	}

	return shares, master
}

func propertyVerifiedPartials(
	t *testing.T,
	shares []*SecretShare,
	message []byte,
) []VerifiedPartialSignature {
	t.Helper()

	verified := make(
		[]VerifiedPartialSignature,
		0,
		len(shares),
	)

	for _, share := range shares {
		verificationShare, err :=
			share.VerificationShare()
		if err != nil {
			t.Fatalf(
				"verification share %d: %v",
				share.ID(),
				err,
			)
		}

		partial, err := share.SignPartial(
			message,
		)
		if err != nil {
			t.Fatalf(
				"partial %d: %v",
				share.ID(),
				err,
			)
		}

		checked, err := VerifyPartial(
			verificationShare,
			message,
			partial,
		)
		if err != nil {
			t.Fatalf(
				"verify partial %d: %v",
				share.ID(),
				err,
			)
		}

		verified = append(
			verified,
			checked,
		)
	}

	return verified
}

func propertyReversePartials(
	input []VerifiedPartialSignature,
) []VerifiedPartialSignature {
	out := append(
		[]VerifiedPartialSignature(nil),
		input...,
	)

	for left, right := 0, len(out)-1; left < right; left, right = left+1, right-1 {
		out[left], out[right] =
			out[right], out[left]
	}

	return out
}

func propertyAssertReconstruction(
	t *testing.T,
	masterPublic PublicKey,
	message []byte,
	partials []VerifiedPartialSignature,
	threshold int,
	expectedSignature Signature,
	expectedRandomness [32]byte,
) {
	t.Helper()

	combined, err := CombineVerifiedPartials(
		masterPublic,
		message,
		partials,
		threshold,
	)
	if err != nil {
		t.Fatalf(
			"combine failed: %v",
			err,
		)
	}

	if combined != expectedSignature {
		t.Fatal(
			"threshold signature differs from direct master signature",
		)
	}

	randomness, err := VerifyAndDeriveRandomness(
		masterPublic,
		message,
		combined,
	)
	if err != nil {
		t.Fatalf(
			"randomness derivation failed: %v",
			err,
		)
	}

	if randomness != expectedRandomness {
		t.Fatal(
			"threshold randomness differs from direct master randomness",
		)
	}
}

func TestThresholdPropertyAcrossConfigurations(
	t *testing.T,
) {
	cases := []struct {
		name      string
		threshold int
		shareIDs  []uint64
		seed      uint64
	}{
		{
			name:      "1-of-1",
			threshold: 1,
			shareIDs: []uint64{
				1,
			},
			seed: 1,
		},
		{
			name:      "1-of-5-unsorted",
			threshold: 1,
			shareIDs: []uint64{
				9,
				1,
				7,
				3,
				5,
			},
			seed: 2,
		},
		{
			name:      "2-of-3",
			threshold: 2,
			shareIDs: []uint64{
				9,
				2,
				5,
			},
			seed: 3,
		},
		{
			name:      "3-of-5",
			threshold: 3,
			shareIDs: []uint64{
				1,
				2,
				3,
				4,
				5,
			},
			seed: 4,
		},
		{
			name:      "4-of-7-even-ids",
			threshold: 4,
			shareIDs: []uint64{
				2,
				4,
				6,
				8,
				10,
				12,
				14,
			},
			seed: 5,
		},
		{
			name:      "5-of-8-irregular",
			threshold: 5,
			shareIDs: []uint64{
				101,
				7,
				55,
				2,
				89,
				34,
				13,
				144,
			},
			seed: 6,
		},
		{
			name:      "3-of-5-large-share-ids",
			threshold: 3,
			shareIDs: []uint64{
				math.MaxUint64,
				math.MaxUint64 - 2,
				uint64(1) << 63,
				17,
				3,
			},
			seed: 7,
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			message := []byte(
				"rabbit-vrf-property-" + tc.name,
			)

			shares, master :=
				propertyThresholdShares(
					t,
					tc.threshold,
					tc.shareIDs,
					tc.seed,
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

			firstSubset := append(
				[]VerifiedPartialSignature(nil),
				verified[:tc.threshold]...,
			)

			lastSubset := append(
				[]VerifiedPartialSignature(nil),
				verified[len(verified)-tc.threshold:]...,
			)

			reversedFirst :=
				propertyReversePartials(
					firstSubset,
				)

			reversedAll :=
				propertyReversePartials(
					verified,
				)

			propertyAssertReconstruction(
				t,
				masterPublic,
				message,
				firstSubset,
				tc.threshold,
				directSignature,
				directRandomness,
			)

			propertyAssertReconstruction(
				t,
				masterPublic,
				message,
				lastSubset,
				tc.threshold,
				directSignature,
				directRandomness,
			)

			propertyAssertReconstruction(
				t,
				masterPublic,
				message,
				reversedFirst,
				tc.threshold,
				directSignature,
				directRandomness,
			)

			propertyAssertReconstruction(
				t,
				masterPublic,
				message,
				reversedAll,
				tc.threshold,
				directSignature,
				directRandomness,
			)

			if tc.threshold > 1 {
				insufficient := verified[:tc.threshold-1]

				if _, err :=
					CombineVerifiedPartials(
						masterPublic,
						message,
						insufficient,
						tc.threshold,
					); !errors.Is(
					err,
					ErrInsufficientPartials,
				) {
					t.Fatalf(
						"insufficient shares accepted: %v",
						err,
					)
				}
			}
		})
	}
}

func TestThresholdPropertyRejectsDuplicateValidToken(
	t *testing.T,
) {
	shareIDs := []uint64{
		11,
		22,
		33,
		44,
		55,
	}

	shares, master := propertyThresholdShares(
		t,
		3,
		shareIDs,
		91,
	)

	message := []byte(
		"rabbit-vrf-property-duplicate",
	)

	masterPublic, err := master.PublicKey()
	if err != nil {
		t.Fatal(err)
	}

	verified := propertyVerifiedPartials(
		t,
		shares,
		message,
	)

	duplicate := []VerifiedPartialSignature{
		verified[0],
		verified[1],
		verified[1],
		verified[2],
	}

	if _, err := CombineVerifiedPartials(
		masterPublic,
		message,
		duplicate,
		3,
	); !errors.Is(
		err,
		ErrDuplicateShareID,
	) {
		t.Fatalf(
			"duplicate valid token accepted: %v",
			err,
		)
	}
}

func TestThresholdPropertyRejectsInvalidThresholds(
	t *testing.T,
) {
	shares, master := propertyThresholdShares(
		t,
		3,
		[]uint64{
			1,
			2,
			3,
			4,
			5,
		},
		123,
	)

	message := []byte(
		"rabbit-vrf-property-invalid-threshold",
	)

	masterPublic, err := master.PublicKey()
	if err != nil {
		t.Fatal(err)
	}

	verified := propertyVerifiedPartials(
		t,
		shares,
		message,
	)

	if _, err := CombineVerifiedPartials(
		masterPublic,
		message,
		verified,
		0,
	); !errors.Is(
		err,
		ErrInvalidThreshold,
	) {
		t.Fatalf(
			"threshold zero accepted: %v",
			err,
		)
	}

	if _, err := CombineVerifiedPartials(
		masterPublic,
		message,
		verified,
		len(verified)+1,
	); !errors.Is(
		err,
		ErrInsufficientPartials,
	) {
		t.Fatalf(
			"threshold above available shares accepted: %v",
			err,
		)
	}
}
