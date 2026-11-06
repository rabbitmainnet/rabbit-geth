package eth

import "github.com/ethereum/go-ethereum/common"

// Taking an improved target must not mark the recovery worker stopped.
// A concurrently scheduled still newer target remains queued.
func (h *ethHandler) takeBetterPendingLQCSyncTargetV1(current *lqcSyncTarget) *lqcSyncTarget {
	h.lqcSyncMu.Lock()
	defer h.lqcSyncMu.Unlock()
	if !betterLQCSyncTarget(h.lqcSyncPending, current) {
		return nil
	}
	next := h.lqcSyncPending
	h.lqcSyncPending = nil
	return next
}

func (h *ethHandler) restorePendingLQCSyncTargetV1(target *lqcSyncTarget) {
	h.lqcSyncMu.Lock()
	defer h.lqcSyncMu.Unlock()
	if betterLQCSyncTarget(target, h.lqcSyncPending) {
		h.lqcSyncPending = target
	}
}

func lqcShouldDeferFailedExtensionV1(
	failed bool,
	failedHash, candidateHash common.Hash,
	failedProgress, currentProgress uint64,
) bool {
	return failed &&
		failedHash == candidateHash &&
		failedProgress == currentProgress
}
