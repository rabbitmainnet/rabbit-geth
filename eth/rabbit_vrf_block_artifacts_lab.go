//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"errors"
	"fmt"
	"os"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/internal/rabbitvrfstate"
)

type rabbitVRFFinalizationKeyV1 struct {
	SessionID common.Hash
	RequestID common.Hash
}

func (n *rabbitVRFDKGTransport) rabbitVRFServingBridgeForBlockV1(
	blockNumber uint64,
) (lqc.RabbitVRFDKGBridgeV1, uint64, bool, error) {
	var empty lqc.RabbitVRFDKGBridgeV1
	if n == nil || n.runtime == nil {
		return empty, 0, false, nil
	}
	runtime := n.runtime
	if runtime.backend == nil || runtime.backend.blockchain == nil || runtime.engine == nil || blockNumber == 0 {
		return empty, 0, false, errRabbitVRFDKGRuntimeV1
	}
	chain := runtime.backend.blockchain
	config := chain.Config()
	if config == nil || config.LQC == nil || config.LQC.EpochLength == 0 {
		return empty, 0, false, errRabbitVRFDKGRuntimeV1
	}
	parent := chain.GetHeaderByNumber(blockNumber - 1)
	if parent == nil {
		return empty, 0, false, lqc.ErrWorkV1EngineLabParentMissing
	}
	bridge, exists, err := runtime.engine.RabbitVRFDKGBridgeForTargetBlockV1(
		chain, blockNumber-1, parent.Hash(), blockNumber, config.LQC.EpochLength,
	)
	return bridge, config.LQC.EpochLength, exists, err
}

// rabbitVRFArtifactsForBridgeV1 joins results with the certificate of the
// serving session. It never obtains that certificate from current preparation.
func (n *rabbitVRFDKGTransport) rabbitVRFArtifactsForBridgeV1(
	blockNumber, epochLength uint64,
	bridge lqc.RabbitVRFDKGBridgeV1,
) (lqc.RabbitVRFKeysetCertificateV1, []lqc.RabbitVRFFinalizationV1, bool, error) {
	var empty lqc.RabbitVRFKeysetCertificateV1
	epoch, err := lqc.RabbitVRFEpochForBlockV1(blockNumber, epochLength)
	if err != nil {
		return empty, nil, false, err
	}
	sessionID, err := lqc.RabbitVRFDKGSessionIDV1(bridge.Session)
	if err != nil || sessionID != bridge.SessionID || bridge.TargetVRFEpoch != bridge.Session.TargetVRFEpoch {
		return empty, nil, false, errRabbitVRFDKGArtifactSessionMismatch
	}
	if epoch != bridge.TargetVRFEpoch {
		return empty, nil, false, nil
	}
	if n == nil || n.runtime == nil {
		return empty, nil, false, nil
	}

	n.mu.Lock()
	candidates := make([]lqc.RabbitVRFFinalizationV1, 0, len(n.finalizations))
	for key, finalization := range n.finalizations {
		// Keep the preceding epoch for shallow reorgs and future prepared results.
		if finalization.Epoch < epoch && epoch-finalization.Epoch > 1 {
			delete(n.finalizations, key)
			continue
		}
		if key.SessionID == bridge.SessionID && finalization.Epoch == epoch {
			candidates = append(candidates, finalization)
		}
	}
	n.mu.Unlock()
	if len(candidates) == 0 {
		return empty, nil, false, nil
	}
	store, err := n.keysetCertificateStoreV1()
	if err != nil {
		return empty, nil, false, err
	}
	// Load verifies member signatures and quorum against the canonical bridge.
	certificate, err := store.Load(bridge.Session, bridge.Members)
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, rabbitvrfstate.ErrDKGTransportKeyStoreMissingV1) {
		return empty, nil, false, nil
	}
	if err != nil {
		return empty, nil, false, err
	}
	pending := make([]lqc.RabbitVRFFinalizationV1, 0, len(candidates))
	for _, finalization := range candidates {
		request, err := n.runtime.canonicalPendingRequestV1(finalization.RequestID)
		if errors.Is(err, errRabbitVRFRequestNotPendingV1) {
			continue
		}
		if err != nil {
			return empty, nil, false, fmt.Errorf("resolve rabbit vrf finalization request %s for block %d: %w", finalization.RequestID, blockNumber, err)
		}
		if request.RequestBlock == 0 || request.RequestBlock >= blockNumber || finalization.Round != request.RequestBlock {
			return empty, nil, false, errors.New("rabbit vrf finalization canonical epoch/round mismatch")
		}
		if err := lqc.ValidateRabbitVRFFinalizationProofV1(bridge.Session, bridge.Members, certificate, finalization); err != nil {
			return empty, nil, false, err
		}
		pending = append(pending, finalization)
	}
	canonical, err := lqc.CanonicalRabbitVRFFinalizationsV1(pending)
	if err != nil {
		return empty, nil, false, err
	}
	if len(canonical) == 0 {
		return empty, nil, false, nil
	}
	if len(canonical) > lqc.MaxRabbitVRFFinalizationsPerBlockV1 {
		canonical = canonical[:lqc.MaxRabbitVRFFinalizationsPerBlockV1]
	}
	return certificate, canonical, true, nil
}
