//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
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
