package eth

import (
	"testing"
	"time"
)

func TestRabbitQueuedLiveOperationsCancelOnShutdown(t *testing.T) {
	for _, operation := range []string{"import", "recovery"} {
		t.Run(operation, func(t *testing.T) {
			h := (*ethHandler)(&handler{quitSync: make(chan struct{})})
			h.lqcLiveImportMu.Lock()
			defer h.lqcLiveImportMu.Unlock()

			done := make(chan error, 1)
			go func() {
				if operation == "import" {
					_, err := h.insertLQCLiveBlock(nil)
					done <- err
				} else {
					done <- h.startLQCRecoveryAfterLiveImport(nil)
				}
			}()
			select {
			case err := <-done:
				t.Fatalf("operation bypassed occupied lock: %v", err)
			case <-time.After(50 * time.Millisecond):
			}
			close(h.quitSync)
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("shutdown returned success")
				}
			case <-time.After(time.Second):
				t.Fatal("queued operation ignored shutdown")
			}
		})
	}
}
