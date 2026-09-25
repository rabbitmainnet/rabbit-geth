//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"errors"
	"fmt"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/enode"
)

func mustRabbitVRFDKGTransportV1(
	t *testing.T,
	chainID int64,
	networkID uint64,
	genesis common.Hash,
) *rabbitVRFDKGTransport {
	t.Helper()

	transport, err := newRabbitVRFDKGTransport(
		rabbitVRFDKGTransportConfig{
			ChainID:   big.NewInt(chainID),
			NetworkID: networkID,
			Genesis:   genesis,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return transport
}

func runRabbitVRFDKGHandshakePairV1(
	t *testing.T,
	left *rabbitVRFDKGTransport,
	right *rabbitVRFDKGTransport,
) [2]error {
	t.Helper()

	leftRW, rightRW := p2p.MsgPipe()
	defer leftRW.Close()
	defer rightRW.Close()

	leftPeer := &rabbitVRFDKGPeer{
		peer: p2p.NewPeerPipe(
			enode.ID{1},
			"left",
			nil,
			leftRW,
		),
		rw: leftRW,
	}
	rightPeer := &rabbitVRFDKGPeer{
		peer: p2p.NewPeerPipe(
			enode.ID{2},
			"right",
			nil,
			rightRW,
		),
		rw: rightRW,
	}

	results := make(chan error, 2)
	go func() { results <- left.handshake(leftPeer) }()
	go func() { results <- right.handshake(rightPeer) }()

	var got [2]error
	for index := range got {
		select {
		case got[index] = <-results:
		case <-time.After(2 * time.Second):
			t.Fatal("rabbit vrf dkg handshake timed out")
		}
	}
	return got
}

func TestRabbitVRFDKGTransportV1HandshakeAcceptsMatchingNetwork(
	t *testing.T,
) {
	genesis := common.HexToHash(
		"0x9280928092809280928092809280928092809280928092809280928092809280",
	)

	left := mustRabbitVRFDKGTransportV1(t, 9280, 9280, genesis)
	right := mustRabbitVRFDKGTransportV1(t, 9280, 9280, genesis)

	results := runRabbitVRFDKGHandshakePairV1(t, left, right)
	for _, err := range results {
		if err != nil {
			t.Fatalf("matching handshake failed: %v", err)
		}
	}
}

func TestRabbitVRFDKGTransportV1HandshakeRejectsChainIDMismatch(
	t *testing.T,
) {
	genesis := common.HexToHash(
		"0x9280928092809280928092809280928092809280928092809280928092809280",
	)

	left := mustRabbitVRFDKGTransportV1(t, 9280, 9280, genesis)
	right := mustRabbitVRFDKGTransportV1(t, 9281, 9280, genesis)

	results := runRabbitVRFDKGHandshakePairV1(t, left, right)
	for _, err := range results {
		if err == nil {
			t.Fatal("chain ID mismatch was accepted")
		}
	}
}

func TestRabbitVRFDKGTransportV1HandshakeRejectsGenesisMismatch(
	t *testing.T,
) {
	leftGenesis := common.Hash{1}
	rightGenesis := common.Hash{2}

	left := mustRabbitVRFDKGTransportV1(
		t, 9280, 9280, leftGenesis,
	)
	right := mustRabbitVRFDKGTransportV1(
		t, 9280, 9280, rightGenesis,
	)

	results := runRabbitVRFDKGHandshakePairV1(t, left, right)
	for _, err := range results {
		if err == nil {
			t.Fatal("genesis mismatch was accepted")
		}
	}
}

func TestRabbitVRFDKGTransportV1HandshakeRejectsSessionVersionMismatch(
	t *testing.T,
) {
	genesis := common.Hash{1}
	transport := mustRabbitVRFDKGTransportV1(
		t, 9280, 9280, genesis,
	)

	localRW, remoteRW := p2p.MsgPipe()
	defer localRW.Close()
	defer remoteRW.Close()

	peer := &rabbitVRFDKGPeer{
		peer: p2p.NewPeerPipe(
			enode.ID{1},
			"local",
			nil,
			localRW,
		),
		rw: localRW,
	}

	remoteDone := make(chan error, 1)
	go func() {
		message, err := remoteRW.ReadMsg()
		if err != nil {
			remoteDone <- err
			return
		}
		if message.Code != rabbitVRFDKGStatusMsg {
			remoteDone <- errors.New("unexpected rabbit vrf dkg status code")
			return
		}

		var localStatus rabbitVRFDKGStatusPacket
		if err := message.Decode(&localStatus); err != nil {
			remoteDone <- err
			return
		}

		remoteStatus := localStatus
		remoteStatus.SessionVersion++

		remoteDone <- p2p.Send(
			remoteRW,
			rabbitVRFDKGStatusMsg,
			remoteStatus,
		)
	}()

	err := transport.handshake(peer)
	if err == nil {
		t.Fatal("session version mismatch was accepted")
	}
	if err.Error() != "rabbit vrf dkg session version mismatch" {
		t.Fatalf("unexpected handshake error: %v", err)
	}

	select {
	case err := <-remoteDone:
		if err != nil {
			t.Fatalf("remote handshake side failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("remote handshake side timed out")
	}
}

func TestRabbitVRFDKGTransportV1RejectsDuplicateAndClosedPeer(
	t *testing.T,
) {
	transport := mustRabbitVRFDKGTransportV1(
		t,
		9280,
		9280,
		common.Hash{1},
	)

	first := &rabbitVRFDKGPeer{
		peer: p2p.NewPeer(
			enode.ID{9},
			"first",
			nil,
		),
	}
	if err := transport.register(first); err != nil {
		t.Fatalf("initial peer registration failed: %v", err)
	}

	duplicate := &rabbitVRFDKGPeer{
		peer: p2p.NewPeer(
			enode.ID{9},
			"duplicate",
			nil,
		),
	}
	if err := transport.register(duplicate); !errors.Is(err, errRabbitVRFDKGPeerKnown) {
		t.Fatalf("duplicate peer error=%v want=%v", err, errRabbitVRFDKGPeerKnown)
	}

	transport.Close()

	afterClose := &rabbitVRFDKGPeer{
		peer: p2p.NewPeer(
			enode.ID{10},
			"after-close",
			nil,
		),
	}
	if err := transport.register(afterClose); !errors.Is(err, errRabbitVRFDKGProtocolClosed) {
		t.Fatalf("closed transport error=%v want=%v", err, errRabbitVRFDKGProtocolClosed)
	}
}

func newRabbitVRFDKGTransportArtifactFixtureV1(
	t *testing.T,
) (
	*rabbitVRFDKGTransport,
	rabbitVRFDKGTransportArtifactPacketV1,
) {
	t.Helper()

	participantKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}

	context, err := lqc.NewRabbitVRFDKGSessionContextV1(
		big.NewInt(9280),
		11,
		crypto.Keccak256Hash(
			[]byte("rabbit-vrf-p2p-artifact-committee"),
		),
		32,
	)
	if err != nil {
		t.Fatal(err)
	}

	member := lqc.RabbitVRFCommitteeMemberV1{
		ShareID: 7,
		TicketHash: crypto.Keccak256Hash(
			[]byte("rabbit-vrf-p2p-artifact-ticket"),
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
			SessionID:        sessionID,
			CanonicalSession: context,
			CanonicalMembers: []lqc.RabbitVRFCommitteeMemberV1{
				member,
			},
		},
	}

	transport, err := newRabbitVRFDKGTransport(
		rabbitVRFDKGTransportConfig{
			ChainID:   big.NewInt(9280),
			NetworkID: 9280,
			Genesis: crypto.Keccak256Hash(
				[]byte("rabbit-vrf-p2p-artifact-genesis"),
			),
			Runtime: runtime,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	return transport, rabbitVRFDKGTransportArtifactPacketV1{
		Binding:  binding,
		Envelope: envelope,
	}
}

func TestRabbitVRFDKGTransportV1AcceptsValidArtifact(
	t *testing.T,
) {
	transport, packet :=
		newRabbitVRFDKGTransportArtifactFixtureV1(t)

	if err := transport.validateTransportArtifactV1(packet); err != nil {
		t.Fatalf("valid transport artifact rejected: %v", err)
	}
}

func TestRabbitVRFDKGTransportV1RejectsWrongArtifactSession(
	t *testing.T,
) {
	transport, packet :=
		newRabbitVRFDKGTransportArtifactFixtureV1(t)

	packet.Binding.SessionID[0] ^= 0x01

	if err := transport.validateTransportArtifactV1(packet); err == nil {
		t.Fatal("wrong-session transport artifact accepted")
	}
}

func TestRabbitVRFDKGTransportV1RejectsNonCanonicalMember(
	t *testing.T,
) {
	transport, packet :=
		newRabbitVRFDKGTransportArtifactFixtureV1(t)

	transport.runtime.mu.Lock()
	transport.runtime.current.CanonicalMembers = nil
	transport.runtime.mu.Unlock()

	if err := transport.validateTransportArtifactV1(packet); err == nil {
		t.Fatal("non-canonical transport artifact accepted")
	}
}

func TestRabbitVRFDKGTransportV1RejectsTamperedArtifactSignature(
	t *testing.T,
) {
	transport, packet :=
		newRabbitVRFDKGTransportArtifactFixtureV1(t)

	packet.Envelope.Signature = append(
		[]byte(nil),
		packet.Envelope.Signature...,
	)
	packet.Envelope.Signature[0] ^= 0x01

	if err := transport.validateTransportArtifactV1(packet); err == nil {
		t.Fatal("tampered transport artifact signature accepted")
	}
}

func startRabbitVRFDKGTransportArtifactWireV1(
	t *testing.T,
	receiver *rabbitVRFDKGTransport,
) (p2p.MsgReadWriter, <-chan error) {
	t.Helper()

	leftRW, rightRW := p2p.MsgPipe()
	t.Cleanup(func() {
		leftRW.Close()
		rightRW.Close()
	})

	sender, err := newRabbitVRFDKGTransport(
		rabbitVRFDKGTransportConfig{
			ChainID:   new(big.Int).Set(receiver.chainID),
			NetworkID: receiver.networkID,
			Genesis:   receiver.genesis,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	receiverPeer := p2p.NewPeerPipe(
		enode.ID{9},
		"receiver",
		nil,
		leftRW,
	)

	senderPeer := &rabbitVRFDKGPeer{
		peer: p2p.NewPeerPipe(
			enode.ID{10},
			"sender",
			nil,
			rightRW,
		),
		rw: rightRW,
	}

	result := make(chan error, 1)
	go func() {
		err := receiver.runPeer(receiverPeer, leftRW)
		leftRW.Close()
		result <- err
	}()

	if err := sender.handshake(senderPeer); err != nil {
		t.Fatal(err)
	}

	return rightRW, result
}

func TestRabbitVRFDKGTransportV1WireAcceptsValidArtifact(
	t *testing.T,
) {
	receiver, packet :=
		newRabbitVRFDKGTransportArtifactFixtureV1(t)

	rw, result :=
		startRabbitVRFDKGTransportArtifactWireV1(
			t,
			receiver,
		)

	if err := p2p.Send(
		rw,
		rabbitVRFDKGTransportArtifactMsg,
		packet,
	); err != nil {
		t.Fatal(err)
	}

	sendResult := make(chan error, 1)
	go func() {
		sendResult <- p2p.Send(rw, 99, uint64(0))
	}()

	select {
	case err := <-result:
		if err == nil ||
			!strings.Contains(
				err.Error(),
				"invalid rabbit vrf dkg message code: 99",
			) {
			t.Fatalf(
				"valid artifact did not reach follow-up message: %v",
				err,
			)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("valid artifact wire test timed out")
	}

	select {
	case <-sendResult:
	case <-time.After(2 * time.Second):
		t.Fatal("follow-up send timed out")
	}
}

func TestRabbitVRFDKGTransportV1WireRejectsTamperedArtifact(
	t *testing.T,
) {
	receiver, packet :=
		newRabbitVRFDKGTransportArtifactFixtureV1(t)

	packet.Envelope.Signature = append(
		[]byte(nil),
		packet.Envelope.Signature...,
	)
	packet.Envelope.Signature[0] ^= 0x01

	rw, result :=
		startRabbitVRFDKGTransportArtifactWireV1(
			t,
			receiver,
		)

	if err := p2p.Send(
		rw,
		rabbitVRFDKGTransportArtifactMsg,
		packet,
	); err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-result:
		if err == nil ||
			!strings.Contains(
				err.Error(),
				"validate rabbit vrf dkg transport artifact",
			) {
			t.Fatalf(
				"tampered artifact returned unexpected result: %v",
				err,
			)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("tampered artifact wire test timed out")
	}
}

func TestRabbitVRFDKGTransportV1RemoteStoreFirstInsert(
	t *testing.T,
) {
	transport, packet :=
		newRabbitVRFDKGTransportArtifactFixtureV1(t)

	inserted, err :=
		transport.storeRemoteTransportArtifactV1(packet)
	if err != nil {
		t.Fatal(err)
	}
	if !inserted {
		t.Fatal("first remote artifact was not inserted")
	}

	stored, ok := transport.remoteTransportArtifactV1(
		packet.Binding.SessionID,
		packet.Binding.ShareID,
	)
	if !ok {
		t.Fatal("stored remote artifact not found")
	}
	if stored.Binding != packet.Binding {
		t.Fatal("stored remote binding changed")
	}
	if string(stored.Envelope.Signature) !=
		string(packet.Envelope.Signature) {
		t.Fatal("stored remote signature changed")
	}
}

func TestRabbitVRFDKGTransportV1RemoteStoreDeduplicates(
	t *testing.T,
) {
	transport, packet :=
		newRabbitVRFDKGTransportArtifactFixtureV1(t)

	inserted, err :=
		transport.storeRemoteTransportArtifactV1(packet)
	if err != nil || !inserted {
		t.Fatalf("first insert failed: inserted=%v err=%v", inserted, err)
	}

	inserted, err =
		transport.storeRemoteTransportArtifactV1(packet)
	if err != nil {
		t.Fatal(err)
	}
	if inserted {
		t.Fatal("duplicate remote artifact inserted twice")
	}

	transport.mu.RLock()
	count := len(transport.remoteArtifacts)
	transport.mu.RUnlock()

	if count != 1 {
		t.Fatalf("duplicate changed remote artifact count: %d", count)
	}
}

func TestRabbitVRFDKGTransportV1RemoteStoreRejectsConflict(
	t *testing.T,
) {
	transport, packet :=
		newRabbitVRFDKGTransportArtifactFixtureV1(t)

	inserted, err :=
		transport.storeRemoteTransportArtifactV1(packet)
	if err != nil || !inserted {
		t.Fatalf("first insert failed: inserted=%v err=%v", inserted, err)
	}

	transport.mu.Lock()
	conflicting := transport.remoteArtifacts[packet.Binding.ShareID]
	conflicting.Binding.PublicKey[0] ^= 0x01
	transport.remoteArtifacts[packet.Binding.ShareID] = conflicting
	transport.mu.Unlock()

	inserted, err =
		transport.storeRemoteTransportArtifactV1(packet)
	if inserted {
		t.Fatal("conflicting remote artifact reported inserted")
	}
	if !errors.Is(err, errRabbitVRFDKGArtifactConflict) {
		t.Fatalf("unexpected conflict result: %v", err)
	}
}

func TestRabbitVRFDKGTransportV1RemoteStoreDefensiveSignatureCopy(
	t *testing.T,
) {
	transport, packet :=
		newRabbitVRFDKGTransportArtifactFixtureV1(t)

	if len(packet.Envelope.Signature) == 0 {
		t.Fatal("fixture signature is empty")
	}
	original := packet.Envelope.Signature[0]

	inserted, err :=
		transport.storeRemoteTransportArtifactV1(packet)
	if err != nil || !inserted {
		t.Fatalf("insert failed: inserted=%v err=%v", inserted, err)
	}

	packet.Envelope.Signature[0] ^= 0x01

	stored, ok := transport.remoteTransportArtifactV1(
		packet.Binding.SessionID,
		packet.Binding.ShareID,
	)
	if !ok {
		t.Fatal("stored remote artifact not found")
	}
	if stored.Envelope.Signature[0] != original {
		t.Fatal("caller mutation changed stored signature")
	}

	stored.Envelope.Signature[0] ^= 0x01

	again, ok := transport.remoteTransportArtifactV1(
		packet.Binding.SessionID,
		packet.Binding.ShareID,
	)
	if !ok {
		t.Fatal("stored remote artifact disappeared")
	}
	if again.Envelope.Signature[0] != original {
		t.Fatal("returned mutation changed stored signature")
	}
}

func TestRabbitVRFDKGTransportV1RemoteStoreClearsOnCanonicalSessionChange(
	t *testing.T,
) {
	transport, packet :=
		newRabbitVRFDKGTransportArtifactFixtureV1(t)

	inserted, err :=
		transport.storeRemoteTransportArtifactV1(packet)
	if err != nil || !inserted {
		t.Fatalf(
			"initial remote insert failed: inserted=%v err=%v",
			inserted,
			err,
		)
	}

	oldSession := packet.Binding.SessionID

	newContext, err := lqc.NewRabbitVRFDKGSessionContextV1(
		big.NewInt(9280),
		12,
		crypto.Keccak256Hash(
			[]byte("rabbit-vrf-p2p-next-session-committee"),
		),
		32,
	)
	if err != nil {
		t.Fatal(err)
	}

	newSession, err := lqc.RabbitVRFDKGSessionIDV1(newContext)
	if err != nil {
		t.Fatal(err)
	}
	if newSession == oldSession {
		t.Fatal("session rotation did not change session ID")
	}

	transport.runtime.mu.Lock()
	transport.runtime.current.SessionID = newSession
	transport.runtime.current.CanonicalSession = newContext
	transport.runtime.mu.Unlock()

	if _, ok := transport.remoteTransportArtifactV1(
		oldSession,
		packet.Binding.ShareID,
	); ok {
		t.Fatal("old-session artifact survived canonical session change")
	}

	transport.mu.RLock()
	reconciledSession := transport.remoteSession
	count := len(transport.remoteArtifacts)
	transport.mu.RUnlock()

	if reconciledSession != newSession {
		t.Fatal("remote store did not reconcile to canonical session")
	}
	if count != 0 {
		t.Fatalf("old remote artifacts survived session rotation: %d", count)
	}

	if _, ok := transport.remoteTransportArtifactV1(
		newSession,
		packet.Binding.ShareID,
	); ok {
		t.Fatal("old artifact appeared under new session")
	}
}

func TestRabbitVRFDKGTransportV1PeerSendDeduplicates(
	t *testing.T,
) {
	_, packet :=
		newRabbitVRFDKGTransportArtifactFixtureV1(t)

	leftRW, rightRW := p2p.MsgPipe()
	defer leftRW.Close()
	defer rightRW.Close()

	peer := &rabbitVRFDKGPeer{
		rw:    leftRW,
		known: make(map[common.Hash]struct{}),
	}

	firstResult := make(chan error, 1)
	go func() {
		firstResult <- peer.sendTransportArtifactV1(packet)
	}()

	message, err := rightRW.ReadMsg()
	if err != nil {
		t.Fatal(err)
	}
	if message.Code != rabbitVRFDKGTransportArtifactMsg {
		t.Fatalf("unexpected message code: %d", message.Code)
	}

	var received rabbitVRFDKGTransportArtifactPacketV1
	if err := message.Decode(&received); err != nil {
		t.Fatal(err)
	}

	if received.Binding != packet.Binding {
		t.Fatal("received binding differs from sent binding")
	}
	if string(received.Envelope.Signature) !=
		string(packet.Envelope.Signature) {
		t.Fatal("received envelope signature differs")
	}

	select {
	case err := <-firstResult:
		if err != nil {
			t.Fatalf("first artifact send failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("first artifact send timed out")
	}

	if err := peer.sendTransportArtifactV1(packet); err != nil {
		t.Fatalf("duplicate artifact send failed: %v", err)
	}

	peer.mu.Lock()
	knownCount := len(peer.known)
	_, known := peer.known[packet.Envelope.PayloadHash]
	peer.mu.Unlock()

	if !known {
		t.Fatal("sent artifact was not marked known")
	}
	if knownCount != 1 {
		t.Fatalf("unexpected known artifact count: %d", knownCount)
	}

	secondRead := make(chan error, 1)
	go func() {
		_, err := rightRW.ReadMsg()
		secondRead <- err
	}()

	select {
	case err := <-secondRead:
		t.Fatalf("duplicate unexpectedly produced a message: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestRabbitVRFDKGTransportV1GossipAToBToCNoEchoNoDuplicate(
	t *testing.T,
) {
	receiver, packet :=
		newRabbitVRFDKGTransportArtifactFixtureV1(t)

	aRW, runResult :=
		startRabbitVRFDKGTransportArtifactWireV1(
			t,
			receiver,
		)

	cLocal, cRemote := p2p.MsgPipe()
	defer cLocal.Close()
	defer cRemote.Close()

	cPeer := &rabbitVRFDKGPeer{
		peer: p2p.NewPeerPipe(
			enode.ID{11},
			"rabbit-vrf-c",
			nil,
			cLocal,
		),
		rw:    cLocal,
		known: make(map[common.Hash]struct{}),
	}

	if err := receiver.register(cPeer); err != nil {
		t.Fatal(err)
	}
	defer receiver.unregister(cPeer.id())

	if err := p2p.Send(
		aRW,
		rabbitVRFDKGTransportArtifactMsg,
		packet,
	); err != nil {
		t.Fatal(err)
	}

	cFirst := make(chan rabbitVRFDKGTransportArtifactPacketV1, 1)
	cFirstErr := make(chan error, 1)

	go func() {
		message, err := cRemote.ReadMsg()
		if err != nil {
			cFirstErr <- err
			return
		}
		if message.Code != rabbitVRFDKGTransportArtifactMsg {
			cFirstErr <- fmt.Errorf(
				"unexpected C message code: %d",
				message.Code,
			)
			return
		}

		var got rabbitVRFDKGTransportArtifactPacketV1
		if err := message.Decode(&got); err != nil {
			cFirstErr <- err
			return
		}
		cFirst <- got
	}()

	select {
	case err := <-cFirstErr:
		t.Fatalf("B to C gossip failed: %v", err)
	case got := <-cFirst:
		if got.Binding != packet.Binding {
			t.Fatal("C received wrong binding")
		}
		if string(got.Envelope.Signature) !=
			string(packet.Envelope.Signature) {
			t.Fatal("C received wrong envelope signature")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("B to C gossip timed out")
	}

	aEcho := make(chan error, 1)
	go func() {
		message, err := aRW.ReadMsg()
		if err == nil {
			err = fmt.Errorf(
				"unexpected echo message code: %d",
				message.Code,
			)
		}
		aEcho <- err
	}()

	select {
	case err := <-aEcho:
		t.Fatalf("artifact echoed to origin A: %v", err)
	case err := <-runResult:
		t.Fatalf("B receive loop stopped unexpectedly: %v", err)
	case <-time.After(150 * time.Millisecond):
	}

	if err := p2p.Send(
		aRW,
		rabbitVRFDKGTransportArtifactMsg,
		packet,
	); err != nil {
		t.Fatal(err)
	}

	cDuplicate := make(chan error, 1)
	go func() {
		message, err := cRemote.ReadMsg()
		if err == nil {
			err = fmt.Errorf(
				"unexpected duplicate message code: %d",
				message.Code,
			)
		}
		cDuplicate <- err
	}()

	select {
	case err := <-cDuplicate:
		t.Fatalf("duplicate was retransmitted to C: %v", err)
	case err := <-runResult:
		t.Fatalf("B receive loop stopped after duplicate: %v", err)
	case <-time.After(150 * time.Millisecond):
	}

	stored, ok := receiver.remoteTransportArtifactV1(
		packet.Binding.SessionID,
		packet.Binding.ShareID,
	)
	if !ok {
		t.Fatal("B did not retain received artifact")
	}
	if stored.Binding != packet.Binding {
		t.Fatal("B retained wrong artifact")
	}
}

func TestRabbitVRFDKGTransportV1NewPeerReceivesLocalArtifact(
	t *testing.T,
) {
	transport, packet :=
		newRabbitVRFDKGTransportArtifactFixtureV1(t)

	context := transport.runtime.currentContext()
	if len(context.CanonicalMembers) != 1 {
		t.Fatalf(
			"canonical members=%d want=1",
			len(context.CanonicalMembers),
		)
	}

	member := context.CanonicalMembers[0]

	transport.runtime.mu.Lock()
	transport.runtime.current.Members =
		[]lqc.RabbitVRFCommitteeMemberV1{member}
	transport.runtime.current.TransportBindings =
		[]lqc.RabbitVRFDKGTransportKeyBindingV1{packet.Binding}
	transport.runtime.current.TransportEnvelopes =
		[]lqc.RabbitVRFDKGEnvelopeV1{
			cloneRabbitVRFDKGTransportArtifactPacketV1(
				packet,
			).Envelope,
		}
	transport.runtime.mu.Unlock()

	if !transport.runtime.transportArtifactsReadyV1(
		context.CanonicalSession,
		context.SessionID,
		[]lqc.RabbitVRFCommitteeMemberV1{member},
	) {
		t.Fatal("local transport artifacts not ready")
	}

	remoteRW, runResult :=
		startRabbitVRFDKGTransportArtifactWireV1(
			t,
			transport,
		)

	received := make(
		chan rabbitVRFDKGTransportArtifactPacketV1,
		1,
	)
	receiveErr := make(chan error, 1)

	go func() {
		message, err := remoteRW.ReadMsg()
		if err != nil {
			receiveErr <- err
			return
		}
		if message.Code != rabbitVRFDKGTransportArtifactMsg {
			receiveErr <- fmt.Errorf(
				"unexpected initial sync message code: %d",
				message.Code,
			)
			return
		}

		var got rabbitVRFDKGTransportArtifactPacketV1
		if err := message.Decode(&got); err != nil {
			receiveErr <- err
			return
		}
		received <- got
	}()

	select {
	case err := <-receiveErr:
		t.Fatalf("initial artifact sync failed: %v", err)
	case err := <-runResult:
		t.Fatalf("receive loop stopped during initial sync: %v", err)
	case got := <-received:
		if got.Binding != packet.Binding {
			t.Fatal("new peer received wrong local binding")
		}
		if string(got.Envelope.Signature) !=
			string(packet.Envelope.Signature) {
			t.Fatal("new peer received wrong local envelope")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("new peer initial artifact sync timed out")
	}
}

func TestRabbitVRFDKGTransportV1NewPeerRejectsTamperedLocalArtifact(
	t *testing.T,
) {
	transport, packet :=
		newRabbitVRFDKGTransportArtifactFixtureV1(t)

	context := transport.runtime.currentContext()
	if len(context.CanonicalMembers) != 1 {
		t.Fatalf(
			"canonical members=%d want=1",
			len(context.CanonicalMembers),
		)
	}

	member := context.CanonicalMembers[0]
	tampered :=
		cloneRabbitVRFDKGTransportArtifactPacketV1(packet)
	if len(tampered.Envelope.Signature) == 0 {
		t.Fatal("fixture produced empty signature")
	}
	tampered.Envelope.Signature[0] ^= 0x01

	transport.runtime.mu.Lock()
	transport.runtime.current.Members =
		[]lqc.RabbitVRFCommitteeMemberV1{member}
	transport.runtime.current.TransportBindings =
		[]lqc.RabbitVRFDKGTransportKeyBindingV1{
			tampered.Binding,
		}
	transport.runtime.current.TransportEnvelopes =
		[]lqc.RabbitVRFDKGEnvelopeV1{
			tampered.Envelope,
		}
	transport.runtime.mu.Unlock()

	if transport.runtime.transportArtifactsReadyV1(
		context.CanonicalSession,
		context.SessionID,
		[]lqc.RabbitVRFCommitteeMemberV1{member},
	) {
		t.Fatal("tampered local artifact considered ready")
	}

	remoteRW, runResult :=
		startRabbitVRFDKGTransportArtifactWireV1(
			t,
			transport,
		)

	unexpected := make(chan error, 1)
	go func() {
		message, err := remoteRW.ReadMsg()
		if err == nil {
			err = fmt.Errorf(
				"unexpected published message code: %d",
				message.Code,
			)
		}
		unexpected <- err
	}()

	select {
	case err := <-unexpected:
		t.Fatalf(
			"tampered local artifact was published: %v",
			err,
		)
	case err := <-runResult:
		t.Fatalf(
			"receive loop stopped unexpectedly: %v",
			err,
		)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestRabbitVRFDKGTransportV1StaleSessionDoesNotDisconnectPeer(
	t *testing.T,
) {
	transport, packet :=
		newRabbitVRFDKGTransportArtifactFixtureV1(t)

	remoteRW, runResult :=
		startRabbitVRFDKGTransportArtifactWireV1(
			t,
			transport,
		)

	stale :=
		cloneRabbitVRFDKGTransportArtifactPacketV1(packet)
	staleSession := crypto.Keccak256Hash(
		[]byte("rabbit-vrf-stale-session"),
	)
	if staleSession == packet.Binding.SessionID {
		t.Fatal("stale session unexpectedly equals canonical session")
	}

	stale.Binding.SessionID = staleSession
	stale.Envelope.SessionID = staleSession

	if err := p2p.Send(
		remoteRW,
		rabbitVRFDKGTransportArtifactMsg,
		stale,
	); err != nil {
		t.Fatalf("send stale artifact: %v", err)
	}

	select {
	case err := <-runResult:
		t.Fatalf(
			"stale session disconnected peer: %v",
			err,
		)
	case <-time.After(100 * time.Millisecond):
	}

	if _, ok := transport.remoteTransportArtifactV1(
		staleSession,
		stale.Binding.ShareID,
	); ok {
		t.Fatal("stale-session artifact was retained")
	}

	if err := p2p.Send(
		remoteRW,
		rabbitVRFDKGTransportArtifactMsg,
		packet,
	); err != nil {
		t.Fatalf("send valid artifact after stale: %v", err)
	}

	deadline := time.After(2 * time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case err := <-runResult:
			t.Fatalf(
				"peer disconnected before valid artifact was retained: %v",
				err,
			)
		case <-ticker.C:
			stored, ok := transport.remoteTransportArtifactV1(
				packet.Binding.SessionID,
				packet.Binding.ShareID,
			)
			if ok {
				if stored.Binding != packet.Binding {
					t.Fatal("wrong artifact retained after stale packet")
				}
				return
			}
		case <-deadline:
			t.Fatal("valid artifact after stale session was not retained")
		}
	}
}
