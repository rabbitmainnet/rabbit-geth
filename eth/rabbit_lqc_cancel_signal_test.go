package eth

import (
	"errors"
	"testing"

	"github.com/ethereum/go-ethereum/eth/downloader"
)

// Match the downloader's private cancellation sentinel without exporting it.
type requestedBackfillCancellation struct{}

func (requestedBackfillCancellation) Error() string { return "test backfill cancellation" }
func (requestedBackfillCancellation) Is(target error) bool {
	return target.Error() == "syncing canceled (requested)"
}

func TestLQCRecoveryRetryAfterSyncEvent(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want int
	}{
		{"requested cancellation keeps watching", requestedBackfillCancellation{}, 0},
		{"download failure triggers retry", errors.New("download failed"), 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			events := make(chan downloader.SyncEvent, 1)
			events <- downloader.SyncEvent{Type: downloader.SyncFailed, Err: tc.err}
			if got := lqcWaitRecoverySignal(nil, nil, events, nil, nil); got != tc.want {
				t.Fatalf("recovery action = %d, want %d", got, tc.want)
			}
		})
	}
}
