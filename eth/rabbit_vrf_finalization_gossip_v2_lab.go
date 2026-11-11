//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"errors"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p"
	"golang.org/x/sync/singleflight"
	"time"
)

// A new proof is announced immediately through finalizationWakeV2.
// Periodic replay is only for recovery after lost messages and is intentionally
// slower than the ordinary gossip tick: cryptographic finalization validation
// is expensive and the old 2-second forced replay ran once per connected peer.
const rabbitVRFFinalizationReplayIntervalV2 = 30 * time.Second

// Only verified, locally eligible finalizations are gossiped to v2 peers.
func (n *rabbitVRFDKGTransport) gossipFinalizationsToPeerV2(peer *rabbitVRFDKGPeer, stop <-chan struct{}) {
	ticker := time.NewTicker(rabbitVRFFinalizationReplayIntervalV2)
	defer ticker.Stop()
	replay := false
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
		n.sendFinalizationsToPeerV2(peer, replay)
		select {
		case <-stop:
			return
		case <-wake:
			// New finalization: send new proofs immediately, without re-sending
			// ones already acknowledged by a successful local write.
			replay = false
		case <-ticker.C:
			// Keep a bounded repair path for peers that missed an announcement.
			replay = true
		}
	}
}

// rabbitVRFCoalescedFinalizationScanV2 shares only in-flight scans. No cached
// proofs survive after the scan finishes, so canonical-pending changes and
// a same-head wake trigger new validation. Keys include the head hash.
func rabbitVRFCoalescedFinalizationScanV2(
	flights *singleflight.Group,
	canonicalHead string,
	scan func() ([]lqc.RabbitVRFFinalizationV1, error),
) ([]lqc.RabbitVRFFinalizationV1, error) {
	if flights == nil || scan == nil {
		return nil, errors.New("invalid rabbit vrf finalization scan")
	}
	v, err, _ := flights.Do(canonicalHead, func() (interface{}, error) {
		return scan()
	})
	if err != nil {
		return nil, err
	}
	return v.([]lqc.RabbitVRFFinalizationV1), nil
}

func (n *rabbitVRFDKGTransport) sendFinalizationsToPeerV2(peer *rabbitVRFDKGPeer, replay bool) {
	if n == nil || n.runtime == nil || n.runtime.backend == nil ||
		n.runtime.backend.blockchain == nil || peer == nil || peer.rw == nil ||
		peer.version != rabbitVRFDKGProtocolVersionV2 {
		return
	}
	head := n.runtime.backend.blockchain.CurrentBlock()
	if head == nil {
		return
	}
	// A peer burst for the same canonical head must not recompute
	// BLS12-381 verification independently for every peer. singleflight
	// coalesces only overlapping scans: it is NOT a persistent cache.
	// The head hash separates reorgs, while a later wake always rescans.
	proofs, err := rabbitVRFCoalescedFinalizationScanV2(
		&n.finalizationScanFlightsV2, head.Hash().Hex(),
		func() ([]lqc.RabbitVRFFinalizationV1, error) {
			return n.rabbitVRFFinalizationsForBlockV1(head.Number.Uint64() + 1)
		},
	)
	if err != nil {
		return
	}
	for _, proof := range proofs {
		if err := peer.sendVerifiedFinalizationV2(proof, replay); err != nil {
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
