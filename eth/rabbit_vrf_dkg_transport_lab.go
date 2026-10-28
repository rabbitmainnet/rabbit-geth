//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/crypto/rabbitvrf"
	"github.com/ethereum/go-ethereum/internal/rabbitvrfstate"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p"
)

const (
	rabbitVRFDKGProtocolName      = "rvrfdkg"
	rabbitVRFDKGProtocolVersion   = uint(1)
	rabbitVRFDKGProtocolLength    = uint64(7)
	rabbitVRFDKGProtocolVersionV2 = uint(2)
	rabbitVRFDKGProtocolLengthV2  = uint64(8)

	rabbitVRFDKGStatusMsg               = uint64(0)
	rabbitVRFDKGTransportArtifactMsg    = uint64(1)
	rabbitVRFDKGPolynomialCommitmentMsg = uint64(2)
	rabbitVRFDKGPeerRouteMsg            = uint64(3)
	rabbitVRFDKGEncryptedEvaluationMsg  = uint64(4)
	rabbitVRFDKGThresholdPartialMsg     = uint64(5)
	rabbitVRFDKGKeysetCertificateMsg    = uint64(6)
	rabbitVRFDKGFinalizationMsg         = uint64(7)

	rabbitVRFDKGHandshakeTimeout = 5 * time.Second
	rabbitVRFDKGMaxMessageSize   = 16 * 1024
	rabbitVRFDKGMaxKnownPerPeer  = 4096
)

