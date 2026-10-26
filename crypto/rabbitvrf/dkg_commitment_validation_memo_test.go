package rabbitvrf

import (
	"errors"
	"sync"
	"testing"

	bls12381 "github.com/consensys/gnark-crypto/ecc/bls12-381"
)

func TestDKGCommitmentValidationMemoV1(t *testing.T) {
	m := dkgCommitmentMemoV1{capacity: 4}
	var points []DKGCoefficientCommitmentV1
	for i := byte(1); i <= 6; i++ {
		public, err := fixedSecretKey(t, i).PublicKey()
		if err != nil {
			t.Fatal(err)
		}
		points = append(points, DKGCoefficientCommitmentV1(public))
	}

	for _, point := range points {
		for _, allow := range []bool{false, true} {
			want := validateDKGCoefficientCommitmentUncachedV1(point, allow)
			for repeat := 0; repeat < 2; repeat++ {
				got := m.validate(point, allow)
				if !errors.Is(got, want) {
					t.Fatal("validation mismatch")
				}
			}
		}
	}
	if len(m.known) != 4 || len(m.order) != 4 {
		t.Fatal("cache bound violated")
	}
	if _, exists := m.known[dkgCommitmentMemoKeyV1{points[0], false}]; exists {
		t.Fatal("old entry not evicted")
	}

	var infinityPoint bls12381.G1Affine
	infinity := DKGCoefficientCommitmentV1(infinityPoint.Bytes())
	if err := m.validate(infinity, true); err != nil {
		t.Fatal("canonical infinity should be allowed", err)
	}
	if err := m.validate(infinity, false); err == nil {
		t.Fatal("allowInfinity cache contamination")
	}

	invalid := []DKGCoefficientCommitmentV1{{}}
	var malformed DKGCoefficientCommitmentV1
	for i := range malformed {
		malformed[i] = 255
	}
	invalid = append(invalid, malformed)
	before := len(m.known)
	for _, point := range invalid {
		for _, allow := range []bool{false, true} {
			want := validateDKGCoefficientCommitmentUncachedV1(point, allow)
			got := m.validate(point, allow)
			if want == nil || !errors.Is(got, want) {
				t.Fatal("invalid commitment accepted or error changed")
			}
		}
	}
	if len(m.known) != before {
		t.Fatal("invalid input cached")
	}

	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 30; j++ {
				if err := m.validate(points[j%len(points)], false); err != nil {
					t.Error(err)
				}
			}
		}()
	}
	wg.Wait()
}
