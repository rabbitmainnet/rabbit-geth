//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import "testing"

func TestRabbitVRFFinalizationWakeBroadcastV2(t *testing.T) {
	transport := &rabbitVRFDKGTransport{
		finalizationWakeV2: make(chan struct{}),
	}

	peerA := transport.finalizationWakeSignalV2()
	peerB := transport.finalizationWakeSignalV2()

	transport.mu.Lock()
	close(transport.finalizationWakeV2)
	transport.finalizationWakeV2 = make(chan struct{})
	transport.mu.Unlock()

	for _, wake := range []<-chan struct{}{peerA, peerB} {
		select {
		case <-wake:
		default:
			t.Fatal("connected peer did not receive notification")
		}
	}

	next := transport.finalizationWakeSignalV2()
	select {
	case <-next:
		t.Fatal("next notification channel already closed")
	default:
	}
}
