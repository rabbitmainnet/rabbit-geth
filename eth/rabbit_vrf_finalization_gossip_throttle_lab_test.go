//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"io"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/p2p"
)

// The test wire consumes messages but does not require live sockets.
type rabbitVRFFinalizationCountWireV2 struct{ writes int }

func (w *rabbitVRFFinalizationCountWireV2) ReadMsg() (p2p.Msg, error) { return p2p.Msg{}, io.EOF }
func (w *rabbitVRFFinalizationCountWireV2) WriteMsg(msg p2p.Msg) error {
	_, err := io.Copy(io.Discard, msg.Payload)
	if err == nil {
		w.writes++
	}
	return err
}

func TestRabbitVRFFinalizationGossipReplayIntervalBoundV2(t *testing.T) {
	if rabbitVRFFinalizationReplayIntervalV2 < 20*time.Second || rabbitVRFFinalizationReplayIntervalV2 > time.Minute {
		t.Fatalf("unexpected replay interval: %v", rabbitVRFFinalizationReplayIntervalV2)
	}
}

func TestRabbitVRFFinalizationGossipDedupAndRepairV2(t *testing.T) {
	rw := &rabbitVRFFinalizationCountWireV2{}
	peer := &rabbitVRFDKGPeer{rw: rw, version: rabbitVRFDKGProtocolVersionV2}
	proof := lqc.RabbitVRFFinalizationV1{ProofHash: common.HexToHash("0x1234")}
	if err := peer.sendVerifiedFinalizationV2(proof, false); err != nil {
		t.Fatal(err)
	}
	if err := peer.sendVerifiedFinalizationV2(proof, false); err != nil {
		t.Fatal(err)
	}
	if rw.writes != 1 {
		t.Fatalf("duplicate on immediate wake: writes=%d", rw.writes)
	}
	if err := peer.sendVerifiedFinalizationV2(proof, true); err != nil {
		t.Fatal(err)
	}
	if rw.writes != 2 {
		t.Fatalf("periodic repair must replay: writes=%d", rw.writes)
	}
}
