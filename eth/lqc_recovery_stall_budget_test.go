package eth

import (
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/core/types"
)

func TestLQCRecoveryStallBudget(t *testing.T) {
	head := func(n uint64) *types.Header { return &types.Header{Number: new(big.Int).SetUint64(n)} }
	cases := []struct {
		name          string
		local, target *types.Header
		want          time.Duration
	}{
		{"same-height fork", head(408071), head(408071), 30 * time.Second},
		{"one missing block", head(408071), head(408072), 30 * time.Second},
		{"two missing blocks", head(408071), head(408073), 30 * time.Second},
		{"larger backfill", head(408071), head(408074), 2 * time.Minute},
		{"nil local", nil, head(4), 2 * time.Minute},
		{"nil target", head(4), nil, 2 * time.Minute},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := lqcRecoveryStallBudget(tc.local, tc.target); got != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		})
	}
}
