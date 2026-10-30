package downloader

import (
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core/types"
)

type rabbitFutureErrorChain struct {
	BlockChain
	failure error
}

func (c *rabbitFutureErrorChain) InsertChain(types.Blocks) (int, error) {
	return 0, c.failure
}

func TestRabbitFutureImportErrorClassification(t *testing.T) {
	for _, tc := range []struct {
		name    string
		failure error
		future  bool
	}{
		{"future-block", consensus.ErrFutureBlock, true},
		{"invalid-block", errors.New("invalid test block"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := &Downloader{
				blockchain: &rabbitFutureErrorChain{failure: tc.failure},
				quitCh:     make(chan struct{}),
			}
			err := d.importBlockResults([]*fetchResult{
				{Header: &types.Header{Number: big.NewInt(1)}},
			})
			if tc.future {
				if !errors.Is(err, consensus.ErrFutureBlock) {
					t.Fatalf("FUTURE ERROR IDENTITY LOST: %v", err)
				}
				if errors.Is(err, errInvalidChain) {
					t.Fatalf("TEMPORARY FUTURE BLOCK CLASSIFIED AS INVALID CHAIN: %v", err)
				}
			} else if !errors.Is(err, errInvalidChain) {
				t.Fatalf("invalid block classification lost: %v", err)
			}
		})
	}
}

func TestRabbitFutureImportRetryAndBadBlockReporting(t *testing.T) {
	chain := &rabbitFutureErrorChain{failure: consensus.ErrFutureBlock}
	reports := 0
	d := &Downloader{
		blockchain: chain,
		quitCh:     make(chan struct{}),
		badBlock:   func(*types.Header, *types.Header) { reports++ },
	}
	batch := []*fetchResult{
		{Header: &types.Header{Number: big.NewInt(1)}},
	}

	if err := d.importBlockResults(batch); !errors.Is(err, consensus.ErrFutureBlock) {
		t.Fatalf("temporary rejection lost: %v", err)
	}
	if reports != 0 {
		t.Fatalf("future block reported as permanently invalid: %d", reports)
	}

	// Simulate validation succeeding once the time condition is satisfied.
	chain.failure = nil
	if err := d.importBlockResults(batch); err != nil {
		t.Fatalf("explicit retry failed: %v", err)
	}
	if reports != 0 {
		t.Fatal("retry reported a bad block")
	}
}

type rabbitFutureOnceChain struct {
	BlockChain
	calls int
}

func (c *rabbitFutureOnceChain) InsertChain(types.Blocks) (int, error) {
	c.calls++
	if c.calls == 1 {
		return 0, consensus.ErrFutureBlock
	}
	return 0, nil
}

func TestRabbitFutureImportAutomaticRetry(t *testing.T) {
	chain := &rabbitFutureOnceChain{}
	d := &Downloader{blockchain: chain, quitCh: make(chan struct{})}
	batch := types.Blocks{types.NewBlockWithHeader(&types.Header{Number: big.NewInt(1)})}
	if _, err := d.insertChainWithFutureRetry(batch, time.Second); err != nil {
		t.Fatal(err)
	}
	if chain.calls != 2 {
		t.Fatalf("validation calls=%d, want 2", chain.calls)
	}
}

func TestRabbitFutureImportRetryShutdown(t *testing.T) {
	quit := make(chan struct{})
	close(quit)
	chain := &rabbitFutureOnceChain{}
	d := &Downloader{blockchain: chain, quitCh: quit}
	if _, err := d.insertChainWithFutureRetry(nil, time.Second); !errors.Is(err, errCancelContentProcessing) {
		t.Fatalf("shutdown error=%v", err)
	}
	if chain.calls != 0 {
		t.Fatal("import attempted after shutdown")
	}
}

type rabbitFutureCancelChain struct {
	BlockChain
	cancel chan struct{}
	calls  int
}

func (c *rabbitFutureCancelChain) InsertChain(types.Blocks) (int, error) {
	c.calls++
	if c.calls == 1 {
		close(c.cancel)
	}
	return 0, consensus.ErrFutureBlock
}

func TestRabbitFutureImportCancelDuringWait(t *testing.T) {
	cancel := make(chan struct{})
	chain := &rabbitFutureCancelChain{cancel: cancel}
	d := &Downloader{
		blockchain: chain,
		quitCh:     make(chan struct{}),
		cancelCh:   cancel,
	}
	started := time.Now()
	_, err := d.insertChainWithFutureRetry(nil, 3*time.Second)
	if !errors.Is(err, errCancelContentProcessing) {
		t.Fatalf("cancellation error=%v", err)
	}
	if chain.calls != 1 {
		t.Fatalf("import continued after cancellation: calls=%d", chain.calls)
	}
	if time.Since(started) > time.Second {
		t.Fatal("cancellation waited for the retry deadline")
	}
}

func TestRabbitFutureImportCancellationNotBadChain(t *testing.T) {
	cancel := make(chan struct{})
	close(cancel)
	reports := 0
	d := &Downloader{
		blockchain: &rabbitFutureErrorChain{failure: consensus.ErrFutureBlock},
		quitCh:     make(chan struct{}),
		cancelCh:   cancel,
		badBlock:   func(*types.Header, *types.Header) { reports++ },
	}
	err := d.importBlockResults([]*fetchResult{
		{Header: &types.Header{Number: big.NewInt(1)}},
	})
	if !errors.Is(err, errCancelContentProcessing) {
		t.Fatalf("cancellation identity lost: %v", err)
	}
	if errors.Is(err, errInvalidChain) || reports != 0 {
		t.Fatalf("cancellation classified as bad chain: err=%v reports=%d", err, reports)
	}
}
