package eth

import (
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/p2p"
)

const (
	lqcRegistryProtocolName    = "lqcr"
	lqcRegistryProtocolVersion = uint(1)
	lqcRegistryProtocolLength  = uint64(2)

	lqcRegistryStatusMsg     = uint64(0)
	lqcRegistryOperationsMsg = uint64(1)

	lqcRegistryHandshakeTimeout = 5 * time.Second
	lqcRegistryMaxMessageSize   = 256 * 1024
	lqcRegistryMaxKnownPerPeer  = 4096
)

var (
	errLQCRegistryProtocolClosed = errors.New("lqc registry protocol closed")
	errLQCRegistryPeerKnown      = errors.New("lqc registry peer already connected")
)

type lqcRegistryStatusPacket struct {
	ProtocolVersion uint32
	NetworkID       uint64
	Genesis         common.Hash
}

type lqcRegistryNetwork struct {
	engine    *lqc.LQC
	chain     *core.BlockChain
	networkID uint64

	mu     sync.RWMutex
	peers  map[string]*lqcRegistryPeer
	closed bool
}

type lqcRegistryPeer struct {
	peer *p2p.Peer
	rw   p2p.MsgReadWriter

	mu    sync.Mutex
	known map[common.Hash]struct{}
}

func newLQCRegistryNetwork(engine *lqc.LQC, chain *core.BlockChain, networkID uint64) *lqcRegistryNetwork {
	return &lqcRegistryNetwork{
		engine:    engine,
		chain:     chain,
		networkID: networkID,
		peers:     make(map[string]*lqcRegistryPeer),
	}
}

func (n *lqcRegistryNetwork) Protocol() p2p.Protocol {
	return p2p.Protocol{
		Name:    lqcRegistryProtocolName,
		Version: lqcRegistryProtocolVersion,
		Length:  lqcRegistryProtocolLength,
		Run: func(remote *p2p.Peer, rw p2p.MsgReadWriter) error {
			return n.runPeer(remote, rw)
		},
		NodeInfo: func() interface{} {
			return lqcRegistryStatusPacket{
				ProtocolVersion: uint32(lqcRegistryProtocolVersion),
				NetworkID:       n.networkID,
				Genesis:         n.chain.Genesis().Hash(),
			}
		},
	}
}

func (n *lqcRegistryNetwork) runPeer(remote *p2p.Peer, rw p2p.MsgReadWriter) error {
	peer := &lqcRegistryPeer{peer: remote, rw: rw, known: make(map[common.Hash]struct{})}
	if err := n.handshake(peer); err != nil {
		return err
	}
	if err := n.register(peer); err != nil {
		return err
	}
	defer n.unregister(peer.id())

	go n.sendPending(peer)
	for {
		message, err := rw.ReadMsg()
		if err != nil {
			return err
		}
		if message.Size > lqcRegistryMaxMessageSize {
			return fmt.Errorf("lqc registry message too large: %d", message.Size)
		}
		switch message.Code {
		case lqcRegistryOperationsMsg:
			var operations []lqc.RegistryOperation
			if err := message.Decode(&operations); err != nil {
				return err
			}
			if len(operations) == 0 || len(operations) > MaxRegistryOperationsPerPacket {
				return fmt.Errorf("invalid lqc registry operation packet size: %d", len(operations))
			}
			accepted := make([]lqc.RegistryOperation, 0, len(operations))
			for _, operation := range operations {
				hash, err := n.engine.SubmitRegistryOperation(n.chain, operation)
				if err != nil {
					if errors.Is(err, lqc.ErrRegistryOperationKnown) {
						peer.markKnown(hash)
					}
					continue
				}
				peer.markKnown(hash)
				accepted = append(accepted, operation)
			}
			if len(accepted) > 0 {
				n.BroadcastOperations(accepted, peer.id())
			}
		default:
			return fmt.Errorf("invalid lqc registry message code: %d", message.Code)
		}
	}
}

func (n *lqcRegistryNetwork) sendPending(peer *lqcRegistryPeer) {
	pending := n.engine.PendingRegistryOperations(n.chain)
	for len(pending) > 0 {
		limit := min(len(pending), MaxRegistryOperationsPerPacket)
		if err := peer.sendOperations(n.chain.Config().ChainID, pending[:limit]); err != nil {
			peer.peer.Log().Debug("LQC registry initial pool sync failed", "err", err)
			return
		}
		pending = pending[limit:]
	}
}

