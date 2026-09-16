package lqc

import (
	"errors"
	"testing"
)

func TestCommitteeClaimLedgerV1RejectsDuplicateAndPrunes(t *testing.T) {
	ledger := NewCommitteeClaimLedgerV1()
	emptyRoot, err := ledger.Root()
	if err != nil {
		t.Fatal(err)
	}
	groups := []CommitteeParticipationClaimGroupV1{
		{TargetBlock: 100, Participations: []CompactCommitteeParticipationV1{compactClaimV1(1), compactClaimV1(7)}},
		{TargetBlock: 93, Participations: []CompactCommitteeParticipationV1{compactClaimV1(3)}},
	}
	next, err := ledger.Apply(101, groups)
	if err != nil {
		t.Fatal(err)
	}
	if !next.IsClaimed(100, 1) || !next.IsClaimed(100, 7) || !next.IsClaimed(93, 3) {
		t.Fatal("accepted claim missing from ledger")
	}
	if ledger.IsClaimed(100, 1) {
		t.Fatal("Apply mutated its parent ledger")
	}
	nextRoot, err := next.Root()
	if err != nil {
		t.Fatal(err)
	}
	if nextRoot == emptyRoot {
		t.Fatal("accepted claims did not change ledger root")
	}

	duplicate := []CommitteeParticipationClaimGroupV1{{
		TargetBlock:    100,
		Participations: []CompactCommitteeParticipationV1{compactClaimV1(1)},
	}}
	if _, err := next.Apply(102, duplicate); !errors.Is(err, ErrCommitteeParticipationClaimedV1) {
		t.Fatalf("duplicate error=%v", err)
	}
	if !next.IsClaimed(100, 1) {
		t.Fatal("failed duplicate transition mutated ledger")
	}

	latest, err := next.Apply(109, []CommitteeParticipationClaimGroupV1{{
		TargetBlock:    108,
		Participations: []CompactCommitteeParticipationV1{compactClaimV1(2)},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if latest.IsClaimed(100, 1) || latest.IsClaimed(93, 3) || !latest.IsClaimed(108, 2) {
		t.Fatalf("unexpected pruned ledger: %+v", latest.Windows)
	}
}

func TestCommitteeClaimLedgerV1CanonicalRootIgnoresArrivalOrder(t *testing.T) {
	left, err := NewCommitteeClaimLedgerV1().Apply(101, []CommitteeParticipationClaimGroupV1{
		{TargetBlock: 100, Participations: []CompactCommitteeParticipationV1{compactClaimV1(7), compactClaimV1(1)}},
		{TargetBlock: 99, Participations: []CompactCommitteeParticipationV1{compactClaimV1(2)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	right, err := NewCommitteeClaimLedgerV1().Apply(101, []CommitteeParticipationClaimGroupV1{
		{TargetBlock: 99, Participations: []CompactCommitteeParticipationV1{compactClaimV1(2)}},
		{TargetBlock: 100, Participations: []CompactCommitteeParticipationV1{compactClaimV1(1), compactClaimV1(7)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	leftRoot, err := left.Root()
	if err != nil {
		t.Fatal(err)
	}
	rightRoot, err := right.Root()
	if err != nil {
		t.Fatal(err)
	}
	if leftRoot != rightRoot {
		t.Fatalf("arrival order changed claim root: %s != %s", leftRoot, rightRoot)
	}
}
