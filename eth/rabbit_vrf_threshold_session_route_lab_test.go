//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"errors"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
)

func TestRabbitVRFThresholdSessionBeforeRouteV1(t *testing.T) {
	current := common.HexToHash("0x1234")
	transport := &rabbitVRFDKGTransport{
		runtime: &rabbitVRFDKGRuntime{
			current: rabbitVRFDKGLocalContextV1{SessionID: current},
		},
	}
	packet := lqc.RabbitVRFThresholdPartialV1{
		SessionID: common.HexToHash("0x5678"),
		ShareID:   1,
	}
	err := transport.handleInboundThresholdPartialV1(nil, packet)
	if !errors.Is(err, errRabbitVRFDKGArtifactSessionMismatch) {
		t.Fatalf("different session must remain pending: %v", err)
	}

	packet.SessionID = current
	err = transport.handleInboundThresholdPartialV1(nil, packet)
	if err == nil || errors.Is(err, errRabbitVRFDKGArtifactSessionMismatch) {
		t.Fatalf("current session without authenticated route was not rejected: %v", err)
	}
}
