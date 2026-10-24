//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/p2p"
)

func TestRabbitVRFFinalizationDeliveryV2(t *testing.T) {
	sender, receiver := p2p.MsgPipe()
	defer sender.Close()
	defer receiver.Close()

	peer := &rabbitVRFDKGPeer{
		rw:      sender,
		version: rabbitVRFDKGProtocolVersionV2,
	}

	proof := lqc.RabbitVRFFinalizationV1{
		Version:   lqc.RabbitVRFFinalizationVersionV1,
		ProofHash: common.HexToHash("0x1234"),
	}

	sent := make(chan error, 1)
	go func() {
		sent <- peer.sendVerifiedFinalizationV2(proof)
	}()

	received := make(chan error, 1)
	go func() {
		msg, err := receiver.ReadMsg()
		if err != nil {
			received <- err
			return
		}
		defer msg.Discard()

		if msg.Code != rabbitVRFDKGFinalizationMsg {
			received <- fmt.Errorf("wrong message code: %d", msg.Code)
			return
		}

		var decoded lqc.RabbitVRFFinalizationV1
		if err := msg.Decode(&decoded); err != nil {
			received <- err
			return
		}

		if decoded.ProofHash != proof.ProofHash {
			received <- errors.New("proof hash mismatch")
			return
		}
		received <- nil
	}()

	select {
	case err := <-received:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("P2P proof delivery timeout")
	}

	if err := <-sent; err != nil {
		t.Fatal(err)
	}

	// A successfully sent proof must not be sent again to this peer.
	duplicate := make(chan error, 1)
	go func() {
		duplicate <- peer.sendVerifiedFinalizationV2(proof)
	}()

	select {
	case err := <-duplicate:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("duplicate proof send blocked")
	}
}
