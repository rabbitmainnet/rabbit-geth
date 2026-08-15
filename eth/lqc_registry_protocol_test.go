package eth

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/eth/ethconfig"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/enode"
)

func TestLQCRegistryPeerSendsEachOperationOnce(t *testing.T) {
	left, right := p2p.MsgPipe()
	defer left.Close()
	defer right.Close()
	peer := &lqcRegistryPeer{
		peer:  p2p.NewPeer(enode.ID{1}, "registry-test", nil),
		rw:    left,
		known: make(map[common.Hash]struct{}),
	}
	operation := lqc.RegistryOperation{
		Version:    lqc.RegistryProtocolVersion,
		Action:     lqc.RegistryActionRegister,
		Address:    common.HexToAddress("0x1000000000000000000000000000000000000001"),
		Sequence:   1,
		ValidUntil: 20,
		Signature:  make([]byte, 65),
	}
	done := make(chan error, 1)
	go func() { done <- peer.sendOperations(big.NewInt(928), []lqc.RegistryOperation{operation}) }()
	message, err := right.ReadMsg()
	if err != nil {
		t.Fatal(err)
	}
	if message.Code != lqcRegistryOperationsMsg {
		t.Fatalf("message code = %d, want %d", message.Code, lqcRegistryOperationsMsg)
	}
	var received []lqc.RegistryOperation
	if err := message.Decode(&received); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if len(received) != 1 || received[0].Address != operation.Address {
		t.Fatalf("received operations = %+v", received)
	}
	if err := peer.sendOperations(big.NewInt(928), []lqc.RegistryOperation{operation}); err != nil {
		t.Fatal(err)
	}
	if len(peer.known) != 1 {
		t.Fatalf("known operations = %d, want 1", len(peer.known))
	}
}

func TestLQCRegistryNetworkRejectsDuplicatePeer(t *testing.T) {
	network := &lqcRegistryNetwork{peers: make(map[string]*lqcRegistryPeer)}
	peer := &lqcRegistryPeer{peer: p2p.NewPeer(enode.ID{2}, "registry-test", nil)}
	if err := network.register(peer); err != nil {
		t.Fatal(err)
	}
	if err := network.register(peer); err != errLQCRegistryPeerKnown {
		t.Fatalf("duplicate error = %v, want %v", err, errLQCRegistryPeerKnown)
	}
	network.Close()
	other := &lqcRegistryPeer{peer: p2p.NewPeer(enode.ID{3}, "registry-test", nil)}
	if err := network.register(other); err != errLQCRegistryProtocolClosed {
		t.Fatalf("closed error = %v, want %v", err, errLQCRegistryProtocolClosed)
	}
}

func TestLQCRegistryHandshakeChecksNetworkAndGenesis(t *testing.T) {
	backend := newTestHandler(ethconfig.FullSync)
	defer backend.close()
	tests := []struct {
		name       string
		leftNet    uint64
		rightNet   uint64
		wantErrors bool
	}{
		{name: "matching", leftNet: 928, rightNet: 928},
		{name: "network mismatch", leftNet: 928, rightNet: 929, wantErrors: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			leftRW, rightRW := p2p.MsgPipe()
			defer leftRW.Close()
			defer rightRW.Close()
			leftNetwork := newLQCRegistryNetwork(nil, backend.chain, test.leftNet)
			rightNetwork := newLQCRegistryNetwork(nil, backend.chain, test.rightNet)
			leftPeer := &lqcRegistryPeer{peer: p2p.NewPeer(enode.ID{4}, "left", nil), rw: leftRW, known: make(map[common.Hash]struct{})}
			rightPeer := &lqcRegistryPeer{peer: p2p.NewPeer(enode.ID{5}, "right", nil), rw: rightRW, known: make(map[common.Hash]struct{})}
			results := make(chan error, 2)
			go func() { results <- leftNetwork.handshake(leftPeer) }()
			go func() { results <- rightNetwork.handshake(rightPeer) }()
			leftErr, rightErr := <-results, <-results
			if test.wantErrors {
				if leftErr == nil || rightErr == nil {
					t.Fatalf("mismatched handshake errors: left=%v right=%v", leftErr, rightErr)
				}
			} else if leftErr != nil || rightErr != nil {
				t.Fatalf("matching handshake failed: left=%v right=%v", leftErr, rightErr)
			}
		})
	}
}
