//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
)

func TestRabbitVRFInboundFinalizationRejectsUnverifiedV1(t *testing.T) {
	n := &rabbitVRFDKGTransport{
		runtime:       &rabbitVRFDKGRuntime{},
		finalizations: make(map[rabbitVRFFinalizationKeyV1]lqc.RabbitVRFFinalizationV1),
	}
	proof := lqc.RabbitVRFFinalizationV1{RequestID: common.Hash{1}}
	if err := n.acceptVerifiedFinalizationV1(proof, lqc.RabbitVRFKeysetCertificateV1{}); err == nil {
		t.Fatal("accepted invalid finalization")
	}
	if len(n.finalizations) != 0 {
		t.Fatal("invalid proof reached the producer cache")
	}
}

func TestRabbitVRFInboundFinalizationRejectsUnavailableRuntimeV1(t *testing.T) {
	n := &rabbitVRFDKGTransport{}
	if err := n.acceptVerifiedFinalizationV1(lqc.RabbitVRFFinalizationV1{}, lqc.RabbitVRFKeysetCertificateV1{}); err == nil {
		t.Fatal("accepted finalization without canonical runtime")
	}
}
