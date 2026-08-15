package eth

import (
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/p2p"
)

const (
	lqcWorkTicketProtocolName    = "lqct"
	lqcWorkTicketProtocolVersion = uint(1)
	lqcWorkTicketProtocolLength  = uint64(2)

	lqcWorkTicketStatusMsg  = uint64(0)
	lqcWorkTicketTicketsMsg = uint64(1)

	lqcWorkTicketHandshakeTimeout = 5 * time.Second
	lqcWorkTicketMaxMessageSize   = 8 * 1024
	lqcWorkTicketMaxKnownPerPeer  = 4096

	// Packets are deliberately smaller than the 64-ticket block envelope. Eight
	// portable Argon2id proofs fit the verified weak-PC budget, while larger
	// relay sets are split into independent packets.
	MaxWorkTicketsPerPacket       = 8
	lqcWorkTicketInitialSyncLimit = 64

	// Invalid proofs are expensive by design. Every received ticket, valid or
	// invalid, consumes this per-peer verification budget before Argon2id work.
	lqcWorkTicketBudgetWindow  = 10 * time.Second
	lqcWorkTicketBudgetTickets = 64
	lqcWorkTicketGlobalBudget  = 128
)

var (
	errLQCWorkTicketTransportDisabled = errors.New("lqc work ticket laboratory transport disabled")
	errLQCWorkTicketProtocolClosed    = errors.New("lqc work ticket protocol closed")
	errLQCWorkTicketPeerKnown         = errors.New("lqc work ticket peer already connected")
	errLQCWorkTicketRateLimited       = errors.New("lqc work ticket peer verification budget exceeded")
)

type lqcWorkTicketTransportConfig struct {
	// Laboratory must be set explicitly by isolated lab wiring. The Ethereum
	// backend never constructs or registers this transport in this foundation.
	Laboratory bool
	ChainID    *big.Int
	NetworkID  uint64
	Genesis    common.Hash
}

type lqcWorkTicketStatusPacket struct {
	ProtocolVersion uint32
	NetworkID       uint64
	Genesis         common.Hash
	ChainID         *big.Int
}

type lqcWorkTicketTransport struct {
	chainID   *big.Int
	networkID uint64
	genesis   common.Hash
	pool      *lqc.WorkTicketPool

	mu     sync.RWMutex
	peers  map[string]*lqcWorkTicketPeer
	closed bool

	verificationMu         sync.Mutex
	globalBudgetWindowFrom time.Time
	globalBudgetUsed       int
}

type lqcWorkTicketPeer struct {
	peer *p2p.Peer
	rw   p2p.MsgReadWriter

	mu               sync.Mutex
	known            map[common.Hash]struct{}
	budgetWindowFrom time.Time
	budgetUsed       int
}

func newLQCWorkTicketTransport(config lqcWorkTicketTransportConfig) (*lqcWorkTicketTransport, error) {
	if !config.Laboratory {
		return nil, errLQCWorkTicketTransportDisabled
	}
	if config.ChainID == nil || config.ChainID.Sign() <= 0 {
		return nil, lqc.ErrInvalidWorkTicketChain
	}
	if config.NetworkID == 0 {
		return nil, errors.New("zero lqc work ticket network ID")
	}
	if config.Genesis == (common.Hash{}) {
		return nil, errors.New("zero lqc work ticket genesis")
	}
	return &lqcWorkTicketTransport{
		chainID:   new(big.Int).Set(config.ChainID),
		networkID: config.NetworkID,
		genesis:   config.Genesis,
		pool:      lqc.NewWorkTicketPool(),
		peers:     make(map[string]*lqcWorkTicketPeer),
	}, nil
}

func (n *lqcWorkTicketTransport) Protocol() p2p.Protocol {
	return p2p.Protocol{
		Name:    lqcWorkTicketProtocolName,
		Version: lqcWorkTicketProtocolVersion,
		Length:  lqcWorkTicketProtocolLength,
		Run: func(remote *p2p.Peer, rw p2p.MsgReadWriter) error {
			return n.runPeer(remote, rw)
		},
		NodeInfo: func() interface{} { return n.status() },
	}
}

func (n *lqcWorkTicketTransport) status() lqcWorkTicketStatusPacket {
	return lqcWorkTicketStatusPacket{
		ProtocolVersion: uint32(lqcWorkTicketProtocolVersion),
		NetworkID:       n.networkID,
		Genesis:         n.genesis,
		ChainID:         new(big.Int).Set(n.chainID),
	}
}

