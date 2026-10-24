//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/p2p"
)

func TestRabbitVRFDKGProtocolV2Descriptors(t *testing.T) {
	transport := mustRabbitVRFDKGTransportV1(t, 9280, 9280, common.Hash{1})
	old := transport.Protocol()
	next := transport.ProtocolV2()
	if old.Name != next.Name || old.Name != "rvrfdkg" {
		t.Fatalf("protocol names differ: %q / %q", old.Name, next.Name)
	}
	if old.Version != 1 || old.Length != 7 {
		t.Fatalf("v1 descriptor changed: %d/%d", old.Version, old.Length)
	}
	if next.Version != 2 || next.Length != 8 {
		t.Fatalf("v2 descriptor invalid: %d/%d", next.Version, next.Length)
	}
}

func TestRabbitVRFDKGHandshakeV2MatchingAndMixedVersions(t *testing.T) {
	genesis := common.Hash{1}
	left := mustRabbitVRFDKGTransportV1(t, 9280, 9280, genesis)
	right := mustRabbitVRFDKGTransportV1(t, 9280, 9280, genesis)
	run := func(a, b uint) [2]error {
		t.Helper()
		arw, brw := p2p.MsgPipe()
		defer arw.Close()
		defer brw.Close()
		results := make(chan error, 2)
		go func() { results <- left.handshakeVersion(&rabbitVRFDKGPeer{rw: arw}, a) }()
		go func() { results <- right.handshakeVersion(&rabbitVRFDKGPeer{rw: brw}, b) }()
		var got [2]error
		for i := range got {
			select {
			case got[i] = <-results:
			case <-time.After(3 * time.Second):
				t.Fatal("v2 handshake hung")
			}
		}
		return got
	}
	for _, err := range run(2, 2) {
		if err != nil {
			t.Fatalf("matching v2 handshake: %v", err)
		}
	}
	for _, err := range run(1, 1) {
		if err != nil {
			t.Fatalf("matching v1 handshake: %v", err)
		}
	}
	for _, err := range run(1, 2) {
		if err == nil {
			t.Fatal("mixed version handshake unexpectedly accepted")
		}
	}
}
