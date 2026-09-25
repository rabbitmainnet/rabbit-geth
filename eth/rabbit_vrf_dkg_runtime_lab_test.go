//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"errors"
	"testing"
	"time"

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

func TestRabbitVRFDKGRuntimeV1SecretGateBlocksSyncTransition(
	t *testing.T,
) {
	runtime := &rabbitVRFDKGRuntime{
		secretReady: true,
	}

	if !runtime.beginSecretOperationV1() {
		t.Fatal("secret operation did not enter while ready")
	}

	started := make(chan struct{})
	done := make(chan struct{})

	go func() {
		close(started)
		runtime.setSyncingV1(true)
		close(done)
	}()

	<-started

	select {
	case <-done:
		runtime.endSecretOperationV1()
		t.Fatal("sync transition crossed active secret operation")
	case <-time.After(50 * time.Millisecond):
	}

	runtime.endSecretOperationV1()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("sync transition did not complete after secret operation")
	}

	if runtime.secretReadyV1() {
		t.Fatal("secret readiness remained open after sync started")
	}

	if runtime.beginSecretOperationV1() {
		runtime.endSecretOperationV1()
		t.Fatal("secret operation entered while syncing")
	}
}

func TestRabbitVRFDKGRuntimeV1NoLocalMembersSkipSecretIO(
	t *testing.T,
) {
	runtime := &rabbitVRFDKGRuntime{
		backend: &Ethereum{},
	}

	err := runtime.ensureTransportBindingsV1(
		lqc.RabbitVRFDKGBridgeV1{},
		nil,
	)
	if err != nil {
		t.Fatalf(
			"zero local members touched secret runtime: %v",
			err,
		)
	}
}

func TestRabbitVRFDKGRuntimeV1PreservesTransportBindingsSameSession(
	t *testing.T,
) {
	bridge := lqc.RabbitVRFDKGBridgeV1{}
	bridge.SessionID[0] = 1

	member := lqc.RabbitVRFCommitteeMemberV1{ShareID: 7}
	member.Participant[0] = 2

	binding := lqc.RabbitVRFDKGTransportKeyBindingV1{
		SessionID:   bridge.SessionID,
		ShareID:     member.ShareID,
		Participant: member.Participant,
	}

	runtime := &rabbitVRFDKGRuntime{
		current: rabbitVRFDKGLocalContextV1{
			SessionID:         bridge.SessionID,
			Members:           []lqc.RabbitVRFCommitteeMemberV1{member},
			TransportBindings: []lqc.RabbitVRFDKGTransportKeyBindingV1{binding},
		},
	}

	runtime.setCurrent(
		rabbitVRFDKGLocalContextV1{
			HeadNumber: 2,
			SessionID:  bridge.SessionID,
			Members:    []lqc.RabbitVRFCommitteeMemberV1{member},
		},
	)

	got := runtime.currentContext()
	if len(got.TransportBindings) != 1 {
		t.Fatalf("transport bindings=%d want=1", len(got.TransportBindings))
	}
	if got.TransportBindings[0] != binding {
		t.Fatal("transport binding was not preserved across same-session head")
	}
	if !runtime.transportBindingsReadyV1(bridge.SessionID, []lqc.RabbitVRFCommitteeMemberV1{member}) {
		t.Fatal("preserved transport binding is not recognized as ready")
	}
}

func TestRabbitVRFDKGRuntimeV1DropsTransportBindingsOnSessionChange(
	t *testing.T,
) {
	oldSession := common.Hash{1}
	newSession := common.Hash{2}

	member := lqc.RabbitVRFCommitteeMemberV1{ShareID: 7}
	member.Participant[0] = 3

	binding := lqc.RabbitVRFDKGTransportKeyBindingV1{
		SessionID:   oldSession,
		ShareID:     member.ShareID,
		Participant: member.Participant,
	}

	runtime := &rabbitVRFDKGRuntime{
		current: rabbitVRFDKGLocalContextV1{
			SessionID:         oldSession,
			Members:           []lqc.RabbitVRFCommitteeMemberV1{member},
			TransportBindings: []lqc.RabbitVRFDKGTransportKeyBindingV1{binding},
		},
	}

	runtime.setCurrent(
		rabbitVRFDKGLocalContextV1{
			SessionID: newSession,
			Members:   []lqc.RabbitVRFCommitteeMemberV1{member},
		},
	)

	got := runtime.currentContext()
	if len(got.TransportBindings) != 0 {
		t.Fatalf("stale transport bindings survived session change: %d", len(got.TransportBindings))
	}
}