func (n *lqcWorkTicketTransport) runPeer(remote *p2p.Peer, rw p2p.MsgReadWriter) error {
	peer := &lqcWorkTicketPeer{peer: remote, rw: rw, known: make(map[common.Hash]struct{})}
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
		if message.Size > lqcWorkTicketMaxMessageSize {
			return fmt.Errorf("lqc work ticket message too large: %d", message.Size)
		}
		if message.Code != lqcWorkTicketTicketsMsg {
			return fmt.Errorf("invalid lqc work ticket message code: %d", message.Code)
		}
		var tickets []lqc.WorkTicket
		if err := message.Decode(&tickets); err != nil {
			return err
		}
		if len(tickets) == 0 || len(tickets) > MaxWorkTicketsPerPacket {
			return fmt.Errorf("invalid lqc work ticket packet size: %d", len(tickets))
		}
		novel := make([]lqc.WorkTicket, 0, len(tickets))
		for _, ticket := range tickets {
			hash := lqc.WorkTicketHash(n.chainID, ticket)
			if n.pool.Has(hash) {
				peer.markKnown(hash)
				continue
			}
			novel = append(novel, ticket)
		}
		if len(novel) == 0 {
			continue
		}
		now := time.Now()
		if !peer.consumeVerificationBudget(len(novel), now) || !n.consumeVerificationBudget(len(novel), now) {
			return errLQCWorkTicketRateLimited
		}
		accepted := make([]lqc.WorkTicket, 0, len(novel))
		for _, ticket := range novel {
			hash, err := n.pool.Add(n.chainID, ticket)
			if err != nil {
				if errors.Is(err, lqc.ErrWorkTicketKnown) {
					peer.markKnown(hash)
				}
				continue
			}
			peer.markKnown(hash)
			accepted = append(accepted, ticket)
		}
		if len(accepted) > 0 {
			n.BroadcastTickets(accepted, peer.id())
		}
	}
}

func (n *lqcWorkTicketTransport) consumeVerificationBudget(count int, now time.Time) bool {
	n.verificationMu.Lock()
	defer n.verificationMu.Unlock()
	if count <= 0 || count > MaxWorkTicketsPerPacket {
		return false
	}
	if n.globalBudgetWindowFrom.IsZero() || now.Sub(n.globalBudgetWindowFrom) >= lqcWorkTicketBudgetWindow {
		n.globalBudgetWindowFrom = now
		n.globalBudgetUsed = 0
	}
	if n.globalBudgetUsed+count > lqcWorkTicketGlobalBudget {
		return false
	}
	n.globalBudgetUsed += count
	return true
}

func (n *lqcWorkTicketTransport) handshake(peer *lqcWorkTicketPeer) error {
	status := n.status()
	errorsCh := make(chan error, 2)
	go func() { errorsCh <- p2p.Send(peer.rw, lqcWorkTicketStatusMsg, status) }()
	go func() {
		message, err := peer.rw.ReadMsg()
		if err != nil {
			errorsCh <- err
			return
		}
		if message.Code != lqcWorkTicketStatusMsg || message.Size > lqcWorkTicketMaxMessageSize {
			errorsCh <- errors.New("invalid lqc work ticket handshake message")
			return
		}
		var remote lqcWorkTicketStatusPacket
		if err := message.Decode(&remote); err != nil {
			errorsCh <- err
			return
		}
		switch {
		case remote.ProtocolVersion != status.ProtocolVersion:
			errorsCh <- errors.New("lqc work ticket protocol version mismatch")
		case remote.NetworkID != status.NetworkID:
			errorsCh <- errors.New("lqc work ticket network ID mismatch")
		case remote.Genesis != status.Genesis:
			errorsCh <- errors.New("lqc work ticket genesis mismatch")
		case remote.ChainID == nil || remote.ChainID.Cmp(status.ChainID) != 0:
			errorsCh <- errors.New("lqc work ticket chain ID mismatch")
		default:
			errorsCh <- nil
		}
	}()
	timer := time.NewTimer(lqcWorkTicketHandshakeTimeout)
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

func (n *lqcWorkTicketTransport) register(peer *lqcWorkTicketPeer) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.closed {
		return errLQCWorkTicketProtocolClosed
	}
	if _, exists := n.peers[peer.id()]; exists {
		return errLQCWorkTicketPeerKnown
	}
	n.peers[peer.id()] = peer
	return nil
}

