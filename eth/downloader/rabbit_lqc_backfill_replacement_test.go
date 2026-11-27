package downloader

import (
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/eth/protocols/eth"
)

func TestRabbitBackfillReplacementImportsNewTarget(t *testing.T) {
	lab := newTester(t, FullSync)
	defer lab.terminate()
	d := lab.downloader
	oldChain := testChainForkLightA.shorten(len(testChainBase.blocks) + 80)
	newChain := testChainForkLightB.shorten(len(testChainBase.blocks) + MaxHeaderFetch)
	oldTip := oldChain.blocks[len(oldChain.blocks)-1]
	newTip := newChain.blocks[len(newChain.blocks)-1]

	events := make(chan SyncEvent, 16)
	sub := d.SubscribeSyncEvents(events)
	defer sub.Unsubscribe()

	firstBody := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	var first sync.Once
	d.bodyFetchHook = func(_ []*types.Header) {
		first.Do(func() {
			close(firstBody)
			select {
			case <-d.cancelCh:
			case <-release:
			}
		})
	}

	lab.newPeer("original-eth69", eth.ETH69, oldChain.blocks[1:])
	if err := d.BeaconSync(oldTip.Header(), nil); err != nil {
		t.Fatal(err)
	}
	select {
	case <-firstBody:
	case <-time.After(10 * time.Second):
		t.Fatal("original backfill never reached body retrieval")
	}

	lab.newPeer("replacement-eth69", eth.ETH69, newChain.blocks[1:])
	if err := d.BeaconSync(newTip.Header(), nil); err != nil {
		t.Fatal(err)
	}

	var canceled bool
	deadline := time.NewTimer(20 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case e := <-events:
			if e.Type == SyncFailed {
				if !e.RequestedCancellation() {
					t.Fatalf("unexpected downloader failure: %v", e.Err)
				}
				canceled = true
			}
			if e.Type == SyncCompleted && e.Latest != nil && e.Latest.Hash() == newTip.Hash() {
				if !canceled {
					t.Fatal("replacement did not emit the original cancellation")
				}
				n, hash := newTip.NumberU64(), newTip.Hash()
				if rawdb.ReadCanonicalHash(lab.db, n) != hash || !rawdb.HasBody(lab.db, hash, n) {
					t.Fatal("replacement head is missing canonical block data")
				}
				t.Logf("REPLACEMENT_IMPORTED height=%d hash=%s", n, hash)
				return
			}
		case <-deadline.C:
			t.Fatalf("replacement did not import: head=%v cancellation=%t", lab.chain.CurrentBlock().Number, canceled)
		}
	}
}
