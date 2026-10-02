//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package lqc

import "testing"

func TestWorkV1EngineClaimFilterUsesParentLedgerRecovery(t *testing.T) {
	parent, err := NewCommitteeClaimLedgerV1().Apply(
		102,
		[]CommitteeParticipationClaimGroupV1{{
			TargetBlock: 101,
			Participations: []CompactCommitteeParticipationV1{
				compactClaimV1(1),
			},
		}},
	)
	if err != nil {
		t.Fatalf("build parent ledger: %v", err)
	}

	offered := []CommitteeParticipationClaimGroupV1{
		{
			TargetBlock: 100,
			Participations: []CompactCommitteeParticipationV1{
				compactClaimV1(3),
			},
		},
		{
			TargetBlock: 101,
			Participations: []CompactCommitteeParticipationV1{
				compactClaimV1(1),
				compactClaimV1(2),
			},
		},
	}

	filtered := filterAlreadyClaimedCommitteeParticipationsV1(
		parent,
		offered,
	)

	if len(filtered) != 2 {
		t.Fatalf("groups=%d want=2", len(filtered))
	}

	if len(filtered[0].Participations) != 1 ||
		filtered[0].Participations[0].Position != 3 {
		t.Fatalf("unexpected first group: %+v", filtered[0])
	}

	if len(filtered[1].Participations) != 1 ||
		filtered[1].Participations[0].Position != 2 {
		t.Fatalf("duplicate survived: %+v", filtered[1])
	}

	if _, err := parent.Apply(103, filtered); err != nil {
		t.Fatalf("filtered claims failed Apply: %v", err)
	}
}
