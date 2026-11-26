package eth

import (
	"errors"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"testing"
	"time"
)

func TestRabbitLiveFutureRetryClockCatchesUp(t *testing.T) {
	var parent common.Hash
	attempts := 0
	err := rabbitRetryFutureLiveImport(200*time.Millisecond, time.Millisecond, 11, parent,
		uint64(time.Now().Unix()), func() (uint64, common.Hash) { return 10, parent },
		func() error {
			attempts++
			if attempts == 1 {
				return consensus.ErrFutureBlock
			}
			return nil
		})
	if err != nil || attempts != 2 {
		t.Fatalf("err=%v attempts=%d", err, attempts)
	}
}
func TestRabbitLiveFutureRetryNeverRetriesInvalid(t *testing.T) {
	var parent common.Hash
	bad := errors.New("invalid author")
	calls := 0
	err := rabbitRetryFutureLiveImport(200*time.Millisecond, time.Millisecond, 11, parent,
		uint64(time.Now().Unix()), func() (uint64, common.Hash) { return 10, parent },
		func() error { calls++; return bad })
	if !errors.Is(err, bad) || calls != 1 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}
func TestRabbitLiveFutureRetryParentChangeStops(t *testing.T) {
	var parent common.Hash
	calls := 0
	err := rabbitRetryFutureLiveImport(200*time.Millisecond, time.Millisecond, 11, parent,
		uint64(time.Now().Unix()), func() (uint64, common.Hash) { return 11, parent },
		func() error { calls++; return nil })
	if err != nil || calls != 0 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}
func TestRabbitLiveFutureRetryBounded(t *testing.T) {
	var parent common.Hash
	calls := 0
	err := rabbitRetryFutureLiveImport(15*time.Millisecond, time.Millisecond, 11, parent,
		uint64(time.Now().Unix()), func() (uint64, common.Hash) { return 10, parent },
		func() error { calls++; return consensus.ErrFutureBlock })
	if !errors.Is(err, consensus.ErrFutureBlock) || calls == 0 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}
func TestRabbitLiveFutureRetryNeverAcceptsAheadOfTimestamp(t *testing.T) {
	var parent common.Hash
	calls := 0
	err := rabbitRetryFutureLiveImport(15*time.Millisecond, time.Millisecond, 11, parent,
		uint64(time.Now().Add(10*time.Second).Unix()), func() (uint64, common.Hash) { return 10, parent },
		func() error { calls++; return nil })
	if !errors.Is(err, consensus.ErrFutureBlock) || calls != 0 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}
