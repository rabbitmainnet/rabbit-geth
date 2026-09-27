//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/crypto/rabbitvrf"
	"github.com/ethereum/go-ethereum/p2p"
)

const (
	rabbitVRFDKGProtocolName    = "rvrfdkg"
	rabbitVRFDKGProtocolVersion = uint(1)
	rabbitVRFDKGProtocolLength  = uint64(5)

	rabbitVRFDKGStatusMsg               = uint64(0)
	rabbitVRFDKGTransportArtifactMsg    = uint64(1)
	rabbitVRFDKGPolynomialCommitmentMsg = uint64(2)
	rabbitVRFDKGPeerRouteMsg            = uint64(3)
	rabbitVRFDKGEncryptedEvaluationMsg  = uint64(4)

	rabbitVRFDKGHandshakeTimeout = 5 * time.Second
	rabbitVRFDKGMaxMessageSize   = 16 * 1024
	rabbitVRFDKGMaxKnownPerPeer  = 4096
)

var (
	errRabbitVRFDKGProtocolClosed          = errors.New("rabbit vrf dkg protocol closed")
	errRabbitVRFDKGPeerKnown               = errors.New("rabbit vrf dkg peer already connected")
	errRabbitVRFDKGArtifactConflict        = errors.New("rabbit vrf dkg transport artifact conflict")
	errRabbitVRFDKGArtifactSessionMismatch = errors.New("rabbit vrf dkg transport artifact session mismatch")
)

type rabbitVRFDKGStatusPacket struct {
	ProtocolVersion     uint32
	SessionVersion      uint8
	EnvelopeVersion     uint8
	TransportKeyVersion uint8
	CommitteeVersion    uint8
	NetworkID           uint64
	Genesis             common.Hash
	ChainID             *big.Int
}

type rabbitVRFDKGTransportArtifactPacketV1 struct {
	Binding  lqc.RabbitVRFDKGTransportKeyBindingV1
	Envelope lqc.RabbitVRFDKGEnvelopeV1
}

type rabbitVRFDKGPolynomialCommitmentPacketV1 struct {
	Commitment lqc.RabbitVRFDKGPolynomialCommitmentV1
	Envelope   lqc.RabbitVRFDKGEnvelopeV1
}

type rabbitVRFDKGTransportConfig struct {
	ChainID   *big.Int
	NetworkID uint64
	Genesis   common.Hash
	Runtime   *rabbitVRFDKGRuntime
}

type rabbitVRFDKGTransport struct {
	chainID   *big.Int
	networkID uint64
	genesis   common.Hash
	runtime   *rabbitVRFDKGRuntime

	mu     sync.RWMutex
	peers  map[string]*rabbitVRFDKGPeer
	closed bool

	remoteSession     common.Hash
	remoteArtifacts   map[uint64]rabbitVRFDKGTransportArtifactPacketV1
	remoteCommitments map[uint64]rabbitVRFDKGPolynomialCommitmentPacketV1

	routeSession common.Hash
	routes       map[uint64]string
}

type rabbitVRFDKGPeer struct {
	peer *p2p.Peer
	rw   p2p.MsgReadWriter

	mu    sync.Mutex
	known map[common.Hash]struct{}
}

func newRabbitVRFDKGTransport(
	config rabbitVRFDKGTransportConfig,
) (*rabbitVRFDKGTransport, error) {
	if config.ChainID == nil || config.ChainID.Sign() <= 0 {
		return nil, errors.New("invalid rabbit vrf dkg chain ID")
	}
	if config.NetworkID == 0 {
		return nil, errors.New("zero rabbit vrf dkg network ID")
	}
	if config.Genesis == (common.Hash{}) {
		return nil, errors.New("zero rabbit vrf dkg genesis")
	}

	return &rabbitVRFDKGTransport{
		chainID:           new(big.Int).Set(config.ChainID),
		networkID:         config.NetworkID,
		genesis:           config.Genesis,
		runtime:           config.Runtime,
		peers:             make(map[string]*rabbitVRFDKGPeer),
		remoteArtifacts:   make(map[uint64]rabbitVRFDKGTransportArtifactPacketV1),
		remoteCommitments: make(map[uint64]rabbitVRFDKGPolynomialCommitmentPacketV1),
		routes:            make(map[uint64]string),
	}, nil
}

