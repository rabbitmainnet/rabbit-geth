//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"sort"

	"github.com/ethereum/go-ethereum/common"
)

func (n *rabbitVRFDKGTransport) sendRetainedRemoteTransportArtifactsV1(
	peer *rabbitVRFDKGPeer,
) error {
	if n == nil || n.runtime == nil || peer == nil {
		return nil
	}

	context := n.runtime.currentContext()
	if context.SessionID == (common.Hash{}) {
		return nil
	}

	n.reconcileRemoteSessionV1(context.SessionID)

	n.mu.RLock()
	if n.closed || n.remoteSession != context.SessionID {
		n.mu.RUnlock()
		return nil
	}

	shareIDs := make([]uint64, 0, len(n.remoteArtifacts))
	for shareID := range n.remoteArtifacts {
		shareIDs = append(shareIDs, shareID)
	}
	sort.Slice(shareIDs, func(i, j int) bool {
		return shareIDs[i] < shareIDs[j]
	})

	packets := make(
		[]rabbitVRFDKGTransportArtifactPacketV1,
		0,
		len(shareIDs),
	)
	for _, shareID := range shareIDs {
		packets = append(
			packets,
			cloneRabbitVRFDKGTransportArtifactPacketV1(
				n.remoteArtifacts[shareID],
			),
		)
	}
	n.mu.RUnlock()

	for _, packet := range packets {
		current := n.runtime.currentContext()
		if current.SessionID != context.SessionID {
			return nil
		}

		if err := peer.sendTransportArtifactV1(packet); err != nil {
			return err
		}
	}

	return nil
}

func (n *rabbitVRFDKGTransport) sendPendingTransportArtifactsV1(
	peer *rabbitVRFDKGPeer,
) error {
	if err := n.sendLocalTransportArtifactsV1(peer); err != nil {
		return err
	}

	return n.sendRetainedRemoteTransportArtifactsV1(peer)
}
