package eth

import (
	"testing"

	"github.com/consensys/gnark-crypto/ecc/bls12-381/fr"

	"github.com/ethereum/go-ethereum/consensus/lqc"
	rabbitvrf "github.com/ethereum/go-ethereum/crypto/rabbitvrf"
	"github.com/ethereum/go-ethereum/internal/rabbitvrfstate"
)

func rabbitVRFTestSecretShareV1(t *testing.T, shareID uint64, scalar uint64) *rabbitvrf.SecretShare {
	t.Helper()
	var value fr.Element
	value.SetUint64(scalar)
	encoded := value.Bytes()
	share, err := rabbitvrf.SecretShareFromBytes(shareID, encoded[:])
	if err != nil {
		t.Fatal(err)
	}
	return share
}

func TestRabbitVRFThresholdRuntimeHelpersV1(t *testing.T) {
	// f(x) = 7 + 3x, threshold = 2.
	master := rabbitVRFTestSecretShareV1(t, 99, 7)
	thresholdPublicKey, err := master.PublicKey()
	if err != nil {
		t.Fatal(err)
	}

	shares := []*rabbitvrf.SecretShare{
		rabbitVRFTestSecretShareV1(t, 1, 10),
		rabbitVRFTestSecretShareV1(t, 2, 13),
		rabbitVRFTestSecretShareV1(t, 3, 16),
	}

	verificationShares := make([]lqc.RabbitVRFVerificationShareV1, len(shares))
	for index, share := range shares {
		publicKey, err := share.PublicKey()
		if err != nil {
			t.Fatal(err)
		}
		verificationShares[index] = lqc.RabbitVRFVerificationShareV1{
			ShareID:   uint64(index + 1),
			PublicKey: publicKey,
		}
	}

	keyset := rabbitvrfstate.DKGFinalKeysetV1{
		ThresholdPublicKey: thresholdPublicKey,
		VerificationShares: verificationShares,
	}

	message := []byte("rabbit-vrf-threshold-runtime-v1")

	partials := make([]rabbitvrf.PartialSignature, 0, 2)
	for index := 0; index < 2; index++ {
		partial, verified, err := rabbitVRFSignThresholdPartialWithKeysetV1(
			shares[index],
			uint64(index+1),
			keyset,
			message,
		)
		if err != nil {
			t.Fatal(err)
		}
		if partial.ShareID != uint64(index+1) || verified.ShareID() != uint64(index+1) {
			t.Fatalf("unexpected partial share id %d", partial.ShareID)
		}
		partials = append(partials, partial)
	}

	signature, randomness, err := rabbitVRFCombineThresholdPartialsWithKeysetV1(
		2,
		keyset,
		message,
		partials,
	)
	if err != nil {
		t.Fatal(err)
	}
	if randomness == ([32]byte{}) {
		t.Fatal("zero randomness")
	}
	expectedRandomness, err := rabbitvrf.VerifyAndDeriveRandomness(
		thresholdPublicKey,
		message,
		signature,
	)
	if err != nil {
		t.Fatal(err)
	}
	if randomness != expectedRandomness {
		t.Fatal("runtime randomness mismatch")
	}

	if _, _, err := rabbitVRFCombineThresholdPartialsWithKeysetV1(
		2,
		keyset,
		message,
		partials[:1],
	); err == nil {
		t.Fatal("insufficient partials accepted")
	}

	invalidShare := partials[0]
	invalidShare.ShareID = 99
	if _, _, err := rabbitVRFCombineThresholdPartialsWithKeysetV1(
		2,
		keyset,
		message,
		[]rabbitvrf.PartialSignature{invalidShare, partials[1]},
	); err == nil {
		t.Fatal("partial outside finalized keyset accepted")
	}

	if _, _, err := rabbitVRFCombineThresholdPartialsWithKeysetV1(
		2,
		keyset,
		[]byte("different-message"),
		partials,
	); err == nil {
		t.Fatal("partials accepted for different message")
	}

	if _, _, err := rabbitVRFSignThresholdPartialWithKeysetV1(
		shares[0],
		2,
		keyset,
		message,
	); err == nil {
		t.Fatal("secret share accepted under wrong ShareID")
	}
}
