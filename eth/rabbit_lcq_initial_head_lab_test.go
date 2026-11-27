package eth

import (
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/consensus/ethash"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/eth/ethconfig"
	ethproto "github.com/ethereum/go-ethereum/eth/protocols/eth"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/params"
)

func TestLQCInitialHeadRequestsHeaders(t *testing.T) {
	ahead := newTestHandlerWithBlocks(1, ethconfig.FullSync)
	defer ahead.close()

	cfg := *params.TestChainConfig
	cfg.ChainID = big.NewInt(9280)
	cfg.Ethash = nil
	cfg.LQC = &params.LQCConfig{
		CommitteeMin: 32, CommitteeMax: 128, FallbackSlots: 5,
		EpochLength: 128, TargetBlockTimeMs: 10000,
	}
	db := rawdb.NewMemoryDatabase()
	chain, err := core.NewBlockChain(db, &core.Genesis{
		Config: &cfg,
		Alloc: types.GenesisAlloc{
			testAddr: {Balance: big.NewInt(1000000)},
		},
	}, ethash.NewFaker(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer chain.Stop()

	pool := newTestTxPool()
	h, err := newHandler(&handlerConfig{
		Database: db, Chain: chain, TxPool: pool, BlobPool: pool,
		Network: 1, Sync: ethconfig.FullSync, BloomCache: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	h.Start(10)
	defer h.Stop()

	left, right := p2p.MsgPipe()
	local := ethproto.NewPeer(ethproto.ETH69, p2p.NewPeer(enode.ID{1}, "local", nil), left, pool, pool, &cfg)
	remote := ethproto.NewPeer(ethproto.ETH69, p2p.NewPeer(enode.ID{2}, "remote", nil), right, pool, pool, &cfg)
	defer local.Close()
	defer remote.Close()
	defer left.Close()

	done := make(chan error, 1)
	go func() {
		done <- h.runEthPeer(local, func(*ethproto.Peer) error {
			_, err := left.ReadMsg()
			return err
		})
	}()

	remoteHead := ahead.chain.CurrentBlock()
	if err := remote.Handshake(1, chain, ethproto.BlockRangeUpdatePacket{
		LatestBlock: remoteHead.Number.Uint64(), LatestBlockHash: remoteHead.Hash(),
	}); err != nil {
		t.Fatal(err)
	}

	type readResult struct {
		code uint64
		err  error
	}
	received := make(chan readResult, 1)
	go func() {
		msg, err := right.ReadMsg()
		if err == nil {
			err = msg.Discard()
		}
		received <- readResult{code: msg.Code, err: err}
	}()

	select {
	case result := <-received:
		if result.err != nil {
			t.Fatal(result.err)
		}
		if result.code != ethproto.GetBlockHeadersMsg {
			t.Fatalf("first message after handshake: got %#x, want GetBlockHeaders", result.code)
		}
	case err := <-done:
		t.Fatalf("handler exited before requesting headers: %v", err)
	case <-time.After(time.Second):
		t.Fatal("no GetBlockHeaders request after the initial handshake")
	}
}