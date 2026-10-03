//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"math/big"
	"os"
	"path/filepath"
	"testing"

	bls12381 "github.com/consensys/gnark-crypto/ecc/bls12-381"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	rabbitvrf "github.com/ethereum/go-ethereum/crypto/rabbitvrf"
	"github.com/ethereum/go-ethereum/eth/downloader"
	"github.com/ethereum/go-ethereum/eth/ethconfig"
	rabbitvrfstate "github.com/ethereum/go-ethereum/internal/rabbitvrfstate"
)

func TestRabbitVRFDKGLocalShareWaitAggregateRestart(t *testing.T) {
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
	members := make([]lqc.RabbitVRFCommitteeMemberV1, session.CommitteeSize)
	commitments := make([]lqc.RabbitVRFDKGPolynomialCommitmentV1, session.CommitteeSize)
	for i := range members {
		members[i] = lqc.RabbitVRFCommitteeMemberV1{
			ShareID:     uint64(i + 1),
			TicketHash:  common.BigToHash(big.NewInt(int64(i + 1))),
			Participant: common.BigToAddress(big.NewInt(int64(i + 1))),
		}
		coefficients := make([]rabbitvrf.DKGCoefficientCommitmentV1, session.Threshold)
		var point bls12381.G1Affine
		point.ScalarMultiplicationBase(big.NewInt(1))
		encoded := point.Bytes()
		for j := range coefficients {
			copy(coefficients[j][:], encoded[:])
		}
		commitments[i], _, err = lqc.NewRabbitVRFDKGPolynomialCommitmentV1(
			session, uint64(i+1), coefficients,
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	dir := t.TempDir()
	if _, err := persistRabbitVRFDKGFinalKeysetV1(dir, session, commitments); err != nil {
		t.Fatal(err)
	}
	passwordFile := filepath.Join(dir, "test-password")
	const password = "test-only-password"
	if err := os.WriteFile(passwordFile, []byte(password), 0600); err != nil {
		t.Fatal(err)
	}
	runtime := &rabbitVRFDKGRuntime{
		backend: &Ethereum{
			vrfDKGInstanceDir: dir,
			config:            &ethconfig.Config{RabbitVRFDKGPasswordFile: passwordFile},
			handler:           &handler{downloader: &downloader.Downloader{}},
		},
		secretReady: true,
		current: rabbitVRFDKGLocalContextV1{
			SessionID: id, CanonicalSession: session,
			CanonicalMembers: members, Members: members[:1],
		},
	}
	if ready, err := runtime.ensureLocalSecretSharesV1(); ready || err != nil {
		t.Fatalf("missing evaluations: ready=%v err=%v", ready, err)
	}
	evaluationDir := filepath.Join(dir, "rabbit-vrf", "dkg-evaluations")
	store, err := rabbitvrfstate.NewDKGVerifiedEvaluationStoreV1(evaluationDir, 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	raw := make([]byte, rabbitvrf.DKGPolynomialEvaluationSizeV1)
	raw[len(raw)-1] = byte(session.Threshold)
	value, err := rabbitvrf.DKGPolynomialEvaluationV1FromBytes(raw)
	if err != nil {
		t.Fatal(err)
	}
	for i, dealer := range members {
		if err := store.Store(session, members[0], dealer, value, password); err != nil {
			t.Fatal(err)
		}
		ready, err := runtime.ensureLocalSecretSharesV1()
		if err != nil || ready != (i == len(members)-1) {
			t.Fatalf("evaluations=%d ready=%v err=%v", i+1, ready, err)
		}
	}

	runtime.backend.config.RabbitVRFDKGPasswordFile = ""
	if ready, err := runtime.ensureLocalSecretSharesV1(); !ready || err != nil {
		t.Fatalf("ready cache unnecessarily reread password/share: ready=%v err=%v", ready, err)
	}
	runtime.syncing = true
	if ready, err := runtime.ensureLocalSecretSharesV1(); ready || err != nil {
		t.Fatalf("cache bypassed synchronization guard: ready=%v err=%v", ready, err)
	}
	runtime.syncing = false
	runtime.current.Members = members[1:2]
	if ready, err := runtime.ensureLocalSecretSharesV1(); ready || err == nil {
		t.Fatalf("changed local members incorrectly reused cache: ready=%v err=%v", ready, err)
	}
	runtime.current.Members = members[:1]
	runtime.backend.config.RabbitVRFDKGPasswordFile = passwordFile
	shares, err := rabbitvrfstate.NewStandardDKGSecretShareStoreV1(
		filepath.Join(dir, "rabbit-vrf", "dkg-secret-shares"),
	)
	if err != nil {
		t.Fatal(err)
	}
	share, err := shares.Load(session, members[0], password)
	if err != nil {
		t.Fatal(err)
	}
	publicKey, err := share.PublicKey()
	if err != nil {
		t.Fatal(err)
	}
	// Delete only this test's temporary evaluations to prove persisted-share recovery.
	if err := os.RemoveAll(evaluationDir); err != nil {
		t.Fatal(err)
	}
	restarted := &rabbitVRFDKGRuntime{
		backend:     runtime.backend,
		secretReady: true,
		current:     runtime.currentContext(),
	}
	if ready, err := restarted.ensureLocalSecretSharesV1(); !ready || err != nil {
		t.Fatalf("restart: ready=%v err=%v", ready, err)
	}
	recovered, err := shares.Load(session, members[0], password)
	if err != nil {
		t.Fatal(err)
	}
	recoveredPublic, err := recovered.PublicKey()
	if err != nil || recoveredPublic != publicKey {
		t.Fatal("restart changed secret share")
	}
}