func (n *lqcWorkTicketTransport) unregister(id string) {
	n.mu.Lock()
	delete(n.peers, id)
	n.mu.Unlock()
}

func (n *lqcWorkTicketTransport) Submit(ticket lqc.WorkTicket) (common.Hash, error) {
	if n == nil {
		return common.Hash{}, errLQCWorkTicketTransportDisabled
	}
	n.mu.RLock()
	closed := n.closed
	n.mu.RUnlock()
	if closed {
		return common.Hash{}, errLQCWorkTicketProtocolClosed
	}
	hash := lqc.WorkTicketHash(n.chainID, ticket)
	if n.pool.Has(hash) {
		return hash, lqc.ErrWorkTicketKnown
	}
	if !n.consumeVerificationBudget(1, time.Now()) {
		return common.Hash{}, errLQCWorkTicketRateLimited
	}
	hash, err := n.pool.Add(n.chainID, ticket)
	if err != nil {
		return hash, err
	}
	n.BroadcastTickets([]lqc.WorkTicket{ticket}, "")
	return hash, nil
}

func (n *lqcWorkTicketTransport) sendPending(peer *lqcWorkTicketPeer) {
	tickets := n.pool.All(lqcWorkTicketInitialSyncLimit)
	if err := peer.sendTicketBatches(n.chainID, tickets); err != nil {
		peer.peer.Log().Debug("LQC work ticket initial pool sync failed", "err", err)
	}
}

func (n *lqcWorkTicketTransport) BroadcastTickets(tickets []lqc.WorkTicket, except string) {
	if n == nil || len(tickets) == 0 {
		return
	}
	n.mu.RLock()
	peers := make([]*lqcWorkTicketPeer, 0, len(n.peers))
	for id, peer := range n.peers {
		if id != except {
			peers = append(peers, peer)
		}
	}
	n.mu.RUnlock()
	for _, peer := range peers {
		peer := peer
		go func() {
			if err := peer.sendTicketBatches(n.chainID, tickets); err != nil {
				peer.peer.Log().Debug("LQC work ticket broadcast failed", "err", err)
			}
		}()
	}
}

func (n *lqcWorkTicketTransport) Close() {
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

func (p *lqcWorkTicketPeer) id() string { return p.peer.ID().String() }

func (p *lqcWorkTicketPeer) markKnown(hash common.Hash) {
	p.mu.Lock()
	if len(p.known) >= lqcWorkTicketMaxKnownPerPeer {
		clear(p.known)
	}
	p.known[hash] = struct{}{}
	p.mu.Unlock()
}

func (p *lqcWorkTicketPeer) consumeVerificationBudget(count int, now time.Time) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if count <= 0 || count > MaxWorkTicketsPerPacket {
		return false
	}
	if p.budgetWindowFrom.IsZero() || now.Sub(p.budgetWindowFrom) >= lqcWorkTicketBudgetWindow {
		p.budgetWindowFrom = now
		p.budgetUsed = 0
	}
	if p.budgetUsed+count > lqcWorkTicketBudgetTickets {
		return false
	}
	p.budgetUsed += count
	return true
}

func (p *lqcWorkTicketPeer) sendTicketBatches(chainID *big.Int, tickets []lqc.WorkTicket) error {
	for len(tickets) > 0 {
		limit := min(len(tickets), MaxWorkTicketsPerPacket)
		if err := p.sendTickets(chainID, tickets[:limit]); err != nil {
			return err
		}
		tickets = tickets[limit:]
	}
	return nil
}

func (p *lqcWorkTicketPeer) sendTickets(chainID *big.Int, tickets []lqc.WorkTicket) error {
	if len(tickets) == 0 || len(tickets) > MaxWorkTicketsPerPacket {
		return fmt.Errorf("invalid outbound lqc work ticket packet size: %d", len(tickets))
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	unknown := make([]lqc.WorkTicket, 0, len(tickets))
	for _, ticket := range tickets {
		hash := lqc.WorkTicketHash(chainID, ticket)
		if _, exists := p.known[hash]; !exists {
			unknown = append(unknown, ticket)
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	if err := p2p.Send(p.rw, lqcWorkTicketTicketsMsg, unknown); err != nil {
		return err
	}
	for _, ticket := range unknown {
		if len(p.known) >= lqcWorkTicketMaxKnownPerPeer {
			clear(p.known)
		}
		p.known[lqc.WorkTicketHash(chainID, ticket)] = struct{}{}
	}
	return nil
}
