//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"errors"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
)

func rabbitVRFDKGRuntimeTestMemberV1(
	shareID uint64,
	participant string,
	ticket string,
) lqc.RabbitVRFCommitteeMemberV1 {
	return lqc.RabbitVRFCommitteeMemberV1{
		ShareID:     shareID,
		Participant: common.HexToAddress(participant),
		TicketHash:  common.HexToHash(ticket),
	}
}

func TestRabbitVRFDKGRuntimeV1ResolvesLocalMembers(
	t *testing.T,
) {
	members := []lqc.RabbitVRFCommitteeMemberV1{
		rabbitVRFDKGRuntimeTestMemberV1(
			1,
			"0x0000000000000000000000000000000000000011",
			"0x11",
		),
		rabbitVRFDKGRuntimeTestMemberV1(
			2,
			"0x0000000000000000000000000000000000000022",
			"0x22",
		),
		rabbitVRFDKGRuntimeTestMemberV1(
			3,
			"0x0000000000000000000000000000000000000033",
			"0x33",
		),
	}

	local := map[common.Address]struct{}{
		members[0].Participant: {},
		members[2].Participant: {},
	}

	got, err :=
		rabbitVRFDKGMatchLocalMembersV1(
			local,
			members,
		)
	if err != nil {
		t.Fatal(err)
	}

	if len(got) != 2 {
		t.Fatalf(
			"local member count=%d want=2",
			len(got),
		)
	}

	if got[0] != members[0] ||
		got[1] != members[2] {
		t.Fatal(
			"local members lost canonical committee order",
		)
	}

	if got[0].ShareID != 1 ||
		got[1].ShareID != 3 {
		t.Fatal(
			"immutable ShareIDs changed during local resolution",
		)
	}
}

func TestRabbitVRFDKGRuntimeV1NonMemberHasNoSelection(
	t *testing.T,
) {
	members := []lqc.RabbitVRFCommitteeMemberV1{
		rabbitVRFDKGRuntimeTestMemberV1(
			1,
			"0x0000000000000000000000000000000000000011",
			"0x11",
		),
		rabbitVRFDKGRuntimeTestMemberV1(
			2,
			"0x0000000000000000000000000000000000000022",
			"0x22",
		),
	}

	local := map[common.Address]struct{}{
		common.HexToAddress(
			"0x0000000000000000000000000000000000000099",
		): {},
	}

	got, err :=
		rabbitVRFDKGMatchLocalMembersV1(
			local,
			members,
		)
	if err != nil {
		t.Fatal(err)
	}

	if len(got) != 0 {
		t.Fatal(
			"wallet outside canonical committee was selected",
		)
	}
}

func TestRabbitVRFDKGRuntimeV1RejectsInvalidShareID(
	t *testing.T,
) {
	members := []lqc.RabbitVRFCommitteeMemberV1{
		rabbitVRFDKGRuntimeTestMemberV1(
			7,
			"0x0000000000000000000000000000000000000011",
			"0x11",
		),
	}

	_, err :=
		rabbitVRFDKGMatchLocalMembersV1(
			map[common.Address]struct{}{
				members[0].Participant: {},
			},
			members,
		)

	if !errors.Is(
		err,
		errRabbitVRFDKGRuntimeV1,
	) {
		t.Fatalf(
			"error=%v want=%v",
			err,
			errRabbitVRFDKGRuntimeV1,
		)
	}
}

func TestRabbitVRFDKGRuntimeV1RejectsDuplicateParticipant(
	t *testing.T,
) {
	participant :=
		"0x0000000000000000000000000000000000000011"

	members := []lqc.RabbitVRFCommitteeMemberV1{
		rabbitVRFDKGRuntimeTestMemberV1(
			1,
			participant,
			"0x11",
		),
		rabbitVRFDKGRuntimeTestMemberV1(
			2,
			participant,
			"0x22",
		),
	}

	_, err :=
		rabbitVRFDKGMatchLocalMembersV1(
			map[common.Address]struct{}{},
			members,
		)

	if !errors.Is(
		err,
		errRabbitVRFDKGRuntimeV1,
	) {
		t.Fatalf(
			"error=%v want=%v",
			err,
			errRabbitVRFDKGRuntimeV1,
		)
	}
}
