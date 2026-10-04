package rabbitvrf

import (
	"fmt"
	"testing"
)

// Real random dealer polynomials, verified evaluations, assembled private
// shares and recovered signatures. No live wallets or stored secrets are used.
func TestDKGPopulationFlowV2(t *testing.T) {
	for _, n := range []int{1, 2, 3, 4, 8} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			threshold := n - (n-1)/3
			polynomials := make([]*DKGDealerPolynomialV1, n)
			commitments := make([][]DKGCoefficientCommitmentV1, n)
			constants := make([]DKGCoefficientCommitmentV1, n)
			for i := range polynomials {
				polynomial, err := GenerateDKGDealerPolynomialV1(uint64(threshold))
				if err != nil {
					t.Fatal(err)
				}
				polynomials[i] = polynomial
				defer polynomial.Destroy()
				commitments[i], err = polynomial.Commitments()
				if err != nil {
					t.Fatal(err)
				}
				constants[i] = commitments[i][0]
			}
			key, err := AggregateDKGConstantCommitmentsV1(constants)
			if err != nil {
				t.Fatal(err)
			}
			message := []byte("rabbit-vrf-dkg-population-flow-v2")
			verified := make([]VerifiedPartialSignature, n)
			for recipient := 0; recipient < n; recipient++ {
				id := uint64(recipient + 1)
				evaluations := make([]DKGPolynomialEvaluationV1, n)
				for dealer, polynomial := range polynomials {
					evaluations[dealer], err = polynomial.Evaluate(id)
					if err != nil {
						t.Fatal(err)
					}
					if err := VerifyDKGPolynomialEvaluationV1(id, evaluations[dealer], commitments[dealer]); err != nil {
						t.Fatal(err)
					}
				}
				share, err := AggregateDKGPolynomialEvaluationsV1(id, evaluations)
				if err != nil {
					t.Fatal(err)
				}
				public, err := share.PublicKey()
				if err != nil {
					t.Fatal(err)
				}
				expected, err := DKGVerificationPublicKeyV1(id, commitments)
				if err != nil || expected != public {
					t.Fatal("assembled private share does not match public commitments")
				}
				verification, err := share.VerificationShare()
				if err != nil {
					t.Fatal(err)
				}
				partial, err := share.SignPartial(message)
				if err != nil {
					t.Fatal(err)
				}
				verified[recipient], err = VerifyPartial(verification, message, partial)
				if err != nil {
					t.Fatal(err)
				}
			}
			signature, err := CombineVerifiedPartials(key, message, verified[:threshold], threshold)
			if err != nil {
				t.Fatal(err)
			}
			if err := Verify(key, message, signature); err != nil {
				t.Fatal(err)
			}
			if _, err := CombineVerifiedPartials(key, message, verified[:threshold-1], threshold); err == nil {
				t.Fatal("accepted too few signature shares")
			}
			// A different available subset after DKG must give the same signature.
			other, err := CombineVerifiedPartials(key, message, verified[n-threshold:], threshold)
			if err != nil || other != signature {
				t.Fatal("signer availability changed the recovered signature")
			}
		})
	}
}
