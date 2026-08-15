package eth

import (
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/enode"
)

const workTicketTransportTestKey = "4f3edf983ac63ad7cdbda274f0e29202454b9f6f746b90b5d7a0a8f08c82e55b"

func testWorkTicketTransportConfig() lqcWorkTicketTransportConfig {
	return lqcWorkTicketTransportConfig{
		Laboratory: true,
		ChainID:    big.NewInt(928),
		NetworkID:  928,
		Genesis:    crypto.Keccak256Hash([]byte("isolated-work-ticket-lab-genesis")),
	}
}

func signedTransportWorkTicket(t *testing.T, chainID *big.Int, sequence uint64, previous common.Hash) lqc.WorkTicket {
	t.Helper()
	key, err := crypto.HexToECDSA(workTicketTransportTestKey)
	if err != nil {
		t.Fatal(err)
	}
	participant := crypto.PubkeyToAddress(key.PublicKey)
	anchor := crypto.Keccak256Hash([]byte("work-ticket-transport-anchor"))
	if previous == (common.Hash{}) {
		previous = lqc.InitialWorkTicketPrevious(chainID, anchor, 7, participant)
	}
	ticket := lqc.WorkTicket{
		Version:     lqc.WorkTicketProtocolVersion,
		Epoch:       7,
		Anchor:      anchor,
		Participant: participant,
		Sequence:    sequence,
		Previous:    previous,
	}
	ticket.Proof, err = lqc.GenerateWorkTicketProof(chainID, ticket)
	if err != nil {
		t.Fatal(err)
	}
	hash := lqc.WorkTicketSigningHash(chainID, ticket)
	ticket.Signature, err = crypto.Sign(hash[:], key)
	if err != nil {
		t.Fatal(err)
	}
	return ticket
}

func TestLQCWorkTicketTransportRequiresExplicitLaboratoryMode(t *testing.T) {
	config := testWorkTicketTransportConfig()
	config.Laboratory = false
	if _, err := newLQCWorkTicketTransport(config); !errors.Is(err, errLQCWorkTicketTransportDisabled) {
		t.Fatalf("disabled error = %v", err)
	}
	config = testWorkTicketTransportConfig()
	config.ChainID = nil
	if _, err := newLQCWorkTicketTransport(config); !errors.Is(err, lqc.ErrInvalidWorkTicketChain) {
		t.Fatalf("chain error = %v", err)
	}
	config = testWorkTicketTransportConfig()
	config.Genesis = common.Hash{}
	if _, err := newLQCWorkTicketTransport(config); err == nil {
		t.Fatal("zero genesis accepted")
	}
}

