//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"math/big"
	"path/filepath"
	"testing"

	bls12381 "github.com/consensys/gnark-crypto/ecc/bls12-381"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	rabbitvrf "github.com/ethereum/go-ethereum/crypto/rabbitvrf"
	rabbitvrfstate "github.com/ethereum/go-ethereum/internal/rabbitvrfstate"
)

func TestRabbitVRFDKGFinalKeysetWiringWaitCompleteRestart(t *testing.T) {
	session, err := lqc.NewRabbitVRFDKGSessionContextV1(
		big.NewInt(9280), 11, common.HexToHash("0xaaaa"), 3,
	)
	if err != nil {
		t.Fatal(err)
	}
	id, err := lqc.RabbitVRFDKGSessionIDV1(session)
	if err != nil {
		t.Fatal(err)
	}
	members := make([]lqc.RabbitVRFCommitteeMemberV1, 3)
	commitments := make([]lqc.RabbitVRFDKGPolynomialCommitmentV1, 3)
	for i := range members {
		members[i] = lqc.RabbitVRFCommitteeMemberV1{
			ShareID:     uint64(i + 1),
			Participant: common.BigToAddress(big.NewInt(int64(i + 1))),
		}
		coefficients := make([]rabbitvrf.DKGCoefficientCommitmentV1, session.Threshold)
		for j := range coefficients {
			var point bls12381.G1Affine
			point.ScalarMultiplicationBase(big.NewInt(int64(i + j + 1)))
			encoded := point.Bytes()
			copy(coefficients[j][:], encoded[:])
		}
		commitments[i], _, err = lqc.NewRabbitVRFDKGPolynomialCommitmentV1(
			session, uint64(i+1), coefficients,
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	backend := &Ethereum{vrfDKGInstanceDir: t.TempDir()}
	runtime := &rabbitVRFDKGRuntime{
		backend: backend,
		current: rabbitVRFDKGLocalContextV1{
			SessionID: id, CanonicalSession: session,
			CanonicalMembers: members, Members: members[:1],
			PolynomialCommitments: commitments[:2],
		},
	}
	backend.vrfDKGTransport = &rabbitVRFDKGTransport{runtime: runtime}

	if ready, err := runtime.ensureFinalKeysetV1(); ready || err != nil {
		t.Fatalf("not-synced runtime: ready=%v err=%v", ready, err)
	}
	runtime.secretReady = true
	if ready, err := runtime.ensureFinalKeysetV1(); ready || err != nil {
		t.Fatalf("incomplete commitments: ready=%v err=%v", ready, err)
	}
	runtime.current.PolynomialCommitments = commitments
	if ready, err := runtime.ensureFinalKeysetV1(); !ready || err != nil {
		t.Fatalf("complete commitments: ready=%v err=%v", ready, err)
	}
	store, err := rabbitvrfstate.NewDKGFinalKeysetStoreV1(
		filepath.Join(backend.vrfDKGInstanceDir, "rabbit-vrf", "dkg-final-keysets"),
	)
	if err != nil {
		t.Fatal(err)
	}
	original, err := store.Load(session)
	if err != nil {
		t.Fatal(err)
	}
	restarted := &rabbitVRFDKGRuntime{
		backend:     &Ethereum{vrfDKGInstanceDir: backend.vrfDKGInstanceDir},
		secretReady: true,
		current: rabbitVRFDKGLocalContextV1{
			SessionID: id, CanonicalSession: session,
			CanonicalMembers: members, Members: members[:1],
		},
	}
	if ready, err := restarted.ensureFinalKeysetV1(); !ready || err != nil {
		t.Fatalf("restart recovery: ready=%v err=%v", ready, err)
	}
	recovered, err := store.Load(session)
	if err != nil || recovered.KeysetRoot != original.KeysetRoot {
		t.Fatal("restart changed persisted keyset")
	}
}
