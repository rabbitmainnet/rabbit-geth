//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"sync"
	"testing"
	"time"
)

func TestRabbitVRFFinalizationWakeConcurrentV2(t *testing.T) {
	n := &rabbitVRFDKGTransport{
		finalizationWakeV2: make(chan struct{}),
	}

	const peers = 16
	var wg sync.WaitGroup
	wg.Add(peers)

	for i := 0; i < peers; i++ {
		go func() {
			defer wg.Done()
			wake := n.finalizationWakeSignalV2()
			select {
			case <-wake:
			case <-time.After(time.Second):
				t.Error("VRF finalization notification timed out")
			}
		}()
	}

	time.Sleep(20 * time.Millisecond)

	n.mu.Lock()
	close(n.finalizationWakeV2)
	n.finalizationWakeV2 = make(chan struct{})
	n.mu.Unlock()

	wg.Wait()
}
