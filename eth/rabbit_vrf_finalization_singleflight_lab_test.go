//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/consensus/lqc"
	"golang.org/x/sync/singleflight"
)

func TestRabbitVRFFinalizationSingleflightSharesOnlyConcurrentScansV2(t *testing.T) {
	var flights singleflight.Group
	var executions atomic.Int32
	ready := make(chan struct{})
	done := make(chan struct{})
	const readers = 16
	var wg sync.WaitGroup
	wg.Add(readers)
	errs := make(chan error, readers)
	for i := 0; i < readers; i++ {
		go func() {
			defer wg.Done()
			<-ready
			_, err := rabbitVRFCoalescedFinalizationScanV2(&flights, "head-A", func() ([]lqc.RabbitVRFFinalizationV1, error) {
				executions.Add(1)
				<-done
				return nil, nil
			})
			errs <- err
		}()
	}
	close(ready)
	// Once the first scan starts, hold it so other peer calls can join it.
	deadline := time.Now().Add(3 * time.Second)
	for executions.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if executions.Load() != 1 {
		close(done)
		wg.Wait()
		t.Fatalf("unexpected concurrent scan count %d", executions.Load())
	}
	time.Sleep(50 * time.Millisecond)
	close(done)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if n := executions.Load(); n != 1 {
		t.Fatalf("expected one BLS scan for concurrent peers; got %d", n)
	}
	// A later call, even on the same head, MUST revalidate (not a stale cache).
	_, err := rabbitVRFCoalescedFinalizationScanV2(&flights, "head-A", func() ([]lqc.RabbitVRFFinalizationV1, error) {
		executions.Add(1)
		return nil, nil
	})
	if err != nil || executions.Load() != 2 {
		t.Fatalf("subsequent call did not rescan: %d %v", executions.Load(), err)
	}
}

func TestRabbitVRFFinalizationSingleflightSeparatesHeadsAndErrorsV2(t *testing.T) {
	var flights singleflight.Group
	count := 0
	want := errors.New("invalid canonical proof")
	_, err := rabbitVRFCoalescedFinalizationScanV2(&flights, "before-reorg", func() ([]lqc.RabbitVRFFinalizationV1, error) { count++; return nil, want })
	if !errors.Is(err, want) {
		t.Fatalf("validation error lost: %v", err)
	}
	_, err = rabbitVRFCoalescedFinalizationScanV2(&flights, "after-reorg", func() ([]lqc.RabbitVRFFinalizationV1, error) { count++; return nil, nil })
	if err != nil || count != 2 {
		t.Fatalf("reorg head must rescan: count=%d error=%v", count, err)
	}
}