const MaxRegistryOperationsPerPacket = lqc.MaxRegistryOperationsPerBlock

func (n *lqcRegistryNetwork) handshake(peer *lqcRegistryPeer) error {
	status := lqcRegistryStatusPacket{
		ProtocolVersion: uint32(lqcRegistryProtocolVersion),
		NetworkID:       n.networkID,
		Genesis:         n.chain.Genesis().Hash(),
	}
	errorsCh := make(chan error, 2)
	go func() { errorsCh <- p2p.Send(peer.rw, lqcRegistryStatusMsg, status) }()
	go func() {
		message, err := peer.rw.ReadMsg()
		if err != nil {
			errorsCh <- err
			return
		}
		if message.Code != lqcRegistryStatusMsg || message.Size > lqcRegistryMaxMessageSize {
			errorsCh <- errors.New("invalid lqc registry handshake message")
			return
		}
		var remote lqcRegistryStatusPacket
		if err := message.Decode(&remote); err != nil {
			errorsCh <- err
			return
		}
		switch {
		case remote.ProtocolVersion != status.ProtocolVersion:
			errorsCh <- errors.New("lqc registry protocol version mismatch")
		case remote.NetworkID != status.NetworkID:
			errorsCh <- errors.New("lqc registry network ID mismatch")
		case remote.Genesis != status.Genesis:
			errorsCh <- errors.New("lqc registry genesis mismatch")
		default:
			errorsCh <- nil
		}
	}()
	timer := time.NewTimer(lqcRegistryHandshakeTimeout)
	defer timer.Stop()
	for range 2 {
		select {
		case err := <-errorsCh:
			if err != nil {
				return err
			}
		case <-timer.C:
			return p2p.DiscReadTimeout
		}
	}
	return nil
}

func (n *lqcRegistryNetwork) register(peer *lqcRegistryPeer) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.closed {
		return errLQCRegistryProtocolClosed
	}
	if _, exists := n.peers[peer.id()]; exists {
		return errLQCRegistryPeerKnown
	}
	n.peers[peer.id()] = peer
	return nil
}

func (n *lqcRegistryNetwork) unregister(id string) {
	n.mu.Lock()
	delete(n.peers, id)
	n.mu.Unlock()
}

func (n *lqcRegistryNetwork) BroadcastOperations(operations []lqc.RegistryOperation, except string) {
	if n == nil || len(operations) == 0 {
		return
	}
	n.mu.RLock()
	peers := make([]*lqcRegistryPeer, 0, len(n.peers))
	for id, peer := range n.peers {
		if id != except {
			peers = append(peers, peer)
		}
	}
	n.mu.RUnlock()
	chainID := n.chain.Config().ChainID
	for _, peer := range peers {
		peer := peer
		go func() {
			if err := peer.sendOperations(chainID, operations); err != nil {
				peer.peer.Log().Debug("LQC registry operation broadcast failed", "err", err)
			}
		}()
	}
}

func (n *lqcRegistryNetwork) Close() {
	if n == nil {
		return
	}
	n.mu.Lock()
	n.closed = true
	for _, peer := range n.peers {
		peer.peer.Disconnect(p2p.DiscQuitting)
	}
	n.mu.Unlock()
}

func (p *lqcRegistryPeer) id() string {
	return p.peer.ID().String()
}

func (p *lqcRegistryPeer) markKnown(hash common.Hash) {
	p.mu.Lock()
	if len(p.known) >= lqcRegistryMaxKnownPerPeer {
		clear(p.known)
	}
	p.known[hash] = struct{}{}
	p.mu.Unlock()
}

func (p *lqcRegistryPeer) sendOperations(chainID *big.Int, operations []lqc.RegistryOperation) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	unknown := make([]lqc.RegistryOperation, 0, len(operations))
	for _, operation := range operations {
		hash := lqc.RegistryOperationHash(chainID, operation)
		if _, exists := p.known[hash]; exists {
			continue
		}
		if len(p.known) >= lqcRegistryMaxKnownPerPeer {
			clear(p.known)
		}
		p.known[hash] = struct{}{}
		unknown = append(unknown, operation)
	}
	if len(unknown) == 0 {
		return nil
	}
	return p2p.Send(p.rw, lqcRegistryOperationsMsg, unknown)
}
