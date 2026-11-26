//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	"golang.org/x/sync/singleflight"
)

func TestRabbitVRFFinalizationWakeIsolatesSameHeadConcurrentScansV2(t *testing.T) {
	var group singleflight.Group
	var scans atomic.Int32
	head := common.HexToHash("0x42")
	oldWake := make(chan struct{})
	newWake := make(chan struct{})
	oldKey := rabbitVRFFinalizationScanKeyV2(head, oldWake)
	newKey := rabbitVRFFinalizationScanKeyV2(head, newWake)
	if oldKey == newKey {
		t.Fatal("different wake generations share the same scan key")
	}
	if rabbitVRFFinalizationScanKeyV2(common.HexToHash("0x43"), oldWake) == oldKey {
		t.Fatal("reorg shares scan key")
	}
	started := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan error, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, err := rabbitVRFCoalescedFinalizationScanV2(&group, oldKey, func() ([]lqc.RabbitVRFFinalizationV1, error) {
			scans.Add(1)
			close(started)
			<-release
			return nil, nil
		})
		finished <- err
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		close(release)
		wg.Wait()
		t.Fatal("old scan never started")
	}
	// A new proof changes the wake generation while the old scan is blocked.
	// The second call MUST NOT coalesce with the old result.
	newDone := make(chan error, 1)
	go func() {
		_, err := rabbitVRFCoalescedFinalizationScanV2(&group, newKey, func() ([]lqc.RabbitVRFFinalizationV1, error) {
			scans.Add(1)
			return nil, nil
		})
		newDone <- err
	}()
	select {
	case err := <-newDone:
		if err != nil {
			t.Errorf("new scan: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("new wake joined blocked old scan")
	}
	close(release)
	wg.Wait()
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	if n := scans.Load(); n != 2 {
		t.Fatalf("expected distinct scans, got %d", n)
	}
}
