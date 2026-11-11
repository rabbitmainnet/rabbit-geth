//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"errors"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/log"
)

// Peer connections outlive DKG sessions. A successful socket write is not an
// acknowledgement: a lagging peer may discard an artifact for a future session.
// Replay authenticated public artifacts, bypassing only sender-side deduplication.
// Receiver authentication, session checks and conflict checks remain unchanged.
// rabbitVRFDKGReplayRoundV1 always attempts public authenticated artifact
// delivery even if peer-route proof delivery fails. An outbound route proof
// failure must not starve other DKG participants of public key bindings.
// Relay retains its original prerequisite: route and artifact sends succeed.
func rabbitVRFDKGReplayRoundV1(route, artifacts, relay func() error) error {
	routeErr := route()
	artifactErr := artifacts()
	if routeErr != nil || artifactErr != nil {
		var failures []error
		if routeErr != nil {
			failures = append(failures, fmt.Errorf("peer-route: %w", routeErr))
		}
		if artifactErr != nil {
			failures = append(failures, fmt.Errorf("transport-artifacts: %w", artifactErr))
		}
		return errors.Join(failures...)
	}
	if err := relay(); err != nil {
		return fmt.Errorf("authenticated-relay: %w", err)
	}
	return nil
}

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
		err := rabbitVRFDKGReplayRoundV1(
			func() error { return n.sendLocalPeerRouteProofsV1(peer) },
			func() error { return n.sendPendingTransportArtifactsV1(peer, replay) },
			func() error { return n.sendPendingAuthenticatedRelayV1(peer) },
		)
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
