//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"sync"
	"time"
)

// One owner serializes all scans, including timer and head-triggered scans.
// Stop waits for an in-flight scan before runtime.done can be closed.
func rabbitVRFStartRequestLaneV1(runtimeStop <-chan struct{}, interval time.Duration, process func()) (func(), func()) {
	wake := make(chan struct{}, 1)
	stop := make(chan struct{})
	done := make(chan struct{})
	var once sync.Once
	notify := func() {
		select {
		case wake <- struct{}{}:
		default:
		}
	}
	go func() {
		defer close(done)
		timer := time.NewTicker(interval)
		defer timer.Stop()
		for {
			select {
			case <-runtimeStop:
				return
			case <-stop:
				return
			default:
			}
			select {
			case <-runtimeStop:
				return
			case <-stop:
				return
			case <-wake:
			case <-timer.C:
			}
			select {
			case <-runtimeStop:
				return
			case <-stop:
				return
			default:
			}
			process()
		}
	}()
	return notify, func() { once.Do(func() { close(stop) }); <-done }
}
