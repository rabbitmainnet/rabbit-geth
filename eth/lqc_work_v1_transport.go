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
	// New capability name. Deliberately NOT "lqct": old Argon2 peers must never
	// negotiate this RandomX/Work V1 payload by accident.
	lqcWorkV1ProtocolName    = "lqcw"
	lqcWorkV1ProtocolVersion = uint(1)
	lqcWorkV1ProtocolLength  = uint64(2)

	lqcWorkV1StatusMsg     = uint64(0)
	lqcWorkV1CandidatesMsg = uint64(1)

	lqcWorkV1HandshakeTimeout = 5 * time.Second
	lqcWorkV1MaxMessageSize   = 8 * 1024
	lqcWorkV1MaxKnownPerPeer  = 4096
	lqcWorkV1InitialSyncLimit = 64

	// One wire packet is exactly the Work V1 consensus per-block proof ceiling.
	MaxWorkV1CandidatesPerPacket = int(lqc.MaxWorkTicketsPerBlockV1)

	// The measured LIGHT verifier is the limiting path. These local relay
	// budgets are intentionally much tighter than the legacy Argon2 transport.
	lqcWorkV1BudgetWindow      = 10 * time.Second
	lqcWorkV1PeerBudget        = 8
	lqcWorkV1GlobalBudget      = 16
	lqcWorkV1MaxVerifyInFlight = uint64(1)
)

var (
	errLQCWorkV1TransportDisabled = errors.New("lqc work v1 transport disabled")
	errLQCWorkV1ProtocolClosed    = errors.New("lqc work v1 protocol closed")
	errLQCWorkV1PeerKnown         = errors.New("lqc work v1 peer already connected")
	errLQCWorkV1RateLimited       = errors.New("lqc work v1 verification budget exceeded")
	errLQCWorkV1Context           = errors.New("lqc work v1 context unavailable")
)

type lqcWorkV1StatusPacket struct {
	ProtocolVersion uint32
	WorkVersion     uint32
	NetworkID       uint64
	Genesis         common.Hash
	ChainID         *big.Int
}

type lqcWorkV1Context struct {
	Epoch           uint64
	DatasetAnchor   common.Hash
	ChallengeAnchor common.Hash
	Difficulty      *big.Int
	Eligibility     lqc.WorkRelayEligibilityCheckV1
}

type lqcWorkV1ContextProvider func() (lqcWorkV1Context, error)

type lqcWorkV1CanonicalReconciler func(commitEpoch uint64) error

type lqcWorkV1CanonicalIncludedCheck func(
	candidate lqc.WorkCommitCandidateV1,
	commitEpoch uint64,
) bool

type lqcWorkV1TransportConfig struct {
	Enabled   bool
	ChainID   *big.Int
	NetworkID uint64
	Genesis   common.Hash

	Context         lqcWorkV1ContextProvider
	Hasher          lqc.WorkRelayHasherV1
	PoolPersistence lqc.WorkCommitPoolPersistenceV1
}

type lqcWorkV1Transport struct {
	chainID   *big.Int
	networkID uint64
	genesis   common.Hash

	context      lqcWorkV1ContextProvider
	reconcile    lqcWorkV1CanonicalReconciler
	included     lqcWorkV1CanonicalIncludedCheck
	hasher       lqc.WorkRelayHasherV1
	pool         *lqc.WorkCommitPoolV1
	limiter      *lqc.WorkRelayVerificationLimiterV1
	fairVerifier *lqcWorkV1FairVerifier

	mu     sync.RWMutex
	peers  map[string]*lqcWorkV1Peer
	closed bool

	verificationMu         sync.Mutex
	globalBudgetWindowFrom time.Time
	globalBudgetUsed       int
}

type lqcWorkV1Peer struct {
	peer *p2p.Peer
	rw   p2p.MsgReadWriter

	mu               sync.Mutex
	known            map[common.Hash]struct{}
	budgetWindowFrom time.Time
	budgetUsed       int
}

