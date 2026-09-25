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
	"github.com/ethereum/go-ethereum/p2p"
)

const (
	rabbitVRFDKGProtocolName    = "rvrfdkg"
	rabbitVRFDKGProtocolVersion = uint(1)
	rabbitVRFDKGProtocolLength  = uint64(1)

	rabbitVRFDKGStatusMsg = uint64(0)

	rabbitVRFDKGHandshakeTimeout = 5 * time.Second
	rabbitVRFDKGMaxMessageSize   = 16 * 1024
)

var (
	errRabbitVRFDKGProtocolClosed = errors.New("rabbit vrf dkg protocol closed")
	errRabbitVRFDKGPeerKnown      = errors.New("rabbit vrf dkg peer already connected")
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

type rabbitVRFDKGTransportConfig struct {
	ChainID   *big.Int
	NetworkID uint64
	Genesis   common.Hash
}

type rabbitVRFDKGTransport struct {
	chainID   *big.Int
	networkID uint64
	genesis   common.Hash

	mu     sync.RWMutex
	peers  map[string]*rabbitVRFDKGPeer
	closed bool
}

type rabbitVRFDKGPeer struct {
	peer *p2p.Peer
	rw   p2p.MsgReadWriter
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
		chainID:   new(big.Int).Set(config.ChainID),
		networkID: config.NetworkID,
		genesis:   config.Genesis,
		peers:     make(map[string]*rabbitVRFDKGPeer),
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

func (n *rabbitVRFDKGTransport) runPeer(
	remote *p2p.Peer,
	rw p2p.MsgReadWriter,
) error {
	peer := &rabbitVRFDKGPeer{peer: remote, rw: rw}

	if err := n.handshake(peer); err != nil {
		return err
	}
	if err := n.register(peer); err != nil {
		return err
	}
	defer n.unregister(peer.id())

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
		return fmt.Errorf(
			"invalid rabbit vrf dkg message code: %d",
			message.Code,
		)
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
	n.mu.Unlock()
}

func (n *rabbitVRFDKGTransport) Close() {
	if n == nil {
		return
	}
	n.mu.Lock()
	n.closed = true
	n.peers = make(map[string]*rabbitVRFDKGPeer)
	n.mu.Unlock()
}
