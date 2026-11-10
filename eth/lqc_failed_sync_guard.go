package eth

import (
	"github.com/ethereum/go-ethereum/common"
	"time"
)

// lqcFailedSyncRecoveryGuard is used by enableLQCWhenReady itself.
// It allows recovery at the same height only after independent observations
// of the identical canonical head, with all safety predicates true.
type lqcFailedSyncRecoveryGuard struct {
	head  common.Hash
	since time.Time
}

func (g *lqcFailedSyncRecoveryGuard) reset() { *g = lqcFailedSyncRecoveryGuard{} }

func (g *lqcFailedSyncRecoveryGuard) observe(head common.Hash, now time.Time, readable, confirmed, noConflict bool) bool {
	if head == (common.Hash{}) || now.IsZero() || !readable || !confirmed || !noConflict {
		g.reset()
		return false
	}
	if g.head != head || g.since.IsZero() || now.Before(g.since) {
		g.head, g.since = head, now
		return false
	}
	return now.Sub(g.since) >= 3*time.Second
}
