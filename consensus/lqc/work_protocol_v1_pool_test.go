package lqc

import (
	"bytes"
	"errors"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

var errWorkCommitPoolPersistenceTest = errors.New(
	"work commit pool persistence test failure",
)

type workCommitPoolPersistenceTest struct {
	epoch      uint64
	candidates []WorkCommitCandidateV1
	fail       bool
}

func (s *workCommitPoolPersistenceTest) LoadWorkCommitPoolV1(
	epoch uint64,
) ([]WorkCommitCandidateV1, error) {
	if s.fail {
		return nil, errWorkCommitPoolPersistenceTest
	}
	if s.epoch != epoch {
		return nil, nil
	}
	out := make([]WorkCommitCandidateV1, 0, len(s.candidates))
	for _, candidate := range s.candidates {
		out = append(out, cloneWorkCommitCandidateV1(candidate))
	}
	return out, nil
}

func (s *workCommitPoolPersistenceTest) StoreWorkCommitPoolV1(
	epoch uint64,
	candidates []WorkCommitCandidateV1,
) error {
	if s.fail {
		return errWorkCommitPoolPersistenceTest
	}
	s.epoch = epoch
	s.candidates = nil
	for _, candidate := range candidates {
		s.candidates = append(
			s.candidates,
			cloneWorkCommitCandidateV1(candidate),
		)
	}
	return nil
}

func TestWorkCommitPoolV1DeterministicPendingAcrossArrivalOrder(
	t *testing.T,
) {
	left := NewWorkCommitPoolV1(64)
	right := NewWorkCommitPoolV1(64)

	if err := left.ResetCommitEpochV1(7); err != nil {
		t.Fatal(err)
	}
	if err := right.ResetCommitEpochV1(7); err != nil {
		t.Fatal(err)
	}

	candidates := make([]WorkCommitCandidateV1, 0, 20)
	for i := 0; i < 20; i++ {
		candidates = append(candidates, commitCandidateV1(
			t,
			7,
			i+1,
			uint64(i+1),
			commitProofHashV1(i+1),
		))
	}

	for _, candidate := range candidates {
		if err := left.AddVerifiedV1(candidate); err != nil {
			t.Fatal(err)
		}
	}
	for i := len(candidates) - 1; i >= 0; i-- {
		if err := right.AddVerifiedV1(candidates[i]); err != nil {
			t.Fatal(err)
		}
	}

	pendingLeft, err := left.PendingV1()
	if err != nil {
		t.Fatal(err)
	}
	pendingRight, err := right.PendingV1()
	if err != nil {
		t.Fatal(err)
	}

	if len(pendingLeft) != int(MaxWorkTicketsPerBlockV1) ||
		len(pendingRight) != int(MaxWorkTicketsPerBlockV1) {
		t.Fatalf(
			"pending sizes = %d/%d",
			len(pendingLeft),
			len(pendingRight),
		)
	}

	for i := range pendingLeft {
		if pendingLeft[i].ProofHash != pendingRight[i].ProofHash {
			t.Fatalf("arrival order changed pending at %d", i)
		}
	}
}

func TestWorkCommitPoolV1HasNoPerWalletLaneOrQuota(
	t *testing.T,
) {
	pool := NewWorkCommitPoolV1(64)
	if err := pool.ResetCommitEpochV1(7); err != nil {
		t.Fatal(err)
	}

	// All 16 valid-work candidates belong to the SAME wallet.
	for i := 0; i < 16; i++ {
		candidate := commitCandidateV1(
			t,
			7,
			1,
			uint64(i+1),
			commitProofHashV1(i+1),
		)
		if err := pool.AddVerifiedV1(candidate); err != nil {
			t.Fatal(err)
		}
	}

	pending, err := pool.PendingV1()
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != int(MaxWorkTicketsPerBlockV1) {
		t.Fatalf(
			"pending = %d, want %d",
			len(pending),
			MaxWorkTicketsPerBlockV1,
		)
	}

	wantAddress := pending[0].Signed.Ticket.Participant
	for _, candidate := range pending {
		if candidate.Signed.Ticket.Participant != wantAddress {
			t.Fatal("same-wallet work was unexpectedly split/filtered")
		}
	}
}

func TestWorkCommitPoolV1IdentitySplitSameProofsSameBatch(
	t *testing.T,
) {
	one := NewWorkCommitPoolV1(64)
	split := NewWorkCommitPoolV1(64)
	if err := one.ResetCommitEpochV1(7); err != nil {
		t.Fatal(err)
	}
	if err := split.ResetCommitEpochV1(7); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 32; i++ {
		proof := commitProofHashV1(i + 1)

		if err := one.AddVerifiedV1(commitCandidateV1(
			t,
			7,
			1,
			uint64(i+1),
			proof,
		)); err != nil {
			t.Fatal(err)
		}

		if err := split.AddVerifiedV1(commitCandidateV1(
			t,
			7,
			1000+i,
			1,
			proof,
		)); err != nil {
			t.Fatal(err)
		}
	}

	left, err := one.PendingV1()
	if err != nil {
		t.Fatal(err)
	}
	right, err := split.PendingV1()
	if err != nil {
		t.Fatal(err)
	}

	if len(left) != len(right) {
		t.Fatal("identity split changed pending count")
	}
	for i := range left {
		if left[i].ProofHash != right[i].ProofHash {
			t.Fatalf(
				"identity split changed pending proof at %d",
				i,
			)
		}
	}
}

func TestWorkCommitPoolV1DuplicateAndCapacityRules(
	t *testing.T,
) {
	pool := NewWorkCommitPoolV1(2)
	if err := pool.ResetCommitEpochV1(7); err != nil {
		t.Fatal(err)
	}

	a := commitCandidateV1(
		t, 7, 1, 1, commitProofHashV1(1),
	)
	b := commitCandidateV1(
		t, 7, 2, 1, commitProofHashV1(2),
	)
	c := commitCandidateV1(
		t, 7, 3, 1, commitProofHashV1(3),
	)

	if err := pool.AddVerifiedV1(a); err != nil {
		t.Fatal(err)
	}
	if err := pool.AddVerifiedV1(b); err != nil {
		t.Fatal(err)
	}

	// Duplicate detection happens before "pool full".
	if err := pool.AddVerifiedV1(a); err != ErrDuplicateRandomXWorkHash {
		t.Fatalf("duplicate proof error = %v", err)
	}

	semanticDuplicate := cloneWorkCommitCandidateV1(a)
	semanticDuplicate.ProofHash = commitProofHashV1(99)
	if err := pool.AddVerifiedV1(
		semanticDuplicate,
	); err != ErrDuplicateWorkTicketV3 {
		t.Fatalf("semantic duplicate error = %v", err)
	}

	if err := pool.AddVerifiedV1(c); err != ErrWorkCommitPoolFullV1 {
		t.Fatalf("pool full error = %v", err)
	}
}

func TestWorkCommitPoolV1RemoveIncludedAndEpochReset(
	t *testing.T,
) {
	pool := NewWorkCommitPoolV1(64)
	if err := pool.ResetCommitEpochV1(7); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 12; i++ {
		if err := pool.AddVerifiedV1(commitCandidateV1(
			t,
			7,
			i+1,
			1,
			commitProofHashV1(i+1),
		)); err != nil {
			t.Fatal(err)
		}
	}

	pending, err := pool.PendingV1()
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 8 {
		t.Fatalf("pending = %d, want 8", len(pending))
	}

	remove := make([]common.Hash, 0, len(pending))
	for _, candidate := range pending {
		remove = append(remove, candidate.ProofHash)
	}

	if removed := pool.RemoveIncludedV1(remove); removed != 8 {
		t.Fatalf("removed = %d, want 8", removed)
	}
	if pool.LenV1() != 4 {
		t.Fatalf("len = %d, want 4", pool.LenV1())
	}

	if err := pool.ResetCommitEpochV1(8); err != nil {
		t.Fatal(err)
	}
	status := pool.StatusV1()
	if status.Epoch != 8 || status.Count != 0 {
		t.Fatalf("unexpected status after reset: %+v", status)
	}

	oldEpoch := commitCandidateV1(
		t,
		7,
		99,
		1,
		commitProofHashV1(99),
	)
	if err := pool.AddVerifiedV1(
		oldEpoch,
	); err != ErrWorkCommitEpochMismatchV1 {
		t.Fatalf("old epoch error = %v", err)
	}
}

func TestWorkCommitPoolV1PersistenceRestoresAndRollsBackFailures(
	t *testing.T,
) {
	store := new(workCommitPoolPersistenceTest)
	first := NewPersistentWorkCommitPoolV1(64, store)
	if err := first.ResetCommitEpochV1(7); err != nil {
		t.Fatal(err)
	}
	a := commitCandidateV1(
		t,
		7,
		1,
		1,
		commitProofHashV1(1),
	)
	b := commitCandidateV1(
		t,
		7,
		2,
		1,
		commitProofHashV1(2),
	)
	if err := first.AddVerifiedV1(a); err != nil {
		t.Fatal(err)
	}

	restarted := NewPersistentWorkCommitPoolV1(64, store)
	if err := restarted.ResetCommitEpochV1(7); err != nil {
		t.Fatal(err)
	}
	if restarted.LenV1() != 1 {
		t.Fatalf("restored len=%d want=1", restarted.LenV1())
	}

	store.fail = true
	if err := restarted.AddVerifiedV1(b); !errors.Is(
		err,
		errWorkCommitPoolPersistenceTest,
	) {
		t.Fatalf("add persistence error=%v", err)
	}
	if restarted.LenV1() != 1 {
		t.Fatal("failed persistent add changed the in-memory pool")
	}
	if removed := restarted.RemoveIncludedV1(
		[]common.Hash{a.ProofHash},
	); removed != 0 {
		t.Fatalf("removed=%d want=0 on persistence failure", removed)
	}
	if restarted.LenV1() != 1 {
		t.Fatal("failed persistent removal changed the in-memory pool")
	}
}

func TestWorkCommitPoolV1InputMutationDoesNotCorruptPool(
	t *testing.T,
) {
	pool := NewWorkCommitPoolV1(8)
	if err := pool.ResetCommitEpochV1(7); err != nil {
		t.Fatal(err)
	}

	candidate := commitCandidateV1(
		t,
		7,
		1,
		1,
		commitProofHashV1(1),
	)
	originalSignature := append(
		[]byte(nil),
		candidate.Signed.Signature...,
	)

	if err := pool.AddVerifiedV1(candidate); err != nil {
		t.Fatal(err)
	}

	candidate.Signed.Signature[0] ^= 0xff

	all, err := pool.AllCanonicalV1()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("all = %d, want 1", len(all))
	}
	if !bytes.Equal(
		all[0].Signed.Signature,
		originalSignature,
	) {
		t.Fatal("caller mutation corrupted pool")
	}
}

func TestWorkCommitPoolV1VerifiedCandidateLinkage(
	t *testing.T,
) {
	candidate := commitCandidateV1(
		t,
		7,
		1,
		1,
		commitProofHashV1(1),
	)

	verified := VerifiedRandomXWorkTicketV1{
		Ticket: candidate.Signed.Ticket,
		Hash:   candidate.ProofHash,
	}

	linked, err := NewVerifiedWorkCommitCandidateV1(
		candidate.Signed,
		verified,
	)
	if err != nil {
		t.Fatal(err)
	}
	if linked.ProofHash != candidate.ProofHash ||
		linked.Signed.Ticket != candidate.Signed.Ticket {
		t.Fatal("verified candidate linkage changed content")
	}

	bad := verified
	bad.Ticket.Nonce++
	if _, err := NewVerifiedWorkCommitCandidateV1(
		candidate.Signed,
		bad,
	); err != ErrWorkCommitCandidateMismatchV1 {
		t.Fatalf("mismatch error = %v", err)
	}
}
