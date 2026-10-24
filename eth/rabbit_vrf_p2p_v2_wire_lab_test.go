//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"reflect"
	"testing"

	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/p2p"
)

func TestRabbitVRFP2PV2FinalizationMessageWire(t *testing.T) {
	sender, receiver := p2p.MsgPipe()
	defer sender.Close()
	defer receiver.Close()

	packet := lqc.RabbitVRFFinalizationV1{
		Version: lqc.RabbitVRFFinalizationVersionV1,
	}

	sent := make(chan error, 1)
	go func() {
		sent <- p2p.Send(sender, rabbitVRFDKGFinalizationMsg, packet)
	}()

	msg, err := receiver.ReadMsg()
	if err != nil {
		t.Fatal(err)
	}
	defer msg.Discard()

	if msg.Code != rabbitVRFDKGFinalizationMsg {
		t.Fatalf("message code=%d want=%d",
			msg.Code, rabbitVRFDKGFinalizationMsg)
	}

	var decoded lqc.RabbitVRFFinalizationV1
	if err := msg.Decode(&decoded); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(decoded, packet) {
		t.Fatal("finalization changed during P2P transmission")
	}

	if err := <-sent; err != nil {
		t.Fatal(err)
	}
}
