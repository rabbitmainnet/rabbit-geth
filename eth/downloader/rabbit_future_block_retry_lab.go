package downloader

import (
	"errors"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core/types"
)

// rabbitIsFutureBlock identifies temporary time-based consensus rejection.
// It does not change validation or make any header acceptable earlier.
func rabbitIsFutureBlock(err error) bool {
	return errors.Is(err, consensus.ErrFutureBlock)
}

// rabbitRetryFutureImport only retries the same batch while the local clock
// advances; no consensus rule or timestamp is overridden. Hard deadline and
// node shutdown make the wait finite/cancelable.
func rabbitRetryFutureImport(quit <-chan struct{}, maxWait, interval time.Duration, insert func() (int, error)) (int, error) {
	deadline := time.NewTimer(maxWait)
	defer deadline.Stop()
	for {
		index, err := insert()
		if !rabbitIsFutureBlock(err) {
			return index, err
		}
		pause := time.NewTimer(interval)
		select {
		case <-pause.C:
		case <-deadline.C:
			if !pause.Stop() {
				select {
				case <-pause.C:
				default:
				}
			}
			return index, err
		case <-quit:
			if !pause.Stop() {
				select {
				case <-pause.C:
				default:
				}
			}
			return index, err
		}
	}
}

func (d *Downloader) rabbitInsertChainRetryFuture(blocks types.Blocks) (int, error) {
	// Max 20s: covers short future/slot offsets without waiting indefinitely.
	return rabbitRetryFutureImport(d.quitCh, 20*time.Second, 250*time.Millisecond, func() (int, error) {
		return d.blockchain.InsertChain(blocks)
	})
}

// rabbitClassifyDownloadedImportError preserves the original consensus cause.
// A future block is not an invalid chain: the active sync attempt must stop
// without accusing the serving peer of an invalid block. The recovery owner
// remains responsible for scheduling a later bounded attempt.
func rabbitClassifyDownloadedImportError(err error) error {
	if err == nil {
		return nil
	}
	if rabbitIsFutureBlock(err) {
		return fmt.Errorf("downloaded LQC block not ready yet: %w", err)
	}
	return fmt.Errorf("%w: %v", errInvalidChain, err)
}