func newLQCWorkV1Transport(
	config lqcWorkV1TransportConfig,
) (*lqcWorkV1Transport, error) {
	if !config.Enabled {
		return nil, errLQCWorkV1TransportDisabled
	}
	if config.ChainID == nil || config.ChainID.Sign() <= 0 {
		return nil, lqc.ErrInvalidWorkTicketChain
	}
	if config.NetworkID == 0 {
		return nil, errors.New("zero lqc work v1 network ID")
	}
	if config.Genesis == (common.Hash{}) {
		return nil, errors.New("zero lqc work v1 genesis")
	}
	if config.Context == nil || config.Hasher == nil {
		return nil, errLQCWorkV1Context
	}

	limiter, err := lqc.NewWorkRelayVerificationLimiterV1(
		lqcWorkV1MaxVerifyInFlight,
	)
	if err != nil {
		return nil, err
	}

	transport := &lqcWorkV1Transport{
		chainID:   new(big.Int).Set(config.ChainID),
		networkID: config.NetworkID,
		genesis:   config.Genesis,
		context:   config.Context,
		hasher:    config.Hasher,
		pool: lqc.NewPersistentWorkCommitPoolV1(
			0,
			config.PoolPersistence,
		),
		limiter: limiter,
		peers:   make(map[string]*lqcWorkV1Peer),
	}
	transport.fairVerifier = newLQCWorkV1FairVerifier()
	return transport, nil
}

func (n *lqcWorkV1Transport) Protocol() p2p.Protocol {
	return p2p.Protocol{
		Name:    lqcWorkV1ProtocolName,
		Version: lqcWorkV1ProtocolVersion,
		Length:  lqcWorkV1ProtocolLength,
		Run: func(remote *p2p.Peer, rw p2p.MsgReadWriter) error {
			return n.runPeer(remote, rw)
		},
		NodeInfo: func() interface{} { return n.status() },
	}
}

func (n *lqcWorkV1Transport) status() lqcWorkV1StatusPacket {
	return lqcWorkV1StatusPacket{
		ProtocolVersion: uint32(lqcWorkV1ProtocolVersion),
		WorkVersion:     uint32(lqc.RandomXWorkProtocolVersion),
		NetworkID:       n.networkID,
		Genesis:         n.genesis,
		ChainID:         new(big.Int).Set(n.chainID),
	}
}

func (n *lqcWorkV1Transport) currentContextRaw() (lqcWorkV1Context, error) {
	if n == nil || n.context == nil {
		return lqcWorkV1Context{}, errLQCWorkV1Context
	}

	ctx, err := n.context()
	if err != nil {
		return lqcWorkV1Context{}, err
	}
	if ctx.Epoch == 0 ||
		ctx.DatasetAnchor == (common.Hash{}) ||
		ctx.ChallengeAnchor == (common.Hash{}) ||
		ctx.Difficulty == nil ||
		ctx.Difficulty.Sign() <= 0 ||
		ctx.Eligibility == nil {
		return lqcWorkV1Context{}, errLQCWorkV1Context
	}

	ctx.Difficulty = new(big.Int).Set(ctx.Difficulty)
	if err := n.pool.ResetCommitEpochV1(ctx.Epoch); err != nil {
		return lqcWorkV1Context{}, err
	}
	return ctx, nil
}

func (n *lqcWorkV1Transport) currentContext() (lqcWorkV1Context, error) {
	ctx, err := n.currentContextRaw()
	if err != nil {
		return lqcWorkV1Context{}, err
	}
	if n.reconcile != nil {
		if err := n.reconcile(ctx.Epoch); err != nil {
			return lqcWorkV1Context{}, err
		}
	}
	return ctx, nil
}

func (n *lqcWorkV1Transport) canonicallyIncluded(
	candidate lqc.WorkCommitCandidateV1,
	commitEpoch uint64,
) bool {
	return n != nil && n.included != nil &&
		n.included(candidate, commitEpoch)
}

