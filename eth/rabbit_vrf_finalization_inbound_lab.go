//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
)

// acceptVerifiedFinalizationV1 is the admission boundary for finalizations
// received from a future, separately negotiated VRF P2P extension.
// The caller must provide the canonical session certificate; this function
// does not accept a certificate supplied by the untrusted peer.
func (n *rabbitVRFDKGTransport) acceptVerifiedFinalizationV1(
	proof lqc.RabbitVRFFinalizationV1,
	certificate lqc.RabbitVRFKeysetCertificateV1,
) error {
	if n == nil || n.runtime == nil {
		return errors.New("rabbit vrf finalization receiver unavailable")
	}
	if err := proof.Validate(); err != nil {
		return fmt.Errorf("rabbit vrf finalization shape: %w", err)
	}
	ctx := n.runtime.thresholdContextV1()
	if ctx.SessionID == (common.Hash{}) || ctx.CanonicalSession.TargetVRFEpoch == 0 ||
		proof.Epoch != ctx.CanonicalSession.TargetVRFEpoch {
		return errors.New("rabbit vrf finalization inactive canonical session")
	}
	if err := rabbitVRFValidateFinalizationMemoV1(
		ctx.CanonicalSession, ctx.CanonicalMembers, certificate, proof,
	); err != nil {
		return fmt.Errorf("rabbit vrf finalization cryptographic verification: %w", err)
	}
	request, err := n.runtime.canonicalPendingRequestV1(proof.RequestID)
	if err != nil {
		return fmt.Errorf("rabbit vrf finalization canonical request: %w", err)
	}
	if request.RequestBlock == 0 || request.RequestBlock != proof.Round {
		return errors.New("rabbit vrf finalization canonical round mismatch")
	}
	// Prevent admitting a proof across a local session transition.
	if n.runtime.thresholdContextV1().SessionID != ctx.SessionID {
		return errors.New("rabbit vrf finalization session changed")
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.closed {
		return errRabbitVRFDKGProtocolClosed
	}
	if existing, ok := n.finalizations[rabbitVRFFinalizationKeyV1{SessionID: ctx.SessionID, RequestID: proof.RequestID}]; ok {
		if existing.Epoch == proof.Epoch && existing != proof {
			return errors.New("rabbit vrf finalization proof conflict")
		}
		if existing.Epoch == proof.Epoch {
			return nil
		}
	}
	if n.finalizations == nil {
		n.finalizations = make(map[rabbitVRFFinalizationKeyV1]lqc.RabbitVRFFinalizationV1)
	}
	n.finalizations[rabbitVRFFinalizationKeyV1{SessionID: ctx.SessionID, RequestID: proof.RequestID}] = proof
	return nil
}
