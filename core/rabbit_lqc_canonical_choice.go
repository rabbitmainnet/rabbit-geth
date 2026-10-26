package core

import (
	"bytes"

	"github.com/ethereum/go-ethereum/core/types"
)

// Matches eth.shouldStartLQCSync: taller chain, then smaller hash on a tie.
// Callers hold chainmu and have already validated the candidate block.
func rabbitLQCPreferCanonicalHeaderV1(current, candidate *types.Header) bool {
	if candidate == nil || candidate.Number == nil {
		return false
	}
	if current == nil || current.Number == nil {
		return true
	}
	if cmp := candidate.Number.Cmp(current.Number); cmp != 0 {
		return cmp > 0
	}
	a, b := candidate.Hash(), current.Hash()
	return bytes.Compare(a[:], b[:]) < 0
}
