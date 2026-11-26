package downloader

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/consensus"
)

func TestRabbitFutureRetrySucceedsWhenClockCatchesUp(t *testing.T) {
	tries := 0
	got, err := rabbitRetryFutureImport(make(chan struct{}), time.Second, time.Millisecond, func() (int, error) {
		tries++
		if tries < 3 {
			return 0, consensus.ErrFutureBlock
		}
		return 2, nil
	})
	if err != nil || got != 2 || tries != 3 {
		t.Fatalf("got index=%d err=%v tries=%d", got, err, tries)
	}
}

func TestRabbitFutureRetryDoesNotRetryInvalidBlocks(t *testing.T) {
	tries := 0
	bad := errors.New("unauthorized producer")
	_, err := rabbitRetryFutureImport(make(chan struct{}), time.Second, time.Millisecond, func() (int, error) { tries++; return 0, bad })
	if !errors.Is(err, bad) || tries != 1 {
		t.Fatalf("err=%v tries=%d", err, tries)
	}
}

func TestRabbitFutureRetryBounded(t *testing.T) {
	tries := 0
	_, err := rabbitRetryFutureImport(make(chan struct{}), 6*time.Millisecond, time.Millisecond, func() (int, error) { tries++; return 0, consensus.ErrFutureBlock })
	if !errors.Is(err, consensus.ErrFutureBlock) || tries < 1 {
		t.Fatalf("err=%v tries=%d", err, tries)
	}
}

func TestRabbitFutureRetryShutdown(t *testing.T) {
	quit := make(chan struct{})
	close(quit)
	tries := 0
	_, err := rabbitRetryFutureImport(quit, time.Second, time.Millisecond, func() (int, error) { tries++; return 0, consensus.ErrFutureBlock })
	if !errors.Is(err, consensus.ErrFutureBlock) || tries != 1 {
		t.Fatalf("err=%v tries=%d", err, tries)
	}
}

func TestRabbitDownloadedFutureIsNotInvalidChain(t *testing.T) {
	err := rabbitClassifyDownloadedImportError(consensus.ErrFutureBlock)
	if !errors.Is(err, consensus.ErrFutureBlock) {
		t.Fatalf("future consensus cause lost: %v", err)
	}
	if errors.Is(err, errInvalidChain) {
		t.Fatalf("temporary future block mislabeled invalid chain: %v", err)
	}
}

func TestRabbitDownloadedInvalidRemainsInvalidChain(t *testing.T) {
	bad := errors.New("unauthorized producer")
	err := rabbitClassifyDownloadedImportError(bad)
	if !errors.Is(err, errInvalidChain) {
		t.Fatalf("invalid block no longer terminates sync: %v", err)
	}
	if err == nil || !strings.Contains(err.Error(), bad.Error()) {
		t.Fatalf("invalid original cause missing: %v", err)
	}
}

func TestRabbitDownloadedNilFailure(t *testing.T) {
	if err := rabbitClassifyDownloadedImportError(nil); err != nil {
		t.Fatalf("nil became error: %v", err)
	}
}
