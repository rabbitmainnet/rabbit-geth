package eth

import (
	"errors"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core/types"
)

// Rabbit LCQ LAB: a bounded asynchronous recheck of a one-block-ahead
// announcement that was rejected as temporarily future. A retry always goes
// through the ORIGINAL InsertChain validation: no timestamp tolerance, author
// priority, queue slots, or fork-choice rules are changed.
const (
	rabbitLiveFutureMaxPending = 8
	rabbitLiveFutureMaxWait    = 30 * time.Second
	rabbitLiveFutureInterval   = time.Second
)

var rabbitLiveFuturePending = struct {
	sync.Mutex
	hashes map[common.Hash]struct{}
}{hashes: make(map[common.Hash]struct{})}

func rabbitQueueFutureLiveBlock(h *ethHandler, block *types.Block, err error) bool {
	if !errors.Is(err, consensus.ErrFutureBlock) || block == nil {
		return false
	}
	// A malicious peer cannot queue blocks arbitrarily far in the future.
	if block.Time() > uint64(time.Now().Add(rabbitLiveFutureMaxWait).Unix()) {
		return false
	}
	hash := block.Hash()
	rabbitLiveFuturePending.Lock()
	if _, found := rabbitLiveFuturePending.hashes[hash]; found {
		rabbitLiveFuturePending.Unlock()
		return true // same retry already scheduled
	}
	if len(rabbitLiveFuturePending.hashes) >= rabbitLiveFutureMaxPending {
		rabbitLiveFuturePending.Unlock()
		return false
	}
	rabbitLiveFuturePending.hashes[hash] = struct{}{}
	rabbitLiveFuturePending.Unlock()

	go func() {
		defer func() {
			rabbitLiveFuturePending.Lock()
			delete(rabbitLiveFuturePending.hashes, hash)
			rabbitLiveFuturePending.Unlock()
		}()
		rabbitRetryFutureLiveImport(
			rabbitLiveFutureMaxWait, rabbitLiveFutureInterval,
			block.NumberU64(), block.ParentHash(), block.Time(),
			func() (uint64, common.Hash) {
				head := h.chain.CurrentBlock()
				if head == nil || head.Number == nil {
					return 0, common.Hash{}
				}
				return head.Number.Uint64(), head.Hash()
			},
			func() error {
				_, insertErr := h.chain.InsertChain(types.Blocks{block})
				return insertErr
			},
		)
	}()
	return true
}

// Returns nil for a successful import OR a stale block whose parent is no
// longer canonical. Returns error for actual failed retry/expiry. No direct
// writes to chain state are allowed other than regular InsertChain.
func rabbitRetryFutureLiveImport(maxWait, interval time.Duration, number uint64,
	parent common.Hash, blockTime uint64,
	current func() (uint64, common.Hash), insert func() error) error {
	deadline := time.NewTimer(maxWait)
	defer deadline.Stop()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-deadline.C:
			return consensus.ErrFutureBlock
		case <-ticker.C:
			height, headHash := current()
			if height+1 != number || headHash != parent {
				return nil // canonical parent changed; never force a stale block
			}
			// Never attempt early insertion just because a timer fired.
			// Additional LCQ slot/fallback checks remain in VerifyHeaders.
			if time.Now().Unix() < int64(blockTime) {
				continue
			}
			err := insert()
			if err == nil {
				return nil
			}
			if !errors.Is(err, consensus.ErrFutureBlock) {
				return err
			}
		}
	}
}
