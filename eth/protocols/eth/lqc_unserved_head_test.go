package eth

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestLQCUnservedHeadProductionRange(t *testing.T) {
	peer := new(Peer)
	first := &BlockRangeUpdatePacket{LatestBlock: 100, LatestBlockHash: common.Hash{1}}
	peer.lastRange.Store(first)
	if got := peer.LQCProductionBlockRange(); got != first {
		t.Fatal("the initial announcement should be visible to the production gate")
	}

	peer.MarkLQCHeaderLookupFailed(first)
	if got := peer.LQCProductionBlockRange(); got != nil {
		t.Fatal("the failed announcement should be excluded from the production gate")
	}
	if got := peer.BlockRange(); got != first {
		t.Fatal("the downloader should retain the original announcement")
	}

	updated := *first
	peer.lastRange.Store(&updated)
	peer.MarkLQCHeaderLookupFailed(first)
	if got := peer.LQCProductionBlockRange(); got != &updated {
		t.Fatal("a late failure must not suppress a fresh announcement")
	}
}