func newRabbitVRFDKGTransportMaybeLab(
	backend *Ethereum,
	runtime *rabbitVRFDKGRuntime,
	networkID uint64,
) (*rabbitVRFDKGTransport, error) {
	if runtime == nil {
		return nil, nil
	}
	if backend == nil || backend.blockchain == nil {
		return nil, errors.New("invalid rabbit vrf dkg transport backend")
	}

	chainConfig := backend.blockchain.Config()
	genesis := backend.blockchain.Genesis()
	if chainConfig == nil ||
		chainConfig.ChainID == nil ||
		genesis == nil {
		return nil, errors.New("invalid rabbit vrf dkg chain context")
	}

	return newRabbitVRFDKGTransport(
		rabbitVRFDKGTransportConfig{
			ChainID:   chainConfig.ChainID,
			NetworkID: networkID,
			Genesis:   genesis.Hash(),
			Runtime:   runtime,
		},
	)
}
func (n *rabbitVRFDKGTransport) Protocol() p2p.Protocol {
	return p2p.Protocol{
		Name:    rabbitVRFDKGProtocolName,
		Version: rabbitVRFDKGProtocolVersion,
		Length:  rabbitVRFDKGProtocolLength,
		Run: func(remote *p2p.Peer, rw p2p.MsgReadWriter) error {
			return n.runPeer(remote, rw)
		},
		NodeInfo: func() interface{} { return n.status() },
	}
}

func (n *rabbitVRFDKGTransport) status() rabbitVRFDKGStatusPacket {
	return rabbitVRFDKGStatusPacket{
		ProtocolVersion:     uint32(rabbitVRFDKGProtocolVersion),
		SessionVersion:      lqc.RabbitVRFDKGSessionVersionV1,
		EnvelopeVersion:     lqc.RabbitVRFDKGEnvelopeVersionV1,
		TransportKeyVersion: lqc.RabbitVRFDKGTransportKeyVersionV1,
		CommitteeVersion:    lqc.RabbitVRFCommitteeVersionV1,
		NetworkID:           n.networkID,
		Genesis:             n.genesis,
		ChainID:             new(big.Int).Set(n.chainID),
	}
}

func cloneRabbitVRFDKGTransportArtifactPacketV1(
	packet rabbitVRFDKGTransportArtifactPacketV1,
) rabbitVRFDKGTransportArtifactPacketV1 {
	packet.Envelope.Signature = append(
		[]byte(nil),
		packet.Envelope.Signature...,
	)
	return packet
}

