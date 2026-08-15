package lqc

import (
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

var errRelayEligibilityTestV1 = errors.New("relay eligibility test rejection")

func relayCandidateV1(
	t *testing.T,
	chainID *big.Int,
	epoch uint64,
	anchor common.Hash,
	identitySeed int,
	nonce uint64,
	proofHash common.Hash,
) WorkCommitCandidateV1 {
	t.Helper()

	private, err := crypto.ToECDSA(
		commitTestKeyV1(
			t,
			identitySeed,
		).FillBytes(make([]byte, 32)),
	)
	if err != nil {
		t.Fatal(err)
	}

	ticket := RandomXWorkTicketV1{
		Version:     RandomXWorkProtocolVersion,
		Epoch:       epoch,
		Participant: crypto.PubkeyToAddress(private.PublicKey),
		Nonce:       nonce,
	}

	signingHash, err := RandomXWorkSigningHashV1(
		chainID,
		anchor,
		ticket,
		proofHash,
	)
	if err != nil {
		t.Fatal(err)
	}

	signature, err := crypto.Sign(signingHash[:], private)
	if err != nil {
		t.Fatal(err)
	}

	return WorkCommitCandidateV1{
		Signed: SignedRandomXWorkTicketV1{
			Ticket:    ticket,
			Signature: signature,
		},
		ProofHash: proofHash,
	}
}

func relayGoodProofV1(index byte) common.Hash {
	var raw [32]byte
	raw[31] = index
	if index == 0 {
		raw[31] = 1
	}
	return common.BytesToHash(raw[:])
}

func relayAlwaysEligibleV1(
	common.Address,
) error {
	return nil
}

func TestWorkRelayV1RejectsBadTargetBeforeRandomX(t *testing.T) {
	chainID := big.NewInt(928)
	anchor := common.HexToHash("0x1234")
	difficulty := big.NewInt(8)

	var badRaw [32]byte
	for i := range badRaw {
		badRaw[i] = 0xff
	}
	badProof := common.BytesToHash(badRaw[:])

	candidate := relayCandidateV1(
		t,
		chainID,
		7,
		anchor,
		1,
		1,
		badProof,
	)

	hashCalls := 0
	hasher := func(
		common.Hash,
		[]byte,
	) (common.Hash, error) {
		hashCalls++
		return badProof, nil
	}

	pool := NewWorkCommitPoolV1(16)
	if err := pool.ResetCommitEpochV1(7); err != nil {
		t.Fatal(err)
	}
	limiter, err := NewWorkRelayVerificationLimiterV1(1)
	if err != nil {
		t.Fatal(err)
	}

	err = ValidateAndAdmitRelayedWorkV1(
		chainID,
		7,
		anchor,
		difficulty,
		candidate,
		relayAlwaysEligibleV1,
		hasher,
		limiter,
		pool,
	)
	if err != ErrWorkRelayClaimTargetV1 {
		t.Fatalf("error = %v", err)
	}
	if hashCalls != 0 {
		t.Fatalf("RandomX called %d times before target reject", hashCalls)
	}
}

func TestWorkRelayV1RejectsBadSignatureBeforeRandomX(t *testing.T) {
	chainID := big.NewInt(928)
	anchor := common.HexToHash("0x1234")
	difficulty := big.NewInt(8)

	candidate := relayCandidateV1(
		t,
		chainID,
		7,
		anchor,
		1,
		1,
		relayGoodProofV1(1),
	)
	candidate.Signed.Signature[10] ^= 0x01

	hashCalls := 0
	hasher := func(
		common.Hash,
		[]byte,
	) (common.Hash, error) {
		hashCalls++
		return candidate.ProofHash, nil
	}

	pool := NewWorkCommitPoolV1(16)
	if err := pool.ResetCommitEpochV1(7); err != nil {
		t.Fatal(err)
	}
	limiter, err := NewWorkRelayVerificationLimiterV1(1)
	if err != nil {
		t.Fatal(err)
	}

	err = ValidateAndAdmitRelayedWorkV1(
		chainID,
		7,
		anchor,
		difficulty,
		candidate,
		relayAlwaysEligibleV1,
		hasher,
		limiter,
		pool,
	)
	if err == nil {
		t.Fatal("bad signature was accepted")
	}
	if hashCalls != 0 {
		t.Fatalf("RandomX called %d times before signature reject", hashCalls)
	}
}

func TestWorkRelayV1RejectsIneligibleBeforeRandomX(t *testing.T) {
	chainID := big.NewInt(928)
	anchor := common.HexToHash("0x1234")
	difficulty := big.NewInt(8)

	candidate := relayCandidateV1(
		t,
		chainID,
		7,
		anchor,
		1,
		1,
		relayGoodProofV1(1),
	)

	hashCalls := 0
	hasher := func(
		common.Hash,
		[]byte,
	) (common.Hash, error) {
		hashCalls++
		return candidate.ProofHash, nil
	}

	pool := NewWorkCommitPoolV1(16)
	if err := pool.ResetCommitEpochV1(7); err != nil {
		t.Fatal(err)
	}
	limiter, err := NewWorkRelayVerificationLimiterV1(1)
	if err != nil {
		t.Fatal(err)
	}

	err = ValidateAndAdmitRelayedWorkV1(
		chainID,
		7,
		anchor,
		difficulty,
		candidate,
		func(common.Address) error {
			return errRelayEligibilityTestV1
		},
		hasher,
		limiter,
		pool,
	)
	if err != errRelayEligibilityTestV1 {
		t.Fatalf("error = %v", err)
	}
	if hashCalls != 0 {
		t.Fatalf("RandomX called %d times before eligibility reject", hashCalls)
	}
}

func TestWorkRelayV1KnownCandidateRejectedBeforeRandomX(t *testing.T) {
	chainID := big.NewInt(928)
	anchor := common.HexToHash("0x1234")
	difficulty := big.NewInt(8)

	candidate := relayCandidateV1(
		t,
		chainID,
		7,
		anchor,
		1,
		1,
		relayGoodProofV1(1),
	)

	pool := NewWorkCommitPoolV1(16)
	if err := pool.ResetCommitEpochV1(7); err != nil {
		t.Fatal(err)
	}
	if err := pool.AddVerifiedV1(candidate); err != nil {
		t.Fatal(err)
	}

	limiter, err := NewWorkRelayVerificationLimiterV1(1)
	if err != nil {
		t.Fatal(err)
	}

	hashCalls := 0
	err = ValidateAndAdmitRelayedWorkV1(
		chainID,
		7,
		anchor,
		difficulty,
		candidate,
		relayAlwaysEligibleV1,
		func(common.Hash, []byte) (common.Hash, error) {
			hashCalls++
			return candidate.ProofHash, nil
		},
		limiter,
		pool,
	)
	if err != ErrWorkRelayAlreadyKnownV1 {
		t.Fatalf("error = %v", err)
	}
	if hashCalls != 0 {
		t.Fatalf("RandomX called %d times for known candidate", hashCalls)
	}
}

func TestWorkRelayV1VerificationLimiterBoundsExpensiveWork(t *testing.T) {
	limiter, err := NewWorkRelayVerificationLimiterV1(2)
	if err != nil {
		t.Fatal(err)
	}

	if !limiter.TryAcquireV1() {
		t.Fatal("could not acquire first verification budget slot")
	}
	if !limiter.TryAcquireV1() {
		t.Fatal("could not acquire second verification budget slot")
	}
	if limiter.TryAcquireV1() {
		t.Fatal("verification limiter exceeded max in-flight")
	}
	if limiter.InFlightV1() != 2 {
		t.Fatalf("in-flight = %d, want 2", limiter.InFlightV1())
	}

	limiter.ReleaseV1()
	if !limiter.TryAcquireV1() {
		t.Fatal("verification budget did not reopen after release")
	}
	limiter.ReleaseV1()
	limiter.ReleaseV1()

	if limiter.InFlightV1() != 0 {
		t.Fatalf("in-flight = %d, want 0", limiter.InFlightV1())
	}
}

func TestWorkRelayV1BusyRejectsBeforeRandomX(t *testing.T) {
	chainID := big.NewInt(928)
	anchor := common.HexToHash("0x1234")
	difficulty := big.NewInt(8)
	candidate := relayCandidateV1(
		t,
		chainID,
		7,
		anchor,
		1,
		1,
		relayGoodProofV1(1),
	)

	pool := NewWorkCommitPoolV1(16)
	if err := pool.ResetCommitEpochV1(7); err != nil {
		t.Fatal(err)
	}
	limiter, err := NewWorkRelayVerificationLimiterV1(1)
	if err != nil {
		t.Fatal(err)
	}
	if !limiter.TryAcquireV1() {
		t.Fatal("could not occupy verification slot")
	}
	defer limiter.ReleaseV1()

	hashCalls := 0
	err = ValidateAndAdmitRelayedWorkV1(
		chainID,
		7,
		anchor,
		difficulty,
		candidate,
		relayAlwaysEligibleV1,
		func(common.Hash, []byte) (common.Hash, error) {
			hashCalls++
			return candidate.ProofHash, nil
		},
		limiter,
		pool,
	)
	if err != ErrWorkRelayVerificationBusyV1 {
		t.Fatalf("error = %v", err)
	}
	if hashCalls != 0 {
		t.Fatalf("RandomX called %d times while limiter busy", hashCalls)
	}
}

func TestWorkRelayV1RecomputedHashMustMatchClaim(t *testing.T) {
	chainID := big.NewInt(928)
	anchor := common.HexToHash("0x1234")
	difficulty := big.NewInt(8)

	candidate := relayCandidateV1(
		t,
		chainID,
		7,
		anchor,
		1,
		1,
		relayGoodProofV1(1),
	)

	pool := NewWorkCommitPoolV1(16)
	if err := pool.ResetCommitEpochV1(7); err != nil {
		t.Fatal(err)
	}
	limiter, err := NewWorkRelayVerificationLimiterV1(1)
	if err != nil {
		t.Fatal(err)
	}

	hashCalls := 0
	err = ValidateAndAdmitRelayedWorkV1(
		chainID,
		7,
		anchor,
		difficulty,
		candidate,
		relayAlwaysEligibleV1,
		func(common.Hash, []byte) (common.Hash, error) {
			hashCalls++
			return relayGoodProofV1(2), nil
		},
		limiter,
		pool,
	)
	if err != ErrWorkRelayClaimMismatchV1 {
		t.Fatalf("error = %v", err)
	}
	if hashCalls != 1 {
		t.Fatalf("RandomX calls = %d, want 1", hashCalls)
	}
	if pool.LenV1() != 0 {
		t.Fatal("mismatched proof entered pool")
	}
}

func TestWorkRelayV1SuccessfulAdmissionUsesOneRandomXCall(t *testing.T) {
	chainID := big.NewInt(928)
	anchor := common.HexToHash("0x1234")
	difficulty := big.NewInt(8)

	candidate := relayCandidateV1(
		t,
		chainID,
		7,
		anchor,
		1,
		1,
		relayGoodProofV1(1),
	)

	pool := NewWorkCommitPoolV1(16)
	if err := pool.ResetCommitEpochV1(7); err != nil {
		t.Fatal(err)
	}
	limiter, err := NewWorkRelayVerificationLimiterV1(1)
	if err != nil {
		t.Fatal(err)
	}

	hashCalls := 0
	err = ValidateAndAdmitRelayedWorkV1(
		chainID,
		7,
		anchor,
		difficulty,
		candidate,
		relayAlwaysEligibleV1,
		func(
			epochKey common.Hash,
			input []byte,
		) (common.Hash, error) {
			hashCalls++
			if epochKey == (common.Hash{}) {
				t.Fatal("zero epoch key")
			}
			if len(input) == 0 {
				t.Fatal("empty RandomX input")
			}
			return candidate.ProofHash, nil
		},
		limiter,
		pool,
	)
	if err != nil {
		t.Fatal(err)
	}
	if hashCalls != 1 {
		t.Fatalf("RandomX calls = %d, want 1", hashCalls)
	}
	if pool.LenV1() != 1 {
		t.Fatalf("pool len = %d, want 1", pool.LenV1())
	}
}

func TestWorkRelayV1RejectsWrongEpochBeforeRandomX(t *testing.T) {
	chainID := big.NewInt(928)
	anchor := common.HexToHash("0x1234")
	difficulty := big.NewInt(8)

	candidate := relayCandidateV1(
		t,
		chainID,
		6,
		anchor,
		1,
		1,
		relayGoodProofV1(1),
	)

	hashCalls := 0
	_, err := PrecheckRelayedWorkV1(
		chainID,
		7,
		anchor,
		difficulty,
		candidate,
		relayAlwaysEligibleV1,
	)
	if err != ErrWorkCommitEpochMismatchV1 {
		t.Fatalf("error = %v", err)
	}
	if hashCalls != 0 {
		t.Fatalf("RandomX calls = %d, want 0", hashCalls)
	}
}
