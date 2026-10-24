//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"time"

	"github.com/ethereum/go-ethereum/log"
)

// Peer connections outlive DKG sessions. A successful socket write is not an
// acknowledgement: a lagging peer may discard an artifact for a future session.
// Replay authenticated public artifacts, bypassing only sender-side deduplication.
// Receiver authentication, session checks and conflict checks remain unchanged.
func (n *rabbitVRFDKGTransport) gossipDKGArtifactsToPeerV1(peer *rabbitVRFDKGPeer, stop <-chan struct{}) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	var lastWarning time.Time
	// The initial send races with inbound gossip. Respect known hashes on
	// that pass; only timed recovery passes may bypass sender deduplication.
	replay := false
	for {
		select {
		case <-stop:
			return
		default:
		}
		err := n.sendLocalPeerRouteProofsV1(peer)
		if err == nil {
			err = n.sendPendingTransportArtifactsV1(peer, replay)
		}
		if err == nil {
			err = n.sendPendingAuthenticatedRelayV1(peer)
		}
		if err != nil && time.Since(lastWarning) >= 10*time.Second {
			log.Warn("Rabbit VRF DKG artifact replay waiting", "peer", peer.id(), "err", err)
			lastWarning = time.Now()
		}
		select {
		case <-stop:
			return
		case <-ticker.C:
			replay = true
		}
	}
}