func (n *lqcWorkV1Transport) runPeer(
	remote *p2p.Peer,
	rw p2p.MsgReadWriter,
) error {
	peer := &lqcWorkV1Peer{
		peer:  remote,
		rw:    rw,
		known: make(map[common.Hash]struct{}),
	}
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
		if message.Size > lqcWorkV1MaxMessageSize {
			return fmt.Errorf(
				"lqc work v1 message too large: %d",
				message.Size,
			)
		}
		if message.Code != lqcWorkV1CandidatesMsg {
			return fmt.Errorf(
				"invalid lqc work v1 message code: %d",
				message.Code,
			)
		}

		var candidates []lqc.WorkCommitCandidateV1
		if err := message.Decode(&candidates); err != nil {
			return err
		}
		if len(candidates) == 0 ||
			len(candidates) > MaxWorkV1CandidatesPerPacket {
			return fmt.Errorf(
				"invalid lqc work v1 packet size: %d",
				len(candidates),
			)
		}

		ctx, err := n.currentContext()
		if err != nil {
			return err
		}

		accepted := make([]lqc.WorkCommitCandidateV1, 0, len(candidates))
		for _, candidate := range candidates {
			if candidate.ProofHash == (common.Hash{}) {
				return lqc.ErrInvalidWorkCommitV1
			}
			if n.canonicallyIncluded(candidate, ctx.Epoch) {
				peer.markKnown(candidate.ProofHash)
				continue
			}

			if n.pool.WorkCommitPoolContainsCandidateV1(candidate) {
				peer.markKnown(candidate.ProofHash)
				continue
			}

			prechecked, err := lqc.PrecheckRelayedWorkWithAnchorsV1(
				n.chainID,
				ctx.Epoch,
				ctx.DatasetAnchor,
				ctx.ChallengeAnchor,
				ctx.Difficulty,
				candidate,
				ctx.Eligibility,
			)
			if err != nil {
				return err
			}

			if !peer.consumeVerificationBudget(1, time.Now()) {
				return errLQCWorkV1RateLimited
			}

			err = n.fairVerifier.Run(peer.id(), func() error {
				if err := n.waitVerificationBudget(); err != nil {
					return err
				}
				if err := n.waitVerificationSlot(); err != nil {
					return err
				}
				defer n.limiter.ReleaseV1()

				verified, err := lqc.VerifyPrecheckedRelayedWorkWithAnchorsV1(
					prechecked,
					n.hasher,
				)
				if err != nil {
					return err
				}
				linked, err := lqc.NewVerifiedWorkCommitCandidateV1(
					prechecked.Candidate.Signed,
					verified,
				)
				if err != nil {
					return err
				}
				return n.pool.AddVerifiedV1(linked)
			})

			if err != nil {
				if errors.Is(err, lqc.ErrWorkRelayAlreadyKnownV1) ||
					errors.Is(err, lqc.ErrDuplicateRandomXWorkHash) ||
					errors.Is(err, lqc.ErrDuplicateWorkTicketV3) {
					peer.markKnown(candidate.ProofHash)
					continue
				}
				return err
			}

			peer.markKnown(candidate.ProofHash)
			accepted = append(accepted, candidate)
		}

		if len(accepted) > 0 {
			n.BroadcastCandidates(accepted, peer.id())
		}
	}
}

