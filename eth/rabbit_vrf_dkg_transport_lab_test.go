//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"errors"
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
