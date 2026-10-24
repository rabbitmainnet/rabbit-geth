//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"errors"
	"io"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/consensus/lqc"
	gethcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/p2p"
)

type rabbitVRFTestWriter struct {
	attempts int
	fail     error
}

func (w *rabbitVRFTestWriter) ReadMsg() (p2p.Msg, error) {
	return p2p.Msg{}, io.EOF
}

func (w *rabbitVRFTestWriter) WriteMsg(msg p2p.Msg) error {
	w.attempts++
	if w.fail != nil {
		return w.fail
	}
	_, err := io.Copy(io.Discard, msg.Payload)
	return err
}

func TestRabbitVRFBroadcastPeerFailureV1(t *testing.T) {
	ctx, err := lqc.NewRabbitVRFDKGSessionContextV1(
		big.NewInt(9280), 11,
		gethcrypto.Keccak256Hash([]byte("committee")), 3,
	)
	if err != nil {
		t.Fatal(err)
	}

	keyset := gethcrypto.Keccak256Hash([]byte("keyset"))
	request := gethcrypto.Keccak256Hash([]byte("request"))

	message, _, err := lqc.RabbitVRFThresholdMessageV1(ctx, keyset, request)
	if err != nil {
		t.Fatal(err)
	}

	share := rabbitVRFTestSecretShareV1(t, 1, 10)
	signed, err := share.SignPartial(message)
	if err != nil {
		t.Fatal(err)
	}

	packet, err := lqc.NewRabbitVRFThresholdPartialV1(
		ctx, keyset, request, signed,
	)
	if err != nil {
		t.Fatal(err)
	}

	failure := errors.New("peer disconnected")
	bad := &rabbitVRFTestWriter{fail: failure}
	good1 := &rabbitVRFTestWriter{}
	good2 := &rabbitVRFTestWriter{}

	transport := &rabbitVRFDKGTransport{
		peers: map[string]*rabbitVRFDKGPeer{
			"bad":   &rabbitVRFDKGPeer{rw: bad},
			"good1": &rabbitVRFDKGPeer{rw: good1},
			"good2": &rabbitVRFDKGPeer{rw: good2},
		},
	}

	err = transport.broadcastThresholdPartialV1(packet, "")
	if !errors.Is(err, failure) {
		t.Fatalf("falha do peer nao reportada: %v", err)
	}
	if bad.attempts != 1 || good1.attempts != 1 || good2.attempts != 1 {
		t.Fatalf("tentativas: bad=%d good1=%d good2=%d",
			bad.attempts, good1.attempts, good2.attempts)
	}

	if err := transport.broadcastThresholdPartialV1(packet, "bad"); err != nil {
		t.Fatal(err)
	}
	if good1.attempts != 1 || good2.attempts != 1 {
		t.Fatal("assinatura duplicada para peer saudavel")
	}
}
