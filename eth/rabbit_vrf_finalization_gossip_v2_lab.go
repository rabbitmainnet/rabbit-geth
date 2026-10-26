//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p"
)

// Only verified, locally eligible finalizations are gossiped to v2 peers.
func (n *rabbitVRFDKGTransport) gossipFinalizationsToPeerV2(peer *rabbitVRFDKGPeer, stop <-chan struct{}) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		// Do not start another gossip round after peer shutdown.
		select {
		case <-stop:
			return
		default:
		}

		// Snapshot the broadcast channel before scanning. If a proof is
		// finalized during the scan, the old channel closes and wakes us.
		wake := n.finalizationWakeSignalV2()
		n.sendFinalizationsToPeerV2(peer)
		select {
		case <-stop:
			return
		case <-wake:
		case <-ticker.C:
		}
	}
}

func (n *rabbitVRFDKGTransport) sendFinalizationsToPeerV2(peer *rabbitVRFDKGPeer) {
	if n == nil || n.runtime == nil || n.runtime.backend == nil ||
		n.runtime.backend.blockchain == nil || peer == nil || peer.rw == nil ||
		peer.version != rabbitVRFDKGProtocolVersionV2 {
		return
	}
	head := n.runtime.backend.blockchain.CurrentBlock()
	if head == nil {
		return
	}
	proofs, err := n.rabbitVRFFinalizationsForBlockV1(head.Number.Uint64() + 1)
	if err != nil {
		return
	}
	for _, proof := range proofs {
		if err := peer.sendVerifiedFinalizationV2(proof, true); err != nil {
			return
		}
	}
}

func (peer *rabbitVRFDKGPeer) sendVerifiedFinalizationV2(proof lqc.RabbitVRFFinalizationV1, replay ...bool) error {
	peer.mu.Lock()
	defer peer.mu.Unlock()
	if peer.known == nil {
		peer.known = make(map[common.Hash]struct{})
	}
	if _, ok := peer.known[proof.ProofHash]; ok && (len(replay) == 0 || !replay[0]) {
		return nil
	}
	if err := p2p.Send(peer.rw, rabbitVRFDKGFinalizationMsg, proof); err != nil {
		return err
	}
	if len(peer.known) >= rabbitVRFDKGMaxKnownPerPeer {
		clear(peer.known)
	}
	peer.known[proof.ProofHash] = struct{}{}
	log.Info("Rabbit VRF proof sent to peer",
		"request", proof.RequestID,
		"proof", proof.ProofHash,
		"peer", peer.id())
	return nil
}

// A closed channel broadcasts to all connected V2 peers.
// Snapshot before scanning so a simultaneous finalization is not missed.
func (n *rabbitVRFDKGTransport) finalizationWakeSignalV2() <-chan struct{} {
	if n == nil {
		return nil
	}
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.finalizationWakeV2
}
