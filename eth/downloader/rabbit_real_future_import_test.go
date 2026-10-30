package downloader

import (
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/eth/ethconfig"
)

type rabbitDelayedRealChain struct {
	BlockChain
	readyAt  time.Time
	rejected int
	accepted int
}

func (c *rabbitDelayedRealChain) InsertChain(blocks types.Blocks) (int, error) {
	if time.Now().Before(c.readyAt) {
		c.rejected++
		return 0, consensus.ErrFutureBlock
	}
	c.accepted++
	return c.BlockChain.InsertChain(blocks)
}

func TestRabbitRealBlockImportAfterTemporaryFuture(t *testing.T) {
	tester := newTester(t, ethconfig.FullSync)
	defer tester.terminate()

	chain := &rabbitDelayedRealChain{
		BlockChain: tester.chain,
		readyAt:    time.Now().Add(250 * time.Millisecond),
	}
	reports := 0
	d := &Downloader{
		blockchain: chain,
		quitCh:     make(chan struct{}),
		badBlock:   func(*types.Header, *types.Header) { reports++ },
	}

	block := testChainBase.blocks[1]
	result := &fetchResult{
		Header:       block.Header(),
		Transactions: block.Transactions(),
		Uncles:       block.Uncles(),
		Withdrawals:  block.Withdrawals(),
	}
	if err := d.importBlockResults([]*fetchResult{result}); err != nil {
		t.Fatal(err)
	}
	head := tester.chain.CurrentBlock()
	if head.Hash() != block.Hash() || head.Root != block.Root() {
		t.Fatalf("canonical hash/state mismatch: head=%s want=%s", head.Hash(), block.Hash())
	}
	if chain.rejected == 0 || chain.accepted != 1 || reports != 0 {
		t.Fatalf("rejected=%d accepted=%d badReports=%d",
			chain.rejected, chain.accepted, reports)
	}
	t.Logf("REAL BLOCK IMPORTED: number=%d hash=%s state=%s retries=%d",
		block.NumberU64(), head.Hash(), head.Root, chain.rejected)
}
