//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"errors"
	"github.com/ethereum/go-ethereum/crypto"
	"math/big"
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

func TestRabbitVRFDKGRuntimeV1PreservesTransportEnvelopeSameSession(
	t *testing.T,
) {
	sessionID := common.Hash{1}

	member := lqc.RabbitVRFCommitteeMemberV1{
		ShareID:     7,
		Participant: common.Address{2},
	}

	envelope := lqc.RabbitVRFDKGEnvelopeV1{
		SessionID:     sessionID,
		SenderShareID: member.ShareID,
		Participant:   member.Participant,
		Signature:     []byte{1, 2, 3},
	}

	runtime := &rabbitVRFDKGRuntime{
		current: rabbitVRFDKGLocalContextV1{
			SessionID:          sessionID,
			Members:            []lqc.RabbitVRFCommitteeMemberV1{member},
			TransportEnvelopes: []lqc.RabbitVRFDKGEnvelopeV1{envelope},
		},
	}

	runtime.setCurrent(
		rabbitVRFDKGLocalContextV1{
			HeadNumber: 2,
			SessionID:  sessionID,
			Members:    []lqc.RabbitVRFCommitteeMemberV1{member},
		},
	)

	got := runtime.currentContext()
	if len(got.TransportEnvelopes) != 1 {
		t.Fatalf("transport envelopes=%d want=1", len(got.TransportEnvelopes))
	}
	if len(got.TransportEnvelopes[0].Signature) != 3 {
		t.Fatal("transport envelope signature was not preserved")
	}

	got.TransportEnvelopes[0].Signature[0] = 9
	again := runtime.currentContext()
	if again.TransportEnvelopes[0].Signature[0] != 1 {
		t.Fatal("transport envelope signature was not defensively copied")
	}
}

func TestRabbitVRFDKGRuntimeV1DropsTransportEnvelopeOnSessionChange(
	t *testing.T,
) {
	oldSession := common.Hash{1}
	newSession := common.Hash{2}

	member := lqc.RabbitVRFCommitteeMemberV1{
		ShareID:     7,
		Participant: common.Address{3},
	}

	envelope := lqc.RabbitVRFDKGEnvelopeV1{
		SessionID:     oldSession,
		SenderShareID: member.ShareID,
		Participant:   member.Participant,
		Signature:     []byte{4, 5, 6},
	}

	runtime := &rabbitVRFDKGRuntime{
		current: rabbitVRFDKGLocalContextV1{
			SessionID:          oldSession,
			Members:            []lqc.RabbitVRFCommitteeMemberV1{member},
			TransportEnvelopes: []lqc.RabbitVRFDKGEnvelopeV1{envelope},
		},
	}

	runtime.setCurrent(
		rabbitVRFDKGLocalContextV1{
			SessionID: newSession,
			Members:   []lqc.RabbitVRFCommitteeMemberV1{member},
		},
	)

	got := runtime.currentContext()
	if len(got.TransportEnvelopes) != 0 {
		t.Fatalf("stale transport envelopes survived session change: %d", len(got.TransportEnvelopes))
	}
}

func TestRabbitVRFDKGRuntimeV1TransportArtifactsRequireValidSignature(
	t *testing.T,
) {
	context, err := lqc.NewRabbitVRFDKGSessionContextV1(
		big.NewInt(9280),
		11,
		crypto.Keccak256Hash(
			[]byte("rabbit-vrf-runtime-artifact-ready-committee"),
		),
		32,
	)
	if err != nil {
		t.Fatal(err)
	}

	participantKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}

	member := lqc.RabbitVRFCommitteeMemberV1{
		ShareID: 7,
		TicketHash: crypto.Keccak256Hash(
			[]byte("rabbit-vrf-runtime-artifact-ready-ticket"),
		),
		Participant: crypto.PubkeyToAddress(
			participantKey.PublicKey,
		),
	}

	transportKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}

	publicKey, err :=
		lqc.RabbitVRFDKGTransportPublicKeyV1FromBytes(
			crypto.CompressPubkey(
				&transportKey.PublicKey,
			),
		)
	if err != nil {
		t.Fatal(err)
	}

	binding, root, err :=
		lqc.NewRabbitVRFDKGTransportKeyBindingV1(
			context,
			member,
			publicKey,
		)
	if err != nil {
		t.Fatal(err)
	}

	envelope, err := lqc.NewRabbitVRFDKGEnvelopeV1(
		context,
		member,
		lqc.RabbitVRFDKGMessageTransportKeyBindingV1,
		root,
	)
	if err != nil {
		t.Fatal(err)
	}

	signingHash, err :=
		lqc.RabbitVRFDKGEnvelopeSigningHashV1(
			context,
			envelope,
		)
	if err != nil {
		t.Fatal(err)
	}

	envelope.Signature, err = crypto.Sign(
		signingHash[:],
		participantKey,
	)
	if err != nil {
		t.Fatal(err)
	}

	sessionID, err := lqc.RabbitVRFDKGSessionIDV1(context)
	if err != nil {
		t.Fatal(err)
	}

	runtime := &rabbitVRFDKGRuntime{
		current: rabbitVRFDKGLocalContextV1{
			SessionID:          sessionID,
			Members:            []lqc.RabbitVRFCommitteeMemberV1{member},
			TransportBindings:  []lqc.RabbitVRFDKGTransportKeyBindingV1{binding},
			TransportEnvelopes: []lqc.RabbitVRFDKGEnvelopeV1{envelope},
		},
	}

	if !runtime.transportArtifactsReadyV1(
		context,
		sessionID,
		[]lqc.RabbitVRFCommitteeMemberV1{member},
	) {
		t.Fatal("valid authenticated transport artifacts not ready")
	}

	runtime.mu.Lock()
	runtime.current.TransportEnvelopes[0].Signature[0] ^= 1
	runtime.mu.Unlock()

	if runtime.transportArtifactsReadyV1(
		context,
		sessionID,
		[]lqc.RabbitVRFCommitteeMemberV1{member},
	) {
		t.Fatal("tampered transport envelope accepted as ready")
	}
}
