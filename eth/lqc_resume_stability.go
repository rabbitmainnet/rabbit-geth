package eth

import (
	"time"

	"github.com/ethereum/go-ethereum/common"
)

// lqcResumeStabilityGuard requires independent, spaced observations of the
// same canonical head before allowing local block production. It is a local
// availability gate only: it does not change consensus or block validity.
type lqcResumeStabilityGuard struct {
	hash      common.Hash
	firstSeen time.Time
}

func (g *lqcResumeStabilityGuard) reset() {
	*g = lqcResumeStabilityGuard{}
}

func (g *lqcResumeStabilityGuard) allow(hash common.Hash, now time.Time) bool {
	if hash == (common.Hash{}) || now.IsZero() {
		g.reset()
		return false
	}
	if g.hash != hash || g.firstSeen.IsZero() || now.Before(g.firstSeen) {
		g.hash = hash
		g.firstSeen = now
		return false
	}
	return now.Sub(g.firstSeen) >= time.Second
}
