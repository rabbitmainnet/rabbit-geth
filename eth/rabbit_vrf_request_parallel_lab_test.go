//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"errors"
	"math/big"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

func TestRabbitVRFRequestParallelBoundedThreeAndQueueOwnership(t *testing.T) {
	q := rabbitVRFRequestRetryQueueV1{}
	session := common.HexToHash("0xaa")
	for i := 1; i <= 9; i++ {
		q.trackV1(common.BigToHash(big.NewInt(int64(i))), 100, common.HexToHash("0xbb"))
	}
	started := make(chan struct{}, 9)
	release := make(chan struct{})
	done := make(chan error, 1)
	var active, maximum, calls atomic.Int32
	go func() {
		done <- q.runConcurrentV1(time.Unix(1000, 0), session, func(_ common.Hash, _ rabbitVRFRequestRetryOriginV1) error {
			current := active.Add(1)
			defer active.Add(-1)
			calls.Add(1)
			for {
				old := maximum.Load()
				if current <= old || maximum.CompareAndSwap(old, current) {
					break
				}
			}
			started <- struct{}{}
			<-release
			return nil
		})
	}()
	for i := 0; i < 3; i++ {
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			close(release)
			<-done
			t.Fatal("three independent requests did not start together")
		}
	}
	select {
	case <-started:
		close(release)
		<-done
		t.Fatal("more than three workers")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if maximum.Load() != 3 || calls.Load() != 9 {
		t.Fatalf("maximum=%d calls=%d", maximum.Load(), calls.Load())
	}
	for _, origin := range q.Pending {
		if origin.StartedSession != session {
			t.Fatal("successful request lost")
		}
	}
}
func TestRabbitVRFRequestParallelRetrySessionAndReorg(t *testing.T) {
	q := rabbitVRFRequestRetryQueueV1{}
	id := common.HexToHash("0x01")
	session := common.HexToHash("0x11")
	now := time.Unix(1000, 0)
	q.trackV1(id, 100, common.HexToHash("0xaa"))
	var calls atomic.Int32
	process := func(common.Hash, rabbitVRFRequestRetryOriginV1) error { calls.Add(1); return errors.New("temporary") }
	q.runConcurrentV1(now, session, process)
	q.runConcurrentV1(now.Add(time.Second), session, process)
	if calls.Load() != 1 {
		t.Fatal("retry deadline ignored")
	}
	q.runConcurrentV1(now.Add(time.Second), common.HexToHash("0x12"), process)
	if calls.Load() != 2 {
		t.Fatal("session change did not retry")
	}
	origin := q.Pending[id]
	err := q.runConcurrentV1(now.Add(32*time.Second), session, func(common.Hash, rabbitVRFRequestRetryOriginV1) error { return errRabbitVRFRequestWorkerReorgV1 })
	if !errors.Is(err, errRabbitVRFRequestWorkerReorgV1) || q.Pending[id] != origin {
		t.Fatal("reorg committed partial queue progress")
	}
	q.runConcurrentV1(now.Add(33*time.Second), common.HexToHash("0x13"), func(common.Hash, rabbitVRFRequestRetryOriginV1) error { return errRabbitVRFRequestNotPendingV1 })
	if len(q.Pending) != 0 {
		t.Fatal("completed request retained")
	}
}
