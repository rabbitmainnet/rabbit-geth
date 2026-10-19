package lqc

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestRabbitVRFCommitteeMembersV1AssignSequentialShareIDs(
	t *testing.T,
) {
	committee := []WorkSeatV1{
		{
			TicketHash:  common.HexToHash("0x11"),
			Participant: common.HexToAddress("0x1"),
		},
		{
			TicketHash:  common.HexToHash("0x22"),
			Participant: common.HexToAddress("0x2"),
		},
		{
			TicketHash:  common.HexToHash("0x33"),
			Participant: common.HexToAddress("0x3"),
		},
	}

	members, err := RabbitVRFCommitteeMembersV1(
		committee,
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(members) != 3 {
		t.Fatalf(
			"members=%d want=3",
			len(members),
		)
	}

	for index, member := range members {
		want := uint64(index + 1)
		if member.ShareID != want {
			t.Fatalf(
				"index=%d shareID=%d want=%d",
				index,
				member.ShareID,
				want,
			)
		}

		if member.TicketHash != committee[index].TicketHash {
			t.Fatalf(
				"ticket mismatch at index %d",
				index,
			)
		}

		if member.Participant != committee[index].Participant {
			t.Fatalf(
				"participant mismatch at index %d",
				index,
			)
		}
	}
}

func TestRabbitVRFCommitteeMembersV1RejectsDuplicateIdentity(
	t *testing.T,
) {
	sameParticipant := []WorkSeatV1{
		{
			TicketHash:  common.HexToHash("0x11"),
			Participant: common.HexToAddress("0x1"),
		},
		{
			TicketHash:  common.HexToHash("0x22"),
			Participant: common.HexToAddress("0x1"),
		},
	}

	if _, err := RabbitVRFCommitteeMembersV1(
		sameParticipant,
	); err == nil {
		t.Fatal("duplicate participant accepted")
	}

	sameTicket := []WorkSeatV1{
		{
			TicketHash:  common.HexToHash("0x11"),
			Participant: common.HexToAddress("0x1"),
		},
		{
			TicketHash:  common.HexToHash("0x11"),
			Participant: common.HexToAddress("0x2"),
		},
	}

	if _, err := RabbitVRFCommitteeMembersV1(
		sameTicket,
	); err == nil {
		t.Fatal("duplicate ticket accepted")
	}
}

func TestRabbitVRFCommitteeRootV1StableAndOrderBound(
	t *testing.T,
) {
	chainID := big.NewInt(9280)
	selectionRoot := common.HexToHash("0xaaaa")
	seed := common.HexToHash("0xbbbb")

	committee := []WorkSeatV1{
		{
			TicketHash:  common.HexToHash("0x11"),
			Participant: common.HexToAddress("0x1"),
		},
		{
			TicketHash:  common.HexToHash("0x22"),
			Participant: common.HexToAddress("0x2"),
		},
		{
			TicketHash:  common.HexToHash("0x33"),
			Participant: common.HexToAddress("0x3"),
		},
	}

	first, firstMembers, err := RabbitVRFCommitteeRootV1(
		chainID,
		7,
		selectionRoot,
		seed,
		committee,
	)
	if err != nil {
		t.Fatal(err)
	}

	second, secondMembers, err := RabbitVRFCommitteeRootV1(
		chainID,
		7,
		selectionRoot,
		seed,
		committee,
	)
	if err != nil {
		t.Fatal(err)
	}

	if first != second {
		t.Fatal("same committee context produced different root")
	}

	if len(firstMembers) != len(secondMembers) {
		t.Fatal("same committee produced different member count")
	}

	reversed := append(
		[]WorkSeatV1(nil),
		committee...,
	)
	reversed[0], reversed[2] =
		reversed[2], reversed[0]

	reorderedRoot, reorderedMembers, err :=
		RabbitVRFCommitteeRootV1(
			chainID,
			7,
			selectionRoot,
			seed,
			reversed,
		)
	if err != nil {
		t.Fatal(err)
	}

	if reorderedRoot == first {
		t.Fatal("committee reordering did not change root")
	}

	if reorderedMembers[0].ShareID != 1 ||
		reorderedMembers[0].Participant != reversed[0].Participant {
		t.Fatal("ShareID did not follow committee position")
	}
}

func TestRabbitVRFCommitteeRootV1BindsContext(
	t *testing.T,
) {
	committee := []WorkSeatV1{
		{
			TicketHash:  common.HexToHash("0x11"),
			Participant: common.HexToAddress("0x1"),
		},
	}

	chainID := big.NewInt(9280)
	root := common.HexToHash("0xaaaa")
	seed := common.HexToHash("0xbbbb")

	base, _, err := RabbitVRFCommitteeRootV1(
		chainID,
		7,
		root,
		seed,
		committee,
	)
	if err != nil {
		t.Fatal(err)
	}

	otherChain, _, err := RabbitVRFCommitteeRootV1(
		big.NewInt(928),
		7,
		root,
		seed,
		committee,
	)
	if err != nil {
		t.Fatal(err)
	}
	if otherChain == base {
		t.Fatal("chain id did not bind committee root")
	}

	otherEpoch, _, err := RabbitVRFCommitteeRootV1(
		chainID,
		8,
		root,
		seed,
		committee,
	)
	if err != nil {
		t.Fatal(err)
	}
	if otherEpoch == base {
		t.Fatal("source epoch did not bind committee root")
	}

	otherSelection, _, err := RabbitVRFCommitteeRootV1(
		chainID,
		7,
		common.HexToHash("0xaaab"),
		seed,
		committee,
	)
	if err != nil {
		t.Fatal(err)
	}
	if otherSelection == base {
		t.Fatal("selection root did not bind committee root")
	}

	otherSeed, _, err := RabbitVRFCommitteeRootV1(
		chainID,
		7,
		root,
		common.HexToHash("0xbbbc"),
		committee,
	)
	if err != nil {
		t.Fatal(err)
	}
	if otherSeed == base {
		t.Fatal("committee seed did not bind committee root")
	}
}

func TestRabbitVRFCommitteeCommitmentForSnapshotV1Deterministic(
	t *testing.T,
) {
	chainID := big.NewInt(9280)
	snapshot := rabbitVRFCommitteeTestSnapshotV1(
		t,
		chainID,
		100,
	)
	entropy := common.HexToHash("0x12345678")

	rootA, seedA, membersA, err :=
		RabbitVRFCommitteeCommitmentForSnapshotV1(
			chainID,
			snapshot,
			entropy,
			32,
			128,
		)
	if err != nil {
		t.Fatal(err)
	}

	rootB, seedB, membersB, err :=
		RabbitVRFCommitteeCommitmentForSnapshotV1(
			chainID,
			snapshot,
			entropy,
			32,
			128,
		)
	if err != nil {
		t.Fatal(err)
	}

	if rootA != rootB {
		t.Fatal("committee commitment changed")
	}

	if seedA != seedB {
		t.Fatal("committee seed changed")
	}

	if len(membersA) != 32 ||
		len(membersB) != 32 {
		t.Fatalf(
			"member sizes=%d,%d want=32",
			len(membersA),
			len(membersB),
		)
	}

	for index := range membersA {
		if membersA[index] != membersB[index] {
			t.Fatalf(
				"member differs at index %d",
				index,
			)
		}

		if membersA[index].ShareID != uint64(index+1) {
			t.Fatalf(
				"shareID=%d index=%d",
				membersA[index].ShareID,
				index,
			)
		}
	}
}
