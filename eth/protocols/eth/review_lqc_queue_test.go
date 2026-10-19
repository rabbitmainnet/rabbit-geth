package eth

import (
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/params"
)

type reviewLQCWriter struct {
	p2p.MsgReadWriter
	entered chan struct{}
}

func (w *reviewLQCWriter) WriteMsg(msg p2p.Msg) error {
	select {
	case w.entered <- struct{}{}:
	default:
	}
	return w.MsgReadWriter.WriteMsg(msg)
}

func reviewLQCPeer(t *testing.T, id byte) (*Peer, *p2p.MsgPipeRW, chan struct{}) {
	t.Helper()
	local, remote := p2p.MsgPipe()
	entered := make(chan struct{}, 32)
	native := p2p.NewPeerPipe(enode.ID{id}, "review", nil, local)
	peer := NewPeer(69, native,
		&reviewLQCWriter{local, entered}, nil, nil, &params.ChainConfig{})
	t.Cleanup(func() {
		peer.Close()
		native.Disconnect(p2p.DiscQuitting)
		remote.Close()
		select {
		case <-peer.lqcBroadcastDone:
		case <-time.After(time.Second):
			t.Error("worker de envio nao encerrou")
		}
	})
	return peer, remote, entered
}

func reviewLQCBlock() *types.Block {
	return types.NewBlockWithHeader(&types.Header{
		Number: big.NewInt(1), Difficulty: big.NewInt(0),
	})
}

func TestReviewLQCQueueIsolatesSlowPeer(t *testing.T) {
	slow, _, entered := reviewLQCPeer(t, 1)
	fast, remote, _ := reviewLQCPeer(t, 2)
	block := reviewLQCBlock()

	if !slow.AsyncSendLQCNewBlock(block, big.NewInt(1)) {
		t.Fatal("primeiro envio recusado")
	}
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("peer lento nao iniciou escrita")
	}

	received := make(chan error, 1)
	go func() {
		msg, err := remote.ReadMsg()
		if err == nil {
			err = msg.Discard()
		}
		received <- err
	}()
	if !fast.AsyncSendLQCNewBlock(block, big.NewInt(1)) {
		t.Fatal("envio ao peer saudavel recusado")
	}
	select {
	case err := <-received:
		if err != nil {
			t.Fatal(err)
		}
		t.Log("CORRIGIDO: peer saudavel recebeu com o outro ainda bloqueado.")
	case <-time.After(time.Second):
		t.Fatal("peer lento bloqueou o saudavel")
	}
}

func TestReviewLQCQueuePreservesRangeThenBlock(t *testing.T) {
	peer, remote, _ := reviewLQCPeer(t, 3)
	received := make(chan []uint64, 1)
	go func() {
		codes := []uint64{}
		for i := 0; i < 2; i++ {
			msg, err := remote.ReadMsg()
			if err != nil {
				received <- codes
				return
			}
			codes = append(codes, msg.Code)
			msg.Discard()
		}
		received <- codes
	}()
	peer.AsyncSendLQCBlockRange(BlockRangeUpdatePacket{LatestBlock: 1})
	peer.AsyncSendLQCNewBlock(reviewLQCBlock(), big.NewInt(1))
	select {
	case codes := <-received:
		if len(codes) != 2 ||
			codes[0] != BlockRangeUpdateMsg || codes[1] != NewBlockMsg {
			t.Fatalf("ordem inesperada: %v", codes)
		}
	case <-time.After(time.Second):
		t.Fatal("mensagens nao recebidas")
	}
}

func TestReviewLQCQueueBoundsAndDisconnects(t *testing.T) {
	peer, _, entered := reviewLQCPeer(t, 4)
	block := reviewLQCBlock()
	peer.AsyncSendLQCNewBlock(block, big.NewInt(1))
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("escrita nao iniciou")
	}
	for i := 0; i < cap(peer.lqcBroadcastQueue); i++ {
		if !peer.AsyncSendLQCNewBlock(block, big.NewInt(1)) {
			t.Fatalf("fila recusou item %d antes do limite", i)
		}
	}
	if peer.AsyncSendLQCNewBlock(block, big.NewInt(1)) {
		t.Fatal("fila ultrapassou limite")
	}
	select {
	case <-peer.Done():
	case <-time.After(time.Second):
		t.Fatal("peer saturado nao foi desconectado")
	}
	select {
	case <-peer.lqcBroadcastDone:
	case <-time.After(time.Second):
		t.Fatal("worker bloqueado nao encerrou apos desconexao")
	}
	if peer.AsyncSendLQCNewBlock(block, big.NewInt(1)) {
		t.Fatal("aceitou envio apos desconexao")
	}
}