func TestLQCWorkTicketTransportProtocolIsBounded(t *testing.T) {
	transport, err := newLQCWorkTicketTransport(testWorkTicketTransportConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	protocol := transport.Protocol()
	if protocol.Name != "lqct" || protocol.Version != 1 || protocol.Length != 2 {
		t.Fatalf("protocol = %+v", protocol)
	}
	if MaxWorkTicketsPerPacket != 8 || lqcWorkTicketMaxMessageSize != 8*1024 || lqcWorkTicketBudgetTickets != 64 || lqcWorkTicketGlobalBudget != 128 {
		t.Fatalf("unsafe transport bounds: packet=%d message=%d peer=%d global=%d", MaxWorkTicketsPerPacket, lqcWorkTicketMaxMessageSize, lqcWorkTicketBudgetTickets, lqcWorkTicketGlobalBudget)
	}
}

func TestLQCWorkTicketTransportRejectsDuplicatePeerAndClosedNetwork(t *testing.T) {
	transport, err := newLQCWorkTicketTransport(testWorkTicketTransportConfig())
	if err != nil {
		t.Fatal(err)
	}
	peer := &lqcWorkTicketPeer{peer: p2p.NewPeer(enode.ID{9}, "ticket-test", nil)}
	if err := transport.register(peer); err != nil {
		t.Fatal(err)
	}
	if err := transport.register(peer); !errors.Is(err, errLQCWorkTicketPeerKnown) {
		t.Fatalf("duplicate peer error = %v", err)
	}
	transport.Close()
	other := &lqcWorkTicketPeer{peer: p2p.NewPeer(enode.ID{10}, "ticket-test", nil)}
	if err := transport.register(other); !errors.Is(err, errLQCWorkTicketProtocolClosed) {
		t.Fatalf("closed network error = %v", err)
	}
}

func TestLQCWorkTicketPeerSendsEachTicketOnce(t *testing.T) {
	left, right := p2p.MsgPipe()
	defer left.Close()
	defer right.Close()
	peer := &lqcWorkTicketPeer{
		peer:  p2p.NewPeer(enode.ID{1}, "ticket-test", nil),
		rw:    left,
		known: make(map[common.Hash]struct{}),
	}
	chainID := big.NewInt(928)
	ticket := signedTransportWorkTicket(t, chainID, 1, common.Hash{})
	done := make(chan error, 1)
	go func() { done <- peer.sendTickets(chainID, []lqc.WorkTicket{ticket}) }()
	message, err := right.ReadMsg()
	if err != nil {
		t.Fatal(err)
	}
	if message.Code != lqcWorkTicketTicketsMsg {
		t.Fatalf("message code = %d", message.Code)
	}
	var received []lqc.WorkTicket
	if err := message.Decode(&received); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if len(received) != 1 || received[0].Proof != ticket.Proof {
		t.Fatalf("received tickets = %+v", received)
	}
	if err := peer.sendTickets(chainID, []lqc.WorkTicket{ticket}); err != nil {
		t.Fatal(err)
	}
	if len(peer.known) != 1 {
		t.Fatalf("known tickets = %d, want 1", len(peer.known))
	}
}

func TestLQCWorkTicketVerificationBudget(t *testing.T) {
	peer := &lqcWorkTicketPeer{known: make(map[common.Hash]struct{})}
	now := time.Unix(100, 0)
	for range lqcWorkTicketBudgetTickets / MaxWorkTicketsPerPacket {
		if !peer.consumeVerificationBudget(MaxWorkTicketsPerPacket, now) {
			t.Fatal("budget rejected an allowed packet")
		}
	}
	if peer.consumeVerificationBudget(1, now) {
		t.Fatal("budget allowed work above the window limit")
	}
	if !peer.consumeVerificationBudget(MaxWorkTicketsPerPacket, now.Add(lqcWorkTicketBudgetWindow)) {
		t.Fatal("budget did not reset after its window")
	}
}

func TestLQCWorkTicketGlobalVerificationBudget(t *testing.T) {
	transport, err := newLQCWorkTicketTransport(testWorkTicketTransportConfig())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(200, 0)
	for range lqcWorkTicketGlobalBudget / MaxWorkTicketsPerPacket {
		if !transport.consumeVerificationBudget(MaxWorkTicketsPerPacket, now) {
			t.Fatal("global budget rejected an allowed packet")
		}
	}
	if transport.consumeVerificationBudget(1, now) {
		t.Fatal("global budget allowed work above the window limit")
	}
	if !transport.consumeVerificationBudget(MaxWorkTicketsPerPacket, now.Add(lqcWorkTicketBudgetWindow)) {
		t.Fatal("global budget did not reset after its window")
	}
}

func TestLQCWorkTicketHandshakeChecksNetworkGenesisAndChain(t *testing.T) {
	tests := []struct {
		name   string
		change func(*lqcWorkTicketTransportConfig)
		fail   bool
	}{
		{name: "matching"},
		{name: "network mismatch", fail: true, change: func(config *lqcWorkTicketTransportConfig) { config.NetworkID++ }},
		{name: "genesis mismatch", fail: true, change: func(config *lqcWorkTicketTransportConfig) { config.Genesis[0] ^= 1 }},
		{name: "chain mismatch", fail: true, change: func(config *lqcWorkTicketTransportConfig) { config.ChainID = big.NewInt(929) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			leftConfig := testWorkTicketTransportConfig()
			rightConfig := testWorkTicketTransportConfig()
			if test.change != nil {
				test.change(&rightConfig)
			}
			leftNetwork, err := newLQCWorkTicketTransport(leftConfig)
			if err != nil {
				t.Fatal(err)
			}
			rightNetwork, err := newLQCWorkTicketTransport(rightConfig)
			if err != nil {
				t.Fatal(err)
			}
			leftRW, rightRW := p2p.MsgPipe()
			defer leftRW.Close()
			defer rightRW.Close()
			leftPeer := &lqcWorkTicketPeer{peer: p2p.NewPeer(enode.ID{2}, "left", nil), rw: leftRW, known: make(map[common.Hash]struct{})}
			rightPeer := &lqcWorkTicketPeer{peer: p2p.NewPeer(enode.ID{3}, "right", nil), rw: rightRW, known: make(map[common.Hash]struct{})}
			results := make(chan error, 2)
			go func() { results <- leftNetwork.handshake(leftPeer) }()
			go func() { results <- rightNetwork.handshake(rightPeer) }()
			first, second := <-results, <-results
			if test.fail && (first == nil || second == nil) {
				t.Fatalf("mismatch accepted: first=%v second=%v", first, second)
			}
			if !test.fail && (first != nil || second != nil) {
				t.Fatalf("matching handshake failed: first=%v second=%v", first, second)
			}
		})
	}
}

func TestLQCWorkTicketTransportAPIValidatesAndRetainsSignedTicket(t *testing.T) {
	transport, err := newLQCWorkTicketTransport(testWorkTicketTransportConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	api := newLQCWorkTicketTransportAPI(transport)
	ticket := signedTransportWorkTicket(t, transport.chainID, 1, common.Hash{})
	args := WorkTicketArgs{
		Version:     hexutil.Uint64(ticket.Version),
		Epoch:       hexutil.Uint64(ticket.Epoch),
		Anchor:      ticket.Anchor,
		Participant: ticket.Participant,
		Sequence:    hexutil.Uint64(ticket.Sequence),
		Previous:    ticket.Previous,
		Proof:       ticket.Proof,
		Signature:   append(hexutil.Bytes(nil), ticket.Signature...),
	}
	wantHash := lqc.WorkTicketHash(transport.chainID, ticket)
	hash, err := api.SubmitWorkTicket(args)
	if err != nil || hash != wantHash {
		t.Fatalf("submit hash=%s err=%v", hash, err)
	}
	if _, err := api.SubmitWorkTicket(args); !errors.Is(err, lqc.ErrWorkTicketKnown) {
		t.Fatalf("duplicate error = %v", err)
	}
	status, err := api.WorkTicketPoolStatus()
	if err != nil || status.Pending != 1 || status.Participants != 1 {
		t.Fatalf("status=%+v err=%v", status, err)
	}
	pending, err := api.PendingWorkTickets(1)
	if err != nil || len(pending) != 1 || pending[0].Hash != wantHash {
		t.Fatalf("pending=%+v err=%v", pending, err)
	}
	bad := args
	bad.Version = 256
	if _, err := api.SubmitWorkTicket(bad); !errors.Is(err, lqc.ErrInvalidWorkTicketVersion) {
		t.Fatalf("oversized version error = %v", err)
	}
	if _, err := (*LQCWorkTicketTransportAPI)(nil).WorkTicketPoolStatus(); !errors.Is(err, errLQCWorkTicketTransportUnavailable) {
		t.Fatalf("nil API error = %v", err)
	}
}
