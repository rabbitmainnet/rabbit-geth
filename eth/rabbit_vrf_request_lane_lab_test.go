//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestRabbitVRFRequestLaneIndependentAndSerializedV1(t *testing.T) {
	runtimeStop := make(chan struct{})
	entered := make(chan struct{}, 10)
	release := make(chan struct{})
	var active, overlap int32
	wake, stop := rabbitVRFStartRequestLaneV1(runtimeStop, time.Hour, func() {
		if atomic.AddInt32(&active, 1) != 1 {
			atomic.StoreInt32(&overlap, 1)
		}
		entered <- struct{}{}
		<-release
		atomic.AddInt32(&active, -1)
	})
	// Preparation remains blocked independently of the request worker.
	preparationRelease := make(chan struct{})
	preparationStarted := make(chan struct{})
	preparationDone := make(chan struct{})
	go func() { close(preparationStarted); <-preparationRelease; close(preparationDone) }()
	<-preparationStarted
	wake()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		close(release)
		close(preparationRelease)
		stop()
		t.Fatal("request waited for preparation")
	}
	for i := 0; i < 100; i++ {
		wake()
	}
	stopped := make(chan struct{})
	go func() { stop(); close(stopped) }()
	select {
	case <-stopped:
		t.Fatal("stop returned before in-flight request finished")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("request worker did not stop")
	}
	close(preparationRelease)
	<-preparationDone
	stop() // idempotent
	if atomic.LoadInt32(&overlap) != 0 {
		t.Fatal("overlapping request scans")
	}
}
func TestRabbitVRFRequestLaneTimerAndRuntimeStopV1(t *testing.T) {
	runtimeStop := make(chan struct{})
	ran := make(chan struct{}, 1)
	_, stop := rabbitVRFStartRequestLaneV1(runtimeStop, time.Millisecond, func() {
		select {
		case ran <- struct{}{}:
		default:
		}
	})
	select {
	case <-ran:
	case <-time.After(2 * time.Second):
		close(runtimeStop)
		stop()
		t.Fatal("timer did not scan")
	}
	close(runtimeStop)
	stop()
}
