//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"errors"

	"github.com/ethereum/go-ethereum/consensus/lqc"
)

// A normal pending->completed transition can race with messages already in
// flight. The original admission path checks shape/session/cryptography BEFORE
// looking up pending state. Only that verified not-pending result is harmless.
// No proof is inserted and no settlement is applied by this late-message path.
func (n *rabbitVRFDKGTransport) acceptFinalizationFromPeerV2(proof lqc.RabbitVRFFinalizationV1, certificate lqc.RabbitVRFKeysetCertificateV1) (bool, error) {
	err := n.acceptVerifiedFinalizationV1(proof, certificate)
	if errors.Is(err, errRabbitVRFRequestNotPendingV1) {
		return false, nil
	}
	return err == nil, err
}
