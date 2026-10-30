package downloader

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/eth/ethconfig"
	ethproto "github.com/ethereum/go-ethereum/eth/protocols/eth"
)

type rabbitFutureBeaconChain struct {
	BlockChain
	attempts atomic.Int32
}

func (c *rabbitFutureBeaconChain) InsertChain(blocks types.Blocks) (int, error) {
	if c.attempts.Add(1) <= 3 {
		return 0, consensus.ErrFutureBlock
	}
	return c.BlockChain.InsertChain(blocks)
}

func TestRabbitBeaconSyncTemporaryFutureKeepsPeer(t *testing.T) {
	success := make(chan struct{})
	var once sync.Once
	tester := newTesterWithNotification(t, ethconfig.FullSync, func() {
		once.Do(func() { close(success) })
	})
	defer tester.terminate()

	chain := testChainBase.shorten(blockCacheMaxItems - 15)
	delayed := &rabbitFutureBeaconChain{BlockChain: tester.chain}
	tester.downloader.blockchain = delayed
	tester.newPeer("future-peer", ethproto.ETH69, chain.blocks[1:])

	target := chain.blocks[len(chain.blocks)-1]
	if err := tester.downloader.BeaconSync(target.Header(), nil); err != nil {
		t.Fatal(err)
	}
	select {
	case <-success:
	case <-time.After(30 * time.Second):
		t.Fatal("BeaconSync did not complete after temporary future errors")
	}
	head := tester.chain.CurrentBlock()
	if head.Hash() != target.Hash() || head.Root != target.Root() {
		t.Fatal("canonical hash/state differs from target")
	}
	if delayed.attempts.Load() < 4 {
		t.Fatal("future-error injection was not exercised")
	}
	tester.lock.RLock()
	_, kept := tester.peers["future-peer"]
	tester.lock.RUnlock()
	if !kept {
		t.Fatal("peer removed during temporary future-block recovery")
	}
	t.Logf("BEACONSYNC COMPLETE: height=%d hash=%s state=%s attempts=%d peerKept=%v",
		head.Number.Uint64(), head.Hash(), head.Root, delayed.attempts.Load(), kept)
}
