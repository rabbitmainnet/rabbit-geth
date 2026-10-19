//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"errors"
	"math/big"
	"path/filepath"
	"testing"

	bls12381 "github.com/consensys/gnark-crypto/ecc/bls12-381"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	rabbitvrf "github.com/ethereum/go-ethereum/crypto/rabbitvrf"
	rabbitvrfstate "github.com/ethereum/go-ethereum/internal/rabbitvrfstate"
)

func TestRabbitVRFDKGFinalKeysetPersistenceCompleteRestartConflict(t *testing.T) {
	session, err := lqc.NewRabbitVRFDKGSessionContextV1(
		big.NewInt(9280), 11, common.HexToHash("0xaaaa"), 3,
	)
	if err != nil {
		t.Fatal(err)
	}
	coefficient := func(value uint64) rabbitvrf.DKGCoefficientCommitmentV1 {
		var point bls12381.G1Affine
		point.ScalarMultiplicationBase(new(big.Int).SetUint64(value))
		encoded := point.Bytes()
		var out rabbitvrf.DKGCoefficientCommitmentV1
		copy(out[:], encoded[:])
		return out
	}
	makeCommitments := func(offset uint64) []lqc.RabbitVRFDKGPolynomialCommitmentV1 {
		out := make([]lqc.RabbitVRFDKGPolynomialCommitmentV1, session.CommitteeSize)
		for index := range out {
			coefficients := make([]rabbitvrf.DKGCoefficientCommitmentV1, session.Threshold)
			for j := range coefficients {
				coefficients[j] = coefficient(uint64(j + 1))
			}
			coefficients[0] = coefficient(uint64(index+1) + offset)
			commitment, _, err := lqc.NewRabbitVRFDKGPolynomialCommitmentV1(
				session, uint64(index+1), coefficients,
			)
			if err != nil {
				t.Fatal(err)
			}
			out[index] = commitment
		}
		return out
	}

	instance := t.TempDir()
	commitments := makeCommitments(0)
	if _, err := persistRabbitVRFDKGFinalKeysetV1(
		instance, session, commitments[:2],
	); err == nil {
		t.Fatal("incomplete dealer set accepted")
	}
	store, err := rabbitvrfstate.NewDKGFinalKeysetStoreV1(
		filepath.Join(instance, "rabbit-vrf", "dkg-final-keysets"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(session); err == nil {
		t.Fatal("incomplete keyset persisted")
	}

	first, err := persistRabbitVRFDKGFinalKeysetV1(instance, session, commitments)
	if err != nil {
		t.Fatal(err)
	}
	restarted, err := persistRabbitVRFDKGFinalKeysetV1(instance, session, commitments)
	if err != nil {
		t.Fatal(err)
	}
	if first.KeysetRoot != restarted.KeysetRoot ||
		first.TranscriptRoot != restarted.TranscriptRoot ||
		first.ThresholdPublicKey != restarted.ThresholdPublicKey {
		t.Fatal("restart changed finalized keyset")
	}
	if _, err := persistRabbitVRFDKGFinalKeysetV1(
		instance, session, makeCommitments(10),
	); !errors.Is(err, rabbitvrfstate.ErrDKGFinalKeysetStoreConflictV1) {
		t.Fatalf("expected conflict rejection, got %v", err)
	}
	preserved, err := store.Load(session)
	if err != nil || preserved.KeysetRoot != first.KeysetRoot {
		t.Fatal("conflicting keyset replaced persisted keyset")
	}
}