func (n *rabbitVRFDKGTransport) storeRemoteTransportArtifactV1(
	packet rabbitVRFDKGTransportArtifactPacketV1,
) (bool, error) {
	if err := n.validateTransportArtifactV1(packet); err != nil {
		return false, err
	}

	n.runtime.mu.RLock()
	defer n.runtime.mu.RUnlock()

	if n.runtime.current.SessionID == (common.Hash{}) ||
		n.runtime.current.SessionID != packet.Binding.SessionID {
		return false, errors.New("rabbit vrf dkg transport artifact session changed")
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	if n.closed {
		return false, errRabbitVRFDKGProtocolClosed
	}

	if n.remoteSession != packet.Binding.SessionID {
		n.remoteSession = packet.Binding.SessionID
		n.remoteArtifacts = make(
			map[uint64]rabbitVRFDKGTransportArtifactPacketV1,
		)
		n.remoteCommitments = make(
			map[uint64]rabbitVRFDKGPolynomialCommitmentPacketV1,
		)
	}

	if existing, ok := n.remoteArtifacts[packet.Binding.ShareID]; ok {
		if existing.Binding != packet.Binding {
			return false, errRabbitVRFDKGArtifactConflict
		}
		return false, nil
	}

	n.remoteArtifacts[packet.Binding.ShareID] =
		cloneRabbitVRFDKGTransportArtifactPacketV1(packet)

	return true, nil
}

func (n *rabbitVRFDKGTransport) remoteTransportArtifactV1(
	sessionID common.Hash,
	shareID uint64,
) (rabbitVRFDKGTransportArtifactPacketV1, bool) {
	if n == nil ||
		n.runtime == nil ||
		sessionID == (common.Hash{}) ||
		shareID == 0 {
		return rabbitVRFDKGTransportArtifactPacketV1{}, false
	}

	n.runtime.mu.RLock()
	defer n.runtime.mu.RUnlock()

	if n.runtime.current.SessionID == (common.Hash{}) ||
		n.runtime.current.SessionID != sessionID {
		return rabbitVRFDKGTransportArtifactPacketV1{}, false
	}

	n.mu.RLock()
	defer n.mu.RUnlock()

	if n.closed || n.remoteSession != sessionID {
		return rabbitVRFDKGTransportArtifactPacketV1{}, false
	}

	packet, ok := n.remoteArtifacts[shareID]
	if !ok {
		return rabbitVRFDKGTransportArtifactPacketV1{}, false
	}

	return cloneRabbitVRFDKGTransportArtifactPacketV1(packet), true
}

func (n *rabbitVRFDKGTransport) validateTransportArtifactV1(
	packet rabbitVRFDKGTransportArtifactPacketV1,
) error {
	if n == nil || n.runtime == nil {
		return errors.New("rabbit vrf dkg transport runtime unavailable")
	}

	context := n.runtime.currentContext()
	if context.SessionID == (common.Hash{}) ||
		packet.Binding.SessionID != context.SessionID ||
		packet.Envelope.SessionID != context.SessionID {
		return errRabbitVRFDKGArtifactSessionMismatch
	}

	var expected lqc.RabbitVRFCommitteeMemberV1
	found := false

	for _, member := range context.CanonicalMembers {
		if member.ShareID == packet.Binding.ShareID &&
			member.Participant == packet.Binding.Participant {
			expected = member
			found = true
			break
		}
	}

	if !found {
		return errors.New("rabbit vrf dkg transport artifact sender not in canonical committee")
	}

	return lqc.VerifyRabbitVRFDKGTransportKeyEnvelopeV1(
		context.CanonicalSession,
		expected,
		packet.Binding,
		packet.Envelope,
	)
}

func cloneRabbitVRFDKGPolynomialCommitmentsV1(
	input []lqc.RabbitVRFDKGPolynomialCommitmentV1,
) []lqc.RabbitVRFDKGPolynomialCommitmentV1 {
	if len(input) == 0 {
		return nil
	}
	out := make([]lqc.RabbitVRFDKGPolynomialCommitmentV1, len(input))
	for i := range input {
		out[i] = input[i]
		out[i].Coefficients = append(
			[]rabbitvrf.DKGCoefficientCommitmentV1(nil),
			input[i].Coefficients...,
		)
	}
	return out
}

func cloneRabbitVRFDKGPolynomialCommitmentPacketV1(
	packet rabbitVRFDKGPolynomialCommitmentPacketV1,
) rabbitVRFDKGPolynomialCommitmentPacketV1 {
	packet.Commitment.Coefficients = append(
		[]rabbitvrf.DKGCoefficientCommitmentV1(nil),
		packet.Commitment.Coefficients...,
	)
	packet.Envelope.Signature = append([]byte(nil), packet.Envelope.Signature...)
	return packet
}

func (n *rabbitVRFDKGTransport) validatePolynomialCommitmentV1(
	packet rabbitVRFDKGPolynomialCommitmentPacketV1,
) error {
	if n == nil || n.runtime == nil {
		return errors.New("rabbit vrf dkg transport runtime unavailable")
	}
	context := n.runtime.currentContext()
	if context.SessionID == (common.Hash{}) ||
		packet.Commitment.SessionID != context.SessionID ||
		packet.Envelope.SessionID != context.SessionID {
		return errRabbitVRFDKGArtifactSessionMismatch
	}
	if packet.Commitment.DealerShareID == 0 ||
		packet.Commitment.DealerShareID != packet.Envelope.SenderShareID {
		return errors.New("rabbit vrf dkg polynomial commitment dealer mismatch")
	}
	var expected lqc.RabbitVRFCommitteeMemberV1
	found := false
	for _, member := range context.CanonicalMembers {
		if member.ShareID == packet.Commitment.DealerShareID &&
			member.Participant == packet.Envelope.Participant {
			expected = member
			found = true
			break
		}
	}
	if !found {
		return errors.New("rabbit vrf dkg polynomial commitment sender not in canonical committee")
	}
	root, err := lqc.RabbitVRFDKGPolynomialCommitmentPayloadHashV1(
		context.CanonicalSession,
		packet.Commitment,
	)
	if err != nil {
		return err
	}
	if packet.Envelope.MessageType != lqc.RabbitVRFDKGMessagePolynomialCommitmentV1 ||
		packet.Envelope.PayloadHash != root {
		return errors.New("rabbit vrf dkg polynomial commitment payload mismatch")
	}
	return lqc.VerifyRabbitVRFDKGEnvelopeV1(
		context.CanonicalSession,
		expected,
		packet.Envelope,
	)
}

func (n *rabbitVRFDKGTransport) storeRemotePolynomialCommitmentV1(
	packet rabbitVRFDKGPolynomialCommitmentPacketV1,
) (bool, error) {
	if err := n.validatePolynomialCommitmentV1(packet); err != nil {
		return false, err
	}
	n.runtime.mu.RLock()
	defer n.runtime.mu.RUnlock()
	if n.runtime.current.SessionID == (common.Hash{}) ||
		n.runtime.current.SessionID != packet.Commitment.SessionID {
		return false, errors.New("rabbit vrf dkg polynomial commitment session changed")
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.closed {
		return false, errRabbitVRFDKGProtocolClosed
	}
	if n.remoteSession != packet.Commitment.SessionID {
		n.remoteSession = packet.Commitment.SessionID
		n.remoteArtifacts = make(map[uint64]rabbitVRFDKGTransportArtifactPacketV1)
		n.remoteCommitments = make(map[uint64]rabbitVRFDKGPolynomialCommitmentPacketV1)
	}
	if existing, ok := n.remoteCommitments[packet.Commitment.DealerShareID]; ok {
		existingRoot, err := lqc.RabbitVRFDKGPolynomialCommitmentPayloadHashV1(
			n.runtime.current.CanonicalSession,
			existing.Commitment,
		)
		if err != nil {
			return false, err
		}
		newRoot, err := lqc.RabbitVRFDKGPolynomialCommitmentPayloadHashV1(
			n.runtime.current.CanonicalSession,
			packet.Commitment,
		)
		if err != nil {
			return false, err
		}
		if existingRoot != newRoot || existing.Envelope.PayloadHash != packet.Envelope.PayloadHash {
			return false, errRabbitVRFDKGArtifactConflict
		}
		return false, nil
	}
	n.remoteCommitments[packet.Commitment.DealerShareID] =
		cloneRabbitVRFDKGPolynomialCommitmentPacketV1(packet)
	return true, nil
}

func (n *rabbitVRFDKGTransport) runPeer(
	remote *p2p.Peer,
	rw p2p.MsgReadWriter,
) error {
	peer := &rabbitVRFDKGPeer{
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

	go func() {
		if err := n.sendLocalPeerRouteProofsV1(peer); err != nil {
			if peer.peer != nil {
				peer.peer.Log().Debug("Rabbit VRF DKG peer route proof sync failed", "err", err)
			}
			return
		}
		if err := n.sendPendingTransportArtifactsV1(peer); err != nil {
			if peer.peer != nil {
				peer.peer.Log().Debug(
					"Rabbit VRF DKG initial artifact sync failed",
					"err",
					err,
				)
			}
		}
	}()

	for {
		message, err := rw.ReadMsg()
		if err != nil {
			return err
		}
		if message.Size > rabbitVRFDKGMaxMessageSize {
			return fmt.Errorf(
				"rabbit vrf dkg message too large: %d",
				message.Size,
			)
		}
		switch message.Code {
		case rabbitVRFDKGTransportArtifactMsg:
			var packet rabbitVRFDKGTransportArtifactPacketV1
			if err := message.Decode(&packet); err != nil {
				return fmt.Errorf(
					"decode rabbit vrf dkg transport artifact: %w",
					err,
				)
			}
			inserted, err := n.storeRemoteTransportArtifactV1(packet)
			if err != nil {
				if errors.Is(
					err,
					errRabbitVRFDKGArtifactSessionMismatch,
				) {
					continue
				}

				return fmt.Errorf(
					"validate rabbit vrf dkg transport artifact: %w",
					err,
				)
			}

			peer.markKnown(packet.Envelope.PayloadHash)

			if inserted {
				if _, err := n.persistCanonicalTransportKeySetV1(); err != nil {
					return fmt.Errorf("persist rabbit vrf dkg canonical transport key set: %w", err)
				}

				n.broadcastTransportArtifactV1(
					packet,
					peer.id(),
				)
			}
		case rabbitVRFDKGPolynomialCommitmentMsg:
			var packet rabbitVRFDKGPolynomialCommitmentPacketV1
			if err := message.Decode(&packet); err != nil {
				return fmt.Errorf(
					"decode rabbit vrf dkg polynomial commitment: %w",
					err,
				)
			}

			inserted, err := n.storeRemotePolynomialCommitmentV1(packet)
			if err != nil {
				if errors.Is(
					err,
					errRabbitVRFDKGArtifactSessionMismatch,
				) {
					continue
				}
				return fmt.Errorf(
					"validate rabbit vrf dkg polynomial commitment: %w",
					err,
				)
			}

			peer.markKnown(packet.Envelope.PayloadHash)

			if inserted {
				n.broadcastPolynomialCommitmentV1(
					packet,
					peer.id(),
				)
			}

		case rabbitVRFDKGPeerRouteMsg:
			var packet rabbitVRFDKGPeerRouteProofPacketV1
			if err := message.Decode(&packet); err != nil {
				return fmt.Errorf("decode rabbit vrf dkg peer route proof: %w", err)
			}
			if err := n.storePeerRouteProofV1(peer, packet); err != nil {
				if errors.Is(err, errRabbitVRFDKGArtifactSessionMismatch) {
					continue
				}
				return fmt.Errorf("validate rabbit vrf dkg peer route proof: %w", err)
			}

		case rabbitVRFDKGEncryptedEvaluationMsg:
			var packet lqc.RabbitVRFDKGEncryptedEvaluationV1
			if err := message.Decode(&packet); err != nil {
				return fmt.Errorf("decode rabbit vrf dkg encrypted evaluation: %w", err)
			}
			if err := n.runtime.validateInboundEncryptedEvaluationV1(packet); err != nil {
				if errors.Is(err, errRabbitVRFDKGArtifactSessionMismatch) {
					continue
				}
				return fmt.Errorf("validate rabbit vrf dkg encrypted evaluation: %w", err)
			}

		default:
			return fmt.Errorf(
				"invalid rabbit vrf dkg message code: %d",
				message.Code,
			)
		}
	}
}

func (n *rabbitVRFDKGTransport) handshake(
	peer *rabbitVRFDKGPeer,
) error {
	status := n.status()
	errorsCh := make(chan error, 2)

	go func() {
		errorsCh <- p2p.Send(
			peer.rw,
			rabbitVRFDKGStatusMsg,
			status,
		)
	}()

	go func() {
		message, err := peer.rw.ReadMsg()
		if err != nil {
			errorsCh <- err
			return
		}
		if message.Code != rabbitVRFDKGStatusMsg ||
			message.Size > rabbitVRFDKGMaxMessageSize {
			errorsCh <- errors.New("invalid rabbit vrf dkg handshake message")
			return
		}

		var remote rabbitVRFDKGStatusPacket
		if err := message.Decode(&remote); err != nil {
			errorsCh <- err
			return
		}

		switch {
		case remote.ProtocolVersion != status.ProtocolVersion:
			errorsCh <- errors.New("rabbit vrf dkg protocol version mismatch")
		case remote.SessionVersion != status.SessionVersion:
			errorsCh <- errors.New("rabbit vrf dkg session version mismatch")
		case remote.EnvelopeVersion != status.EnvelopeVersion:
			errorsCh <- errors.New("rabbit vrf dkg envelope version mismatch")
		case remote.TransportKeyVersion != status.TransportKeyVersion:
			errorsCh <- errors.New("rabbit vrf dkg transport key version mismatch")
		case remote.CommitteeVersion != status.CommitteeVersion:
			errorsCh <- errors.New("rabbit vrf committee version mismatch")
		case remote.NetworkID != status.NetworkID:
			errorsCh <- errors.New("rabbit vrf dkg network ID mismatch")
		case remote.Genesis != status.Genesis:
			errorsCh <- errors.New("rabbit vrf dkg genesis mismatch")
		case remote.ChainID == nil ||
			remote.ChainID.Cmp(status.ChainID) != 0:
			errorsCh <- errors.New("rabbit vrf dkg chain ID mismatch")
		default:
			errorsCh <- nil
		}
	}()

	timer := time.NewTimer(rabbitVRFDKGHandshakeTimeout)
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

func (peer *rabbitVRFDKGPeer) markKnown(
	hash common.Hash,
) {
	if peer == nil || hash == (common.Hash{}) {
		return
	}

	peer.mu.Lock()
	defer peer.mu.Unlock()

	if peer.known == nil {
		peer.known = make(map[common.Hash]struct{})
	}
	if len(peer.known) >= rabbitVRFDKGMaxKnownPerPeer {
		clear(peer.known)
	}
	peer.known[hash] = struct{}{}
}

func (peer *rabbitVRFDKGPeer) sendTransportArtifactV1(
	packet rabbitVRFDKGTransportArtifactPacketV1,
) error {
	if peer == nil || peer.rw == nil {
		return errors.New("invalid rabbit vrf dkg peer transport")
	}

	hash := packet.Envelope.PayloadHash
	if hash == (common.Hash{}) {
		return errors.New("zero rabbit vrf dkg transport artifact hash")
	}

	peer.mu.Lock()
	defer peer.mu.Unlock()

	if peer.known == nil {
		peer.known = make(map[common.Hash]struct{})
	}
	if _, exists := peer.known[hash]; exists {
		return nil
	}

	if err := p2p.Send(
		peer.rw,
		rabbitVRFDKGTransportArtifactMsg,
		packet,
	); err != nil {
		return err
	}

	if len(peer.known) >= rabbitVRFDKGMaxKnownPerPeer {
		clear(peer.known)
	}
	peer.known[hash] = struct{}{}

	return nil
}

func (peer *rabbitVRFDKGPeer) sendPolynomialCommitmentV1(
	packet rabbitVRFDKGPolynomialCommitmentPacketV1,
) error {
	if peer == nil || peer.rw == nil {
		return errors.New("invalid rabbit vrf dkg peer transport")
	}

	hash := packet.Envelope.PayloadHash
	if hash == (common.Hash{}) {
		return errors.New("zero rabbit vrf dkg polynomial commitment hash")
	}

	peer.mu.Lock()
	defer peer.mu.Unlock()

	if peer.known == nil {
		peer.known = make(map[common.Hash]struct{})
	}
	if _, exists := peer.known[hash]; exists {
		return nil
	}

	if err := p2p.Send(
		peer.rw,
		rabbitVRFDKGPolynomialCommitmentMsg,
		packet,
	); err != nil {
		return err
	}

	if len(peer.known) >= rabbitVRFDKGMaxKnownPerPeer {
		clear(peer.known)
	}
	peer.known[hash] = struct{}{}

	return nil
}

func (peer *rabbitVRFDKGPeer) id() string {
	if peer == nil || peer.peer == nil {
		return ""
	}
	return peer.peer.ID().String()
}

func (n *rabbitVRFDKGTransport) register(
	peer *rabbitVRFDKGPeer,
) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.closed {
		return errRabbitVRFDKGProtocolClosed
	}
	id := peer.id()
	if id == "" {
		return errors.New("invalid rabbit vrf dkg peer")
	}
	if _, exists := n.peers[id]; exists {
		return errRabbitVRFDKGPeerKnown
	}
	n.peers[id] = peer
	return nil
}

func (n *rabbitVRFDKGTransport) unregister(id string) {
	n.mu.Lock()
	delete(n.peers, id)
	for shareID, peerID := range n.routes {
		if peerID == id {
			delete(n.routes, shareID)
		}
	}
	n.mu.Unlock()
}

func (n *rabbitVRFDKGTransport) sendLocalTransportArtifactsV1(
	peer *rabbitVRFDKGPeer,
) error {
	if n == nil || n.runtime == nil || peer == nil {
		return nil
	}

	context := n.runtime.currentContext()
	if context.SessionID == (common.Hash{}) ||
		len(context.Members) == 0 ||
		len(context.TransportBindings) == 0 ||
		len(context.TransportBindings) !=
			len(context.TransportEnvelopes) {
		return nil
	}

	if !n.runtime.transportArtifactsReadyV1(
		context.CanonicalSession,
		context.SessionID,
		context.Members,
	) {
		return nil
	}

	for index := range context.TransportBindings {
		current := n.runtime.currentContext()
		if current.SessionID != context.SessionID {
			return nil
		}

		packet := rabbitVRFDKGTransportArtifactPacketV1{
			Binding:  context.TransportBindings[index],
			Envelope: context.TransportEnvelopes[index],
		}

		if err := peer.sendTransportArtifactV1(
			cloneRabbitVRFDKGTransportArtifactPacketV1(packet),
		); err != nil {
			return err
		}
	}

	return nil
}

func (n *rabbitVRFDKGTransport) broadcastTransportArtifactV1(
	packet rabbitVRFDKGTransportArtifactPacketV1,
	except string,
) {
	if n == nil {
		return
	}

	n.mu.RLock()
	if n.closed {
		n.mu.RUnlock()
		return
	}

	peers := make([]*rabbitVRFDKGPeer, 0, len(n.peers))
	for id, peer := range n.peers {
		if id != except {
			peers = append(peers, peer)
		}
	}
	n.mu.RUnlock()

	for _, peer := range peers {
		peer := peer
		outbound := cloneRabbitVRFDKGTransportArtifactPacketV1(packet)

		go func() {
			if err := peer.sendTransportArtifactV1(outbound); err != nil {
				if peer.peer != nil {
					peer.peer.Log().Debug(
						"Rabbit VRF DKG artifact broadcast failed",
						"err",
						err,
					)
				}
			}
		}()
	}
}

func (n *rabbitVRFDKGTransport) sendLocalPolynomialCommitmentsV1(peer *rabbitVRFDKGPeer) error {
	if n == nil || n.runtime == nil || peer == nil {
		return nil
	}
	context := n.runtime.currentContext()
	if context.SessionID == (common.Hash{}) {
		return nil
	}
	if len(context.PolynomialCommitments) == 0 {
		return nil
	}
	if len(context.PolynomialCommitments) != len(context.PolynomialEnvelopes) {
		return errors.New("rabbit vrf dkg local polynomial commitment/envelope count mismatch")
	}
	for i := range context.PolynomialCommitments {
		if n.runtime.currentContext().SessionID != context.SessionID {
			return nil
		}
		packet := rabbitVRFDKGPolynomialCommitmentPacketV1{
			Commitment: context.PolynomialCommitments[i],
			Envelope:   context.PolynomialEnvelopes[i],
		}
		if err := n.validatePolynomialCommitmentV1(packet); err != nil {
			return fmt.Errorf("validate local rabbit vrf dkg polynomial commitment: %w", err)
		}
		if err := peer.sendPolynomialCommitmentV1(cloneRabbitVRFDKGPolynomialCommitmentPacketV1(packet)); err != nil {
			return err
		}
	}
	return nil
}

func (n *rabbitVRFDKGTransport) broadcastPolynomialCommitmentV1(
	packet rabbitVRFDKGPolynomialCommitmentPacketV1,
	except string,
) {
	if n == nil {
		return
	}

	n.mu.RLock()
	if n.closed {
		n.mu.RUnlock()
		return
	}

	peers := make([]*rabbitVRFDKGPeer, 0, len(n.peers))
	for id, peer := range n.peers {
		if id != except {
			peers = append(peers, peer)
		}
	}
	n.mu.RUnlock()

	for _, peer := range peers {
		peer := peer
		outbound := cloneRabbitVRFDKGPolynomialCommitmentPacketV1(packet)

		go func() {
			if err := peer.sendPolynomialCommitmentV1(outbound); err != nil {
				if peer.peer != nil {
					peer.peer.Log().Debug(
						"Rabbit VRF DKG polynomial commitment broadcast failed",
						"err",
						err,
					)
				}
			}
		}()
	}

}

func (n *rabbitVRFDKGTransport) Close() {
	if n == nil {
		return
	}
	n.mu.Lock()
	n.closed = true
	n.peers = make(map[string]*rabbitVRFDKGPeer)
	n.remoteSession = common.Hash{}
	n.remoteArtifacts = make(
		map[uint64]rabbitVRFDKGTransportArtifactPacketV1,
	)
	n.remoteCommitments = make(
		map[uint64]rabbitVRFDKGPolynomialCommitmentPacketV1,
	)
	n.mu.Unlock()
}
