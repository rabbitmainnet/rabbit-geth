package eth

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// Re-evaluate the same deterministic choice used when scheduling recovery.
// The local head may advance through direct import while recovery is waiting.
func lqcForkRecoveryTargetNeededV1(local, target *types.Header, canonicalHash common.Hash) bool {
	if target == nil || target.Number == nil {
		return false
	}
	if canonicalHash == target.Hash() {
		return false
	}
	return shouldStartLQCSync(local, target.Number.Uint64(), target.Hash())
}