var (
	errRabbitVRFThresholdContextNotReadyV1 = errors.New("rabbit vrf threshold verification context not ready")
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

type rabbitVRFThresholdResultV1 struct {
	Signature  rabbitvrf.Signature
	Randomness common.Hash
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

	encryptedRelay rabbitVRFEncryptedRelayCacheV1

	remoteSession     common.Hash
	remoteArtifacts   map[uint64]rabbitVRFDKGTransportArtifactPacketV1
	remoteCommitments map[uint64]rabbitVRFDKGPolynomialCommitmentPacketV1

	routeSession common.Hash
	routes       map[uint64]string

	partialSession     common.Hash
	partialKeysetRoot  common.Hash
	partials           map[common.Hash]map[uint64]lqc.RabbitVRFThresholdPartialV1
	partialResults     map[common.Hash]rabbitVRFThresholdResultV1
	finalizations      map[rabbitVRFFinalizationKeyV1]lqc.RabbitVRFFinalizationV1
	finalizationWakeV2 chan struct{}

	keysetCertificateSession    common.Hash
	keysetCertificateRoot       common.Hash
	keysetCertificateSignatures map[uint64][]byte
	keysetCertificate           lqc.RabbitVRFKeysetCertificateV1
}

type rabbitVRFDKGPeer struct {
	peer    *p2p.Peer
	rw      p2p.MsgReadWriter
	version uint

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

	transport := &rabbitVRFDKGTransport{
		chainID:            new(big.Int).Set(config.ChainID),
		networkID:          config.NetworkID,
		genesis:            config.Genesis,
		runtime:            config.Runtime,
		peers:              make(map[string]*rabbitVRFDKGPeer),
		remoteArtifacts:    make(map[uint64]rabbitVRFDKGTransportArtifactPacketV1),
		remoteCommitments:  make(map[uint64]rabbitVRFDKGPolynomialCommitmentPacketV1),
		routes:             make(map[uint64]string),
		finalizations:      make(map[rabbitVRFFinalizationKeyV1]lqc.RabbitVRFFinalizationV1),
		finalizationWakeV2: make(chan struct{}),
	}
	if config.Runtime != nil && config.Runtime.engine != nil {
		if err := lqc.SetWorkV1EngineLabRabbitVRFFinalizationProvider(
			config.Runtime.engine,
			transport.rabbitVRFFinalizationsForBlockV1,
		); err != nil {
			return nil, fmt.Errorf("register rabbit vrf finalization provider: %w", err)
		}
		if err := lqc.SetWorkV1EngineLabRabbitVRFKeysetCertificateProvider(
			config.Runtime.engine,
			transport.rabbitVRFKeysetCertificateForBlockV1,
		); err != nil {
			return nil, fmt.Errorf("register rabbit vrf keyset certificate provider: %w", err)
		}
	}
	return transport, nil
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

// ProtocolV2 preserves every v1 DKG message and adds complete finalizations.
func (n *rabbitVRFDKGTransport) ProtocolV2() p2p.Protocol {
	return p2p.Protocol{
		Name:    rabbitVRFDKGProtocolName,
		Version: rabbitVRFDKGProtocolVersionV2,
		Length:  rabbitVRFDKGProtocolLengthV2,
		Run: func(remote *p2p.Peer, rw p2p.MsgReadWriter) error {
			return n.runPeerVersion(remote, rw, rabbitVRFDKGProtocolVersionV2)
		},
		NodeInfo: func() interface{} { return n.statusVersion(rabbitVRFDKGProtocolVersionV2) },
	}
}

func (n *rabbitVRFDKGTransport) statusVersion(version uint) rabbitVRFDKGStatusPacket {
	status := n.status()
	status.ProtocolVersion = uint32(version)
	return status
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
	return n.runPeerVersion(remote, rw, rabbitVRFDKGProtocolVersion)
}

func (n *rabbitVRFDKGTransport) runPeerVersion(
	remote *p2p.Peer, rw p2p.MsgReadWriter, version uint,
) error {
	peer := &rabbitVRFDKGPeer{
		peer:    remote,
		rw:      rw,
		version: version,
		known:   make(map[common.Hash]struct{}),
	}

	if err := n.handshakeVersion(peer, version); err != nil {
		return err
	}
	if err := n.register(peer); err != nil {
		return err
	}
	defer n.unregister(peer.id())
	stopGossip := make(chan struct{})
	defer close(stopGossip)
	if version == rabbitVRFDKGProtocolVersionV2 {
		go n.gossipFinalizationsToPeerV2(peer, stopGossip)
	}

	go n.gossipDKGArtifactsToPeerV1(peer, stopGossip)
	go func() {
		if err := n.sendPendingLocalThresholdPartialsV1(peer); err != nil {
			log.Warn("Rabbit VRF reconnect partial replay incomplete", "err", err)
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
			if err := n.handleInboundEncryptedRelayV1(peer, packet); err != nil {
				if errors.Is(err, errRabbitVRFDKGArtifactSessionMismatch) {
					continue
				}
				if errors.Is(err, errRabbitVRFDKGSecretOperationUnavailableV1) || errors.Is(err, errRabbitVRFDKGSecretTemporarilyUnavailableV1) {
					// Authentication above remains mandatory. Never decrypt while
					// sync is active; periodic sender replay retries after the gate opens.
					log.Debug("Rabbit VRF DKG encrypted evaluation deferred during sync",
						"session", packet.SessionID, "dealer", packet.DealerShareID,
						"recipient", packet.RecipientShareID)
					continue
				}
				return fmt.Errorf("process rabbit vrf dkg encrypted evaluation: %w", err)
			}

		case rabbitVRFDKGThresholdPartialMsg:
			var packet lqc.RabbitVRFThresholdPartialV1
			if err := message.Decode(&packet); err != nil {
				return fmt.Errorf("decode rabbit vrf threshold partial: %w", err)
			}
			if err := n.handleInboundThresholdPartialV1(peer, packet); err != nil {
				if errors.Is(err, errRabbitVRFDKGArtifactSessionMismatch) ||
					errors.Is(err, errRabbitVRFThresholdContextNotReadyV1) ||
					errors.Is(err, errRabbitVRFRequestNotPendingV1) || errors.Is(err, rabbitvrfstate.ErrDKGTransportKeyStoreMissingV1) {
					peer.peer.Log().Debug("Rabbit VRF threshold partial deferred", "request", packet.RequestID, "share", packet.ShareID, "err", err)
					continue
				}
				return fmt.Errorf("process rabbit vrf threshold partial: %w", err)
			}

		case rabbitVRFDKGFinalizationMsg:
			if version != rabbitVRFDKGProtocolVersionV2 {
				return errors.New("rabbit vrf finalization requires protocol v2")
			}
			var proof lqc.RabbitVRFFinalizationV1
			if err := message.Decode(&proof); err != nil {
				return fmt.Errorf("decode rabbit vrf finalization: %w", err)
			}
			certificate, certErr := n.activeKeysetCertificateV1()
			if certErr != nil {
				log.Debug("Rabbit VRF finalization certificate waiting", "request", proof.RequestID, "err", certErr)
				continue
			}
			if err := proof.Validate(); err != nil {
				return err
			}
			current := n.runtime.thresholdContextV1()
			if proof.Epoch != current.CanonicalSession.TargetVRFEpoch {
				continue
			}
			if certificate.SessionID == (common.Hash{}) || certificate.SessionID != current.SessionID ||
				len(certificate.Signatures) == 0 {
				log.Debug("Rabbit VRF finalization deferred until canonical certificate ready", "request", proof.RequestID)
				continue
			}
			accepted, err := n.acceptFinalizationFromPeerV2(proof, certificate)
			if err != nil {
				return fmt.Errorf("verify rabbit vrf received finalization: %w", err)
			}
			if !accepted {
				peer.markKnown(proof.ProofHash)
				log.Info("Rabbit VRF verified late finalization ignored; peer kept", "request", proof.RequestID, "peer", peer.id())
				continue
			}
			log.Info("Rabbit VRF proof received and validated",
				"request", proof.RequestID,
				"proof", proof.ProofHash,
				"peer", peer.id())

			peer.markKnown(proof.ProofHash)

		case rabbitVRFDKGKeysetCertificateMsg:
			var envelope lqc.RabbitVRFDKGEnvelopeV1
			if err := message.Decode(&envelope); err != nil {
				return fmt.Errorf(
					"decode rabbit vrf keyset certificate signature: %w",
					err,
				)
			}

			inserted, err :=
				n.handleInboundKeysetCertificateEnvelopeV1(
					peer,
					envelope,
				)
			if err != nil {
				if errors.Is(
					err,
					errRabbitVRFDKGArtifactSessionMismatch,
				) {
					continue
				}

				return fmt.Errorf(
					"process rabbit vrf keyset certificate signature: %w",
					err,
				)
			}

			if inserted {
				n.broadcastKeysetCertificateEnvelopeV1(
					envelope,
					peer.id(),
				)
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
	return n.handshakeVersion(peer, rabbitVRFDKGProtocolVersion)
}

func (n *rabbitVRFDKGTransport) handshakeVersion(
	peer *rabbitVRFDKGPeer, version uint,
) error {
	status := n.statusVersion(version)
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
	replay ...bool,
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
	if _, exists := peer.known[hash]; exists && !(len(replay) > 0 && replay[0]) {
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
	replay ...bool,
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
	if _, exists := peer.known[hash]; exists && !(len(replay) > 0 && replay[0]) {
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

func (peer *rabbitVRFDKGPeer) sendThresholdPartialV1(packet lqc.RabbitVRFThresholdPartialV1, replay ...bool) error {
	if peer == nil || peer.rw == nil {
		return errors.New("invalid rabbit vrf threshold partial peer")
	}
	messageID, err := lqc.RabbitVRFThresholdPartialMessageIDV1(packet)
	if err != nil {
		return err
	}
	peer.mu.Lock()
	defer peer.mu.Unlock()
	if peer.known == nil {
		peer.known = make(map[common.Hash]struct{})
	}
	if _, exists := peer.known[messageID]; exists && (len(replay) == 0 || !replay[0]) {
		return nil
	}
	if err := p2p.Send(peer.rw, rabbitVRFDKGThresholdPartialMsg, packet); err != nil {
		return err
	}
	if len(peer.known) >= rabbitVRFDKGMaxKnownPerPeer {
		clear(peer.known)
	}
	peer.known[messageID] = struct{}{}
	return nil
}

func (n *rabbitVRFDKGTransport) rabbitVRFFinalizationsForBlockV1(blockNumber uint64) ([]lqc.RabbitVRFFinalizationV1, error) {
	bridge, epochLength, exists, err := n.rabbitVRFServingBridgeForBlockV1(blockNumber)
	if err != nil || !exists {
		return nil, err
	}
	_, finalizations, _, err := n.rabbitVRFArtifactsForBridgeV1(blockNumber, epochLength, bridge)
	return finalizations, err
}

func (n *rabbitVRFDKGTransport) processCanonicalPendingRequestV1(requestID common.Hash) error {

	rabbitPhaseStart := time.Now()
	rabbitPhaseLast := rabbitPhaseStart
	rabbitPhaseMark := func(stage string) {
		now := time.Now()
		log.Info("Rabbit VRF phase timing",
			"request", requestID,
			"phase", stage,
			"step", now.Sub(rabbitPhaseLast),
			"total", now.Sub(rabbitPhaseStart))
		rabbitPhaseLast = now
	}
	defer rabbitPhaseMark("process-exit")
	if n == nil || n.runtime == nil || n.runtime.backend == nil || n.runtime.backend.vrfDKGInstanceDir == "" {
		return errors.New("rabbit vrf threshold local flow unavailable")
	}
	if _, err := n.runtime.canonicalPendingRequestV1(requestID); err != nil {
		return fmt.Errorf("rabbit vrf canonical pending request: %w", err)
	}

	context := n.runtime.thresholdContextV1()
	if context.SessionID == (common.Hash{}) || len(context.Members) == 0 {
		return errors.New("rabbit vrf local threshold committee unavailable")
	}

	keysetStore, err := rabbitvrfstate.NewDKGFinalKeysetStoreV1(filepath.Join(n.runtime.backend.vrfDKGInstanceDir, "rabbit-vrf", "dkg-final-keysets"))
	if err != nil {
		return fmt.Errorf("open rabbit vrf dkg final keyset store: %w", err)
	}
	keyset, err := keysetStore.Load(context.CanonicalSession)
	if err != nil {
		return fmt.Errorf("load rabbit vrf dkg final keyset: %w", err)
	}

	rabbitPhaseMark("process-context-keyset-loaded")
	message, _, err := lqc.RabbitVRFThresholdMessageV1(context.CanonicalSession, keyset.KeysetRoot, requestID)
	if err != nil {
		return fmt.Errorf("build rabbit vrf threshold message: %w", err)
	}

	partialStore, err := rabbitvrfstate.NewThresholdPartialStoreV1(filepath.Join(n.runtime.backend.vrfDKGInstanceDir, "rabbit-vrf", "threshold-partials"))
	if err != nil {
		return fmt.Errorf("open rabbit vrf threshold partial store: %w", err)
	}

	// Restore all persisted partials that belong to the finalized canonical
	// keyset before creating any new local partial. Persisted packets are fed
	// back through collectThresholdPartialV1, so restart recovery re-runs the
	// normal canonical/session/share cryptographic validation path.
	for _, publicShare := range keyset.VerificationShares {
		packet, loadErr := partialStore.Load(
			context.CanonicalSession,
			keyset.KeysetRoot,
			requestID,
			publicShare.ShareID,
		)
		if loadErr != nil {
			if errors.Is(loadErr, os.ErrNotExist) ||
				errors.Is(loadErr, rabbitvrfstate.ErrDKGTransportKeyStoreMissingV1) {
				continue
			}
			return fmt.Errorf(
				"load persisted rabbit vrf threshold partial share %d: %w",
				publicShare.ShareID,
				loadErr,
			)
		}
		if err := n.collectThresholdPartialV1(packet); err != nil {
			return fmt.Errorf(
				"collect persisted rabbit vrf threshold partial share %d: %w",
				publicShare.ShareID,
				err,
			)
		}
	}

	// Only local committee members may create and broadcast new partials. A
	// local partial already restored above is left untouched and need not be
	// re-signed after restart.
	rabbitPhaseMark("process-persisted-partials-restored")
	for _, member := range context.Members {
		packet, loadErr := partialStore.Load(
			context.CanonicalSession,
			keyset.KeysetRoot,
			requestID,
			member.ShareID,
		)
		if loadErr == nil {
			// It was already restored and validated above. Re-broadcast only a
			// local member's own partial so peer route authentication remains
			// bound to the original ShareID owner.
			if err := n.broadcastThresholdPartialV1(packet, "", true); err != nil {
				log.Warn("Rabbit VRF persisted partial broadcast incomplete", "request", requestID, "share", member.ShareID, "err", err)
			}
			continue
		}
		if !errors.Is(loadErr, os.ErrNotExist) &&
			!errors.Is(loadErr, rabbitvrfstate.ErrDKGTransportKeyStoreMissingV1) {
			return fmt.Errorf("load rabbit vrf threshold partial share %d: %w", member.ShareID, loadErr)
		}

		packet, signErr := n.runtime.signLocalThresholdPacketOnceV1(member, message, keyset.KeysetRoot, requestID)
		if signErr != nil {
			return fmt.Errorf("sign rabbit vrf threshold packet share %d: %w", member.ShareID, signErr)
		}

		rabbitPhaseMark("process-bls-signed")
		participationSignature, signErr := n.runtime.signLocalThresholdParticipationV1(member, packet)
		if signErr != nil {
			return fmt.Errorf("sign rabbit vrf threshold participation share %d: %w", member.ShareID, signErr)
		}
		rabbitPhaseMark("process-participation-signed")
		packet.ParticipationSignature = participationSignature
		if signErr := partialStore.Store(context.CanonicalSession, packet); signErr != nil {
			return fmt.Errorf("persist rabbit vrf threshold partial share %d: %w", member.ShareID, signErr)
		}
		rabbitPhaseMark("process-local-partial-persisted")
		if err := n.collectThresholdPartialV1(packet); err != nil {
			return fmt.Errorf("collect local rabbit vrf threshold partial share %d: %w", member.ShareID, err)
		}
		rabbitPhaseMark("process-local-partial-collected")
		if err := n.broadcastThresholdPartialV1(packet, ""); err != nil {
			log.Warn("Rabbit VRF local partial broadcast incomplete", "request", requestID, "share", member.ShareID, "err", err)
		}
	}
	return nil
}

// pendingLocalThresholdPartialsV1 snapshots only authenticated local shares
// for the active session. Bound replay to prevent a reconnect from blocking
// indefinitely on historical requests; fresh pending requests are handled by
// the normal canonical request worker.
func (n *rabbitVRFDKGTransport) pendingLocalThresholdPartialsV1(context rabbitVRFDKGLocalContextV1) []lqc.RabbitVRFThresholdPartialV1 {
	if n == nil || context.SessionID == (common.Hash{}) || len(context.Members) == 0 {
		return nil
	}
	local := make(map[uint64]struct{}, len(context.Members))
	for _, member := range context.Members {
		local[member.ShareID] = struct{}{}
	}
	n.mu.RLock()
	defer n.mu.RUnlock()
	if n.partialSession != context.SessionID || n.partialKeysetRoot == (common.Hash{}) {
		return nil
	}
	out := make([]lqc.RabbitVRFThresholdPartialV1, 0)
	for _, bucket := range n.partials {
		for shareID, packet := range bucket {
			if _, ok := local[shareID]; ok && packet.SessionID == context.SessionID && packet.KeysetRoot == n.partialKeysetRoot {
				out = append(out, packet)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].RequestID != out[j].RequestID {
			return string(out[i].RequestID[:]) < string(out[j].RequestID[:])
		}
		return out[i].ShareID < out[j].ShareID
	})
	const maxReplay = 128
	if len(out) > maxReplay {
		out = out[len(out)-maxReplay:]
	}
	return out
}

func (n *rabbitVRFDKGTransport) sendPendingLocalThresholdPartialsV1(peer *rabbitVRFDKGPeer) error {
	if n == nil || n.runtime == nil || peer == nil {
		return nil
	}
	context := n.runtime.thresholdContextV1()
	packets := n.pendingLocalThresholdPartialsV1(context)
	for _, packet := range packets {
		if n.runtime.thresholdContextV1().SessionID != context.SessionID {
			return nil
		}
		if err := peer.sendThresholdPartialV1(packet); err != nil {
			return fmt.Errorf("replay rabbit vrf local partial: %w", err)
		}
	}
	return nil
}

func (n *rabbitVRFDKGTransport) broadcastThresholdPartialV1(packet lqc.RabbitVRFThresholdPartialV1, excludePeerID string, replay ...bool) error {
	if n == nil {
		return errors.New("invalid rabbit vrf threshold partial transport")
	}
	n.mu.RLock()
	peers := make([]*rabbitVRFDKGPeer, 0, len(n.peers))
	for id, peer := range n.peers {
		if id != excludePeerID && peer != nil {
			peers = append(peers, peer)
		}
	}
	n.mu.RUnlock()
	var sendErrors []error
	for _, peer := range peers {
		if err := peer.sendThresholdPartialV1(packet, replay...); err != nil {
			sendErrors = append(sendErrors, err)
		}
	}
	return errors.Join(sendErrors...)
}

// Partial packets are self-authenticating through the canonical verification
// share, compact BLS participation proof and bound transport signature. The
// RLPx connection authenticates the registered relay, not ownership of ShareID.
func (n *rabbitVRFDKGTransport) thresholdPartialPeerAuthenticatedV1(peer *rabbitVRFDKGPeer, packet lqc.RabbitVRFThresholdPartialV1) bool {
	if n == nil || peer == nil || peer.peer == nil || packet.SessionID == (common.Hash{}) || packet.ShareID == 0 {
		return false
	}
	n.mu.RLock()
	defer n.mu.RUnlock()
	registered, ok := n.peers[peer.id()]
	return !n.closed && ok && registered == peer
}

func (n *rabbitVRFDKGTransport) handleInboundThresholdPartialV1(peer *rabbitVRFDKGPeer, packet lqc.RabbitVRFThresholdPartialV1) error {
	if n == nil || n.runtime == nil {
		return errors.New("rabbit vrf threshold partial transport unavailable")
	}
	context := n.runtime.thresholdContextV1()
	// Stale packets cannot be authenticated against a new session's routes.
	if context.SessionID == (common.Hash{}) || packet.SessionID != context.SessionID {
		return errRabbitVRFDKGArtifactSessionMismatch
	}
	if !n.thresholdPartialPeerAuthenticatedV1(peer, packet) {
		return errors.New("rabbit vrf threshold partial sender route mismatch")
	}
	if _, err := lqc.RabbitVRFThresholdPartialMessageIDV1(packet); err != nil {
		return err
	}
	if _, err := n.thresholdPartialRequestReadyV1(packet.RequestID); err != nil {
		return err
	}
	if n.runtime.backend == nil || n.runtime.backend.vrfDKGInstanceDir == "" {
		return errors.New("rabbit vrf threshold partial persistence unavailable")
	}
	if context.CanonicalTransportKeySetRoot == (common.Hash{}) ||
		uint64(len(context.CanonicalMembers)) != context.CanonicalSession.CommitteeSize ||
		len(context.CanonicalTransportBindings) != len(context.CanonicalMembers) {
		return errRabbitVRFThresholdContextNotReadyV1
	}
	store, err := rabbitvrfstate.NewDKGFinalKeysetStoreV1(filepath.Join(n.runtime.backend.vrfDKGInstanceDir, "rabbit-vrf", "dkg-final-keysets"))
	if err != nil {
		return err
	}
	if _, err := store.Load(context.CanonicalSession); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return errRabbitVRFThresholdContextNotReadyV1
		}
		return err // Corrupt or conflicting keysets are never treated as missing.
	}
	// No packet is stored, counted or rewarded before all existing validators
	// pass. Membership, request, keyset, BLS and participation checks stay intact.
	n.mu.RLock()
	previous, existed := n.partials[packet.RequestID][packet.ShareID]
	existed = existed && n.partialSession == packet.SessionID && n.partialKeysetRoot == packet.KeysetRoot && previous == packet
	n.mu.RUnlock()
	if err := n.collectThresholdPartialV1(packet); err != nil {
		return err
	}
	if !existed {
		_ = n.broadcastThresholdPartialV1(packet, peer.id())
	}
	return nil
}

func (n *rabbitVRFDKGTransport) collectThresholdPartialV1(packet lqc.RabbitVRFThresholdPartialV1) error {

	rabbitPhaseStart := time.Now()
	rabbitPhaseLast := rabbitPhaseStart
	rabbitPhaseMark := func(stage string) {
		now := time.Now()
		log.Info("Rabbit VRF phase timing",
			"request", packet.RequestID,
			"phase", stage,
			"step", now.Sub(rabbitPhaseLast),
			"total", now.Sub(rabbitPhaseStart))
		rabbitPhaseLast = now
	}
	defer rabbitPhaseMark("collect-exit")
	if n == nil || n.runtime == nil {
		return errors.New("rabbit vrf threshold partial transport unavailable")
	}
	if err := n.runtime.validateInboundThresholdPartialV1(packet); err != nil {
		return err
	}
	rabbitPhaseMark("collect-inbound-validation-done")
	context := n.runtime.thresholdContextV1()
	message, messageHash, err := lqc.RabbitVRFThresholdMessageV1(context.CanonicalSession, packet.KeysetRoot, packet.RequestID)
	if err != nil || messageHash != packet.MessageHash {
		return errors.New("rabbit vrf threshold partial canonical message mismatch")
	}

	if n.runtime.backend == nil || n.runtime.backend.vrfDKGInstanceDir == "" {
		return errors.New("rabbit vrf threshold partial persistence unavailable")
	}
	partialStore, err := rabbitvrfstate.NewThresholdPartialStoreV1(
		filepath.Join(n.runtime.backend.vrfDKGInstanceDir, "rabbit-vrf", "threshold-partials"),
	)
	if err != nil {
		return fmt.Errorf("open rabbit vrf threshold partial store: %w", err)
	}
	if err := partialStore.Store(context.CanonicalSession, packet); err != nil {
		return fmt.Errorf("persist verified rabbit vrf threshold partial share %d: %w", packet.ShareID, err)
	}

	rabbitPhaseMark("collect-persistence-done")
	n.mu.Lock()
	if n.partialSession != packet.SessionID || n.partialKeysetRoot != packet.KeysetRoot {
		n.partialSession = packet.SessionID
		n.partialKeysetRoot = packet.KeysetRoot
		n.partials = make(map[common.Hash]map[uint64]lqc.RabbitVRFThresholdPartialV1)
		n.partialResults = make(map[common.Hash]rabbitVRFThresholdResultV1)
	}
	bucket := n.partials[packet.RequestID]
	if bucket == nil {
		bucket = make(map[uint64]lqc.RabbitVRFThresholdPartialV1)
		n.partials[packet.RequestID] = bucket
	}
	if existing, ok := bucket[packet.ShareID]; ok {
		if existing != packet {
			n.mu.Unlock()
			return errors.New("rabbit vrf threshold partial share conflict")
		}
		n.mu.Unlock()
		return nil
	}
	if _, done := n.partialResults[packet.RequestID]; done {
		n.mu.Unlock()
		return nil
	}
	bucket[packet.ShareID] = packet
	threshold := int(context.CanonicalSession.Threshold)
	if len(bucket) < threshold {
		n.mu.Unlock()
		return nil
	}
	shareIDs := make([]uint64, 0, len(bucket))
	for shareID := range bucket {
		shareIDs = append(shareIDs, shareID)
	}
	sort.Slice(shareIDs, func(i, j int) bool { return shareIDs[i] < shareIDs[j] })
	if len(shareIDs) > threshold {
		shareIDs = shareIDs[:threshold]
	}

	selectedPackets := make([]lqc.RabbitVRFThresholdPartialV1, 0, threshold)
	partials := make([]rabbitvrf.PartialSignature, 0, threshold)
	compactSignatures := make([]rabbitvrf.Signature, 0, threshold)
	for _, shareID := range shareIDs {
		stored := bucket[shareID]
		selectedPackets = append(selectedPackets, stored)
		partials = append(partials, rabbitvrf.PartialSignature{
			ShareID:   stored.ShareID,
			Signature: stored.Signature,
		})
		compactSignatures = append(
			compactSignatures,
			stored.CompactParticipationSignature,
		)
	}
	n.mu.Unlock()

	if n.runtime.backend == nil || n.runtime.backend.vrfDKGInstanceDir == "" {
		return errors.New("rabbit vrf threshold keyset runtime unavailable")
	}
	keysetStore, err := rabbitvrfstate.NewDKGFinalKeysetStoreV1(filepath.Join(n.runtime.backend.vrfDKGInstanceDir, "rabbit-vrf", "dkg-final-keysets"))
	if err != nil {
		return fmt.Errorf("open rabbit vrf dkg final keyset store: %w", err)
	}
	keyset, err := keysetStore.Load(context.CanonicalSession)
	if err != nil {
		return fmt.Errorf("load rabbit vrf dkg final keyset: %w", err)
	}
	rabbitPhaseMark("collect-keyset-ready-for-combination")
	signature, randomness, err := rabbitVRFCombineThresholdPartialsWithKeysetV1(context.CanonicalSession.Threshold, keyset, message, partials)
	if err != nil {
		return fmt.Errorf("combine rabbit vrf threshold request %s: %w", packet.RequestID, err)
	}

	rabbitPhaseMark("collect-threshold-combination-done")
	participationAggregateSignature, err := rabbitvrf.AggregateSignaturesV1(compactSignatures)
	if err != nil {
		return fmt.Errorf("aggregate rabbit vrf compact participation request %s: %w", packet.RequestID, err)
	}
	selectedShareIDs := make([]uint64, len(selectedPackets))
	for index, selected := range selectedPackets {
		selectedShareIDs[index] = selected.ShareID
	}
	participationBitmap, err := lqc.RabbitVRFCompactParticipationFixedBitmapV1(
		context.CanonicalSession,
		selectedShareIDs,
	)
	if err != nil {
		return fmt.Errorf("build rabbit vrf compact participation bitmap request %s: %w", packet.RequestID, err)
	}

	request, err := n.runtime.canonicalPendingRequestV1(packet.RequestID)
	if err != nil {
		return fmt.Errorf("resolve rabbit vrf request for finalization: %w", err)
	}
	epoch := context.CanonicalSession.TargetVRFEpoch
	round := request.RequestBlock
	if epoch == 0 || round == 0 {
		return errors.New("rabbit vrf finalization canonical epoch/round unavailable")
	}
	proofHash, err := lqc.RabbitVRFFinalizationProofHashWithParticipationV1(
		packet.RequestID,
		epoch,
		round,
		signature[:],
		participationBitmap,
		participationAggregateSignature,
	)
	if err != nil {
		return fmt.Errorf("build rabbit vrf finalization proof hash: %w", err)
	}
	finalization := lqc.RabbitVRFFinalizationV1{
		Version:                         lqc.RabbitVRFFinalizationVersionV1,
		RequestID:                       packet.RequestID,
		KeysetRoot:                      packet.KeysetRoot,
		Epoch:                           epoch,
		Round:                           round,
		Randomness:                      randomness,
		ProofHash:                       proofHash,
		Signature:                       signature,
		ParticipationBitmap:             participationBitmap,
		ParticipationAggregateSignature: participationAggregateSignature,
	}
	if err := finalization.Validate(); err != nil {
		return fmt.Errorf("validate locally produced rabbit vrf finalization: %w", err)
	}

	n.mu.Lock()
	defer n.mu.Unlock()
	if n.partialSession != packet.SessionID || n.partialKeysetRoot != packet.KeysetRoot {
		return errRabbitVRFDKGArtifactSessionMismatch
	}
	if existing, ok := n.partialResults[packet.RequestID]; ok {
		if existing.Signature != signature || existing.Randomness != randomness {
			return errors.New("rabbit vrf threshold result conflict")
		}
		if existingFinalization, ok := n.finalizations[rabbitVRFFinalizationKeyV1{SessionID: packet.SessionID, RequestID: packet.RequestID}]; ok &&
			existingFinalization != finalization {
			return errors.New("rabbit vrf finalization conflict")
		}
		return nil
	}
	if existingFinalization, ok := n.finalizations[rabbitVRFFinalizationKeyV1{SessionID: packet.SessionID, RequestID: packet.RequestID}]; ok &&
		existingFinalization != finalization {
		return errors.New("rabbit vrf finalization conflict")
	}
	if n.finalizations == nil {
		n.finalizations = make(map[rabbitVRFFinalizationKeyV1]lqc.RabbitVRFFinalizationV1)
	}
	n.partialResults[packet.RequestID] = rabbitVRFThresholdResultV1{
		Signature:  signature,
		Randomness: randomness,
	}
	n.finalizations[rabbitVRFFinalizationKeyV1{SessionID: packet.SessionID, RequestID: packet.RequestID}] = finalization
	rabbitPhaseMark("collect-proof-ready")
	log.Info("Rabbit VRF proof locally ready",
		"request", packet.RequestID,
		"proof", finalization.ProofHash,
		"epoch", finalization.Epoch)
	// Wake every V2 gossip loop after the proof is stored.
	// The transport mutex is already held here.
	if n.finalizationWakeV2 != nil {
		close(n.finalizationWakeV2)
	}
	n.finalizationWakeV2 = make(chan struct{})
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
	replay ...bool,
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
			cloneRabbitVRFDKGTransportArtifactPacketV1(packet), replay...,
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

func (n *rabbitVRFDKGTransport) sendLocalPolynomialCommitmentsV1(peer *rabbitVRFDKGPeer, replay ...bool) error {
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
		if err := peer.sendPolynomialCommitmentV1(cloneRabbitVRFDKGPolynomialCommitmentPacketV1(packet), replay...); err != nil {
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

var errRabbitVRFDKGSecretTemporarilyUnavailableV1 = errors.New(
	"rabbit vrf dkg secret operation temporarily unavailable",
)

// Renew session artifacts on established connections. Receiving peers may
// have discarded an earlier transmission while observing a different session.
func (n *rabbitVRFDKGTransport) syncPeerArtifactsUntilDoneV1(
	peer *rabbitVRFDKGPeer,
	done <-chan struct{},
	interval time.Duration,
) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return nil
		case <-n.runtime.stop:
			return nil
		default:
		}

		if err := n.sendLocalPeerRouteProofsV1(peer); err != nil {
			return err
		}
		if err := n.sendPendingTransportArtifactsV1(peer); err != nil {
			return err
		}
		if err := n.sendPendingAuthenticatedRelayV1(peer); err != nil {
			return err
		}

		select {
		case <-done:
			return nil
		case <-n.runtime.stop:
			return nil
		case <-ticker.C:
			// A successful write does not prove acceptance by the recipient.
			// Permit authenticated artifacts to be retried at a bounded rate.
			peer.mu.Lock()
			clear(peer.known)
			peer.mu.Unlock()
		}
	}
}

// An unknown request is not accepted or persisted. Keep the connection alive
// so canonical synchronization and later sender replay can proceed.
func (n *rabbitVRFDKGTransport) thresholdPartialRequestReadyV1(requestID common.Hash) (rabbitVRFCanonicalRequestV1, error) {
	request, err := n.runtime.canonicalPendingRequestV1(requestID)
	if errors.Is(err, errRabbitVRFRequestMissingV1) {
		return rabbitVRFCanonicalRequestV1{}, errRabbitVRFThresholdContextNotReadyV1
	}
	return request, err
}