func (n *lqcWorkV1Transport) waitVerificationBudget() error {
	for {
		n.mu.RLock()
		closed := n.closed
		n.mu.RUnlock()
		if closed {
			return errLQCWorkV1ProtocolClosed
		}
		if n.consumeVerificationBudget(1, time.Now()) {
			return nil
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func (n *lqcWorkV1Transport) waitVerificationSlot() error {
	for {
		n.mu.RLock()
		closed := n.closed
		n.mu.RUnlock()
		if closed {
			return errLQCWorkV1ProtocolClosed
		}
		if n.limiter.TryAcquireV1() {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (n *lqcWorkV1Transport) consumeVerificationBudget(
	count int,
	now time.Time,
) bool {
	n.verificationMu.Lock()
	defer n.verificationMu.Unlock()

	if count <= 0 || count > MaxWorkV1CandidatesPerPacket {
		return false
	}
	if n.globalBudgetWindowFrom.IsZero() ||
		now.Sub(n.globalBudgetWindowFrom) >= lqcWorkV1BudgetWindow {
		n.globalBudgetWindowFrom = now
		n.globalBudgetUsed = 0
	}
	if n.globalBudgetUsed+count > lqcWorkV1GlobalBudget {
		return false
	}

	n.globalBudgetUsed += count
	return true
}

func (n *lqcWorkV1Transport) handshake(
	peer *lqcWorkV1Peer,
) error {
	status := n.status()
	errorsCh := make(chan error, 2)

	go func() {
		errorsCh <- p2p.Send(
			peer.rw,
			lqcWorkV1StatusMsg,
			status,
		)
	}()
	go func() {
		message, err := peer.rw.ReadMsg()
		if err != nil {
			errorsCh <- err
			return
		}
		if message.Code != lqcWorkV1StatusMsg ||
			message.Size > lqcWorkV1MaxMessageSize {
			errorsCh <- errors.New("invalid lqc work v1 handshake message")
			return
		}

		var remote lqcWorkV1StatusPacket
		if err := message.Decode(&remote); err != nil {
			errorsCh <- err
			return
		}

		switch {
		case remote.ProtocolVersion != status.ProtocolVersion:
			errorsCh <- errors.New("lqc work v1 protocol version mismatch")
		case remote.WorkVersion != status.WorkVersion:
			errorsCh <- errors.New("lqc work v1 algorithm version mismatch")
		case remote.NetworkID != status.NetworkID:
			errorsCh <- errors.New("lqc work v1 network ID mismatch")
		case remote.Genesis != status.Genesis:
			errorsCh <- errors.New("lqc work v1 genesis mismatch")
		case remote.ChainID == nil ||
			remote.ChainID.Cmp(status.ChainID) != 0:
			errorsCh <- errors.New("lqc work v1 chain ID mismatch")
		default:
			errorsCh <- nil
		}
	}()

	timer := time.NewTimer(lqcWorkV1HandshakeTimeout)
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

func (n *lqcWorkV1Transport) register(
	peer *lqcWorkV1Peer,
) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.closed {
		return errLQCWorkV1ProtocolClosed
	}
	if _, exists := n.peers[peer.id()]; exists {
		return errLQCWorkV1PeerKnown
	}
	n.peers[peer.id()] = peer
	return nil
}

func (n *lqcWorkV1Transport) unregister(id string) {
	n.mu.Lock()
	delete(n.peers, id)
	n.mu.Unlock()
}

func (n *lqcWorkV1Transport) Submit(
	candidate lqc.WorkCommitCandidateV1,
) (common.Hash, error) {
	if n == nil {
		return common.Hash{}, errLQCWorkV1TransportDisabled
	}

	n.mu.RLock()
	closed := n.closed
	n.mu.RUnlock()
	if closed {
		return common.Hash{}, errLQCWorkV1ProtocolClosed
	}

	ctx, err := n.currentContext()
	if err != nil {
		return common.Hash{}, err
	}
	if n.canonicallyIncluded(candidate, ctx.Epoch) {
		return candidate.ProofHash, lqc.ErrWorkRelayAlreadyKnownV1
	}

	if n.pool.WorkCommitPoolContainsCandidateV1(candidate) {
		return candidate.ProofHash, lqc.ErrWorkRelayAlreadyKnownV1
	}
	if !n.consumeVerificationBudget(1, time.Now()) {
		return common.Hash{}, errLQCWorkV1RateLimited
	}

	if err := lqc.ValidateAndAdmitRelayedWorkWithAnchorsV1(
		n.chainID,
		ctx.Epoch,
		ctx.DatasetAnchor,
		ctx.ChallengeAnchor,
		ctx.Difficulty,
		candidate,
		ctx.Eligibility,
		n.hasher,
		n.limiter,
		n.pool,
	); err != nil {
		return candidate.ProofHash, err
	}

	n.BroadcastCandidates(
		[]lqc.WorkCommitCandidateV1{candidate},
		"",
	)
	return candidate.ProofHash, nil
}

func (n *lqcWorkV1Transport) sendPending(
	peer *lqcWorkV1Peer,
) {
	if _, err := n.currentContext(); err != nil {
		return
	}

	candidates, err := n.pool.AllCanonicalV1()
	if err != nil {
		return
	}
	if len(candidates) > lqcWorkV1InitialSyncLimit {
		candidates = candidates[:lqcWorkV1InitialSyncLimit]
	}
	if err := peer.sendCandidateBatches(candidates); err != nil {
		peer.peer.Log().Debug(
			"LQC Work V1 initial pool sync failed",
			"err",
			err,
		)
	}
}

func (n *lqcWorkV1Transport) BroadcastCandidates(
	candidates []lqc.WorkCommitCandidateV1,
	except string,
) {
	if n == nil || len(candidates) == 0 {
		return
	}

	n.mu.RLock()
	peers := make([]*lqcWorkV1Peer, 0, len(n.peers))
	for id, peer := range n.peers {
		if id != except {
			peers = append(peers, peer)
		}
	}
	n.mu.RUnlock()

	for _, peer := range peers {
		peer := peer
		go func() {
			if err := peer.sendCandidateBatches(candidates); err != nil {
				peer.peer.Log().Debug(
					"LQC Work V1 broadcast failed",
					"err",
					err,
				)
			}
		}()
	}
}

func (n *lqcWorkV1Transport) PoolStatus() lqc.WorkCommitPoolStatusV1 {
	if n == nil || n.pool == nil {
		return lqc.WorkCommitPoolStatusV1{}
	}
	return n.pool.StatusV1()
}

func (n *lqcWorkV1Transport) Pending() (
	[]lqc.WorkCommitCandidateV1,
	error,
) {
	if n == nil || n.pool == nil {
		return nil, errLQCWorkV1TransportDisabled
	}
	if _, err := n.currentContext(); err != nil {
		return nil, err
	}
	return n.pool.PendingV1()
}

func (n *lqcWorkV1Transport) pendingRaw() (
	[]lqc.WorkCommitCandidateV1,
	error,
) {
	if n == nil || n.pool == nil {
		return nil, errLQCWorkV1TransportDisabled
	}
	return n.pool.PendingV1()
}

func (n *lqcWorkV1Transport) Close() {
	if n == nil {
		return
	}

	n.mu.Lock()
	n.closed = true
	for _, peer := range n.peers {
		peer.peer.Disconnect(p2p.DiscQuitting)
	}
	n.mu.Unlock()

	if n.fairVerifier != nil {
		n.fairVerifier.Close()
	}
}

func (p *lqcWorkV1Peer) id() string {
	return p.peer.ID().String()
}

func (p *lqcWorkV1Peer) markKnown(hash common.Hash) {
	p.mu.Lock()
	if len(p.known) >= lqcWorkV1MaxKnownPerPeer {
		clear(p.known)
	}
	p.known[hash] = struct{}{}
	p.mu.Unlock()
}

func (p *lqcWorkV1Peer) consumeVerificationBudget(
	count int,
	now time.Time,
) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	if count <= 0 || count > MaxWorkV1CandidatesPerPacket {
		return false
	}
	if p.budgetWindowFrom.IsZero() ||
		now.Sub(p.budgetWindowFrom) >= lqcWorkV1BudgetWindow {
		p.budgetWindowFrom = now
		p.budgetUsed = 0
	}
	if p.budgetUsed+count > lqcWorkV1PeerBudget {
		return false
	}

	p.budgetUsed += count
	return true
}

func (p *lqcWorkV1Peer) sendCandidateBatches(
	candidates []lqc.WorkCommitCandidateV1,
) error {
	for len(candidates) > 0 {
		limit := min(len(candidates), MaxWorkV1CandidatesPerPacket)
		if err := p.sendCandidates(candidates[:limit]); err != nil {
			return err
		}
		candidates = candidates[limit:]
	}
	return nil
}

func (p *lqcWorkV1Peer) sendCandidates(
	candidates []lqc.WorkCommitCandidateV1,
) error {
	if len(candidates) == 0 ||
		len(candidates) > MaxWorkV1CandidatesPerPacket {
		return fmt.Errorf(
			"invalid outbound lqc work v1 packet size: %d",
			len(candidates),
		)
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	unknown := make([]lqc.WorkCommitCandidateV1, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.ProofHash == (common.Hash{}) {
			return lqc.ErrInvalidWorkCommitV1
		}
		if _, exists := p.known[candidate.ProofHash]; !exists {
			unknown = append(unknown, candidate)
		}
	}
	if len(unknown) == 0 {
		return nil
	}

	if err := p2p.Send(
		p.rw,
		lqcWorkV1CandidatesMsg,
		unknown,
	); err != nil {
		return err
	}

	for _, candidate := range unknown {
		if len(p.known) >= lqcWorkV1MaxKnownPerPeer {
			clear(p.known)
		}
		p.known[candidate.ProofHash] = struct{}{}
	}
	return nil
}
