package eth

import (
	"errors"
	"time"

	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core/types"
)

func (h *ethHandler) startLQCRecoveryAfterLiveImport(header *types.Header) error {
	if !h.lockLQCLiveImport() {
		return errors.New("LQC recovery canceled during shutdown")
	}
	defer h.lqcLiveImportMu.Unlock()

	select {
	case <-h.quitSync:
		return errors.New("LQC recovery canceled during shutdown")
	default:
	}
	if !shouldStartLQCSync(h.chain.CurrentBlock(), header.Number.Uint64(), header.Hash()) {
		return nil
	}
	return h.downloader.BeaconSync(header, nil)
}

func (h *ethHandler) insertLQCLiveBlock(block *types.Block) (int, error) {
	if !h.lockLQCLiveImport() {
		return 0, errors.New("LQC live import canceled during shutdown")
	}
	defer h.lqcLiveImportMu.Unlock()

	deadline := time.Now().Add(3 * time.Second)
	for {
		select {
		case <-h.quitSync:
			return 0, errors.New("LQC live import canceled during shutdown")
		default:
		}
		index, err := h.chain.InsertChain(types.Blocks{block})
		if !errors.Is(err, consensus.ErrFutureBlock) {
			return index, err
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return index, err
		}
		delay := 100 * time.Millisecond
		if remaining < delay {
			delay = remaining
		}
		timer := time.NewTimer(delay)
		select {
		case <-h.quitSync:
			timer.Stop()
			return index, errors.New("LQC live import canceled during shutdown")
		case <-timer.C:
		}
	}
}

// lockLQCLiveImport allows shutdown to cancel a queued lock acquisition.
func (h *ethHandler) lockLQCLiveImport() bool {
	for {
		select {
		case <-h.quitSync:
			return false
		default:
		}
		if h.lqcLiveImportMu.TryLock() {
			return true
		}
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-h.quitSync:
			timer.Stop()
			return false
		case <-timer.C:
		}
	}
}
