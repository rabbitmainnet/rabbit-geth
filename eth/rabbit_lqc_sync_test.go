package eth

import (
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/core/types"
)

func TestLiveLQCHeadRequiresRecentNonGenesisBlock(t *testing.T) {
	now := time.Unix(2_000_000, 0)
	tests := []struct {
		name   string
		header *types.Header
		want   bool
	}{
		{name: "nil", header: nil, want: false},
		{name: "genesis", header: &types.Header{Number: new(big.Int), Time: uint64(now.Unix())}, want: false},
		{name: "recent", header: &types.Header{Number: big.NewInt(1), Time: uint64(now.Add(-time.Minute).Unix())}, want: true},
		{name: "stale", header: &types.Header{Number: big.NewInt(1), Time: uint64(now.Add(-lqcHeadFreshness - time.Second).Unix())}, want: false},
		{name: "small future skew", header: &types.Header{Number: big.NewInt(1), Time: uint64(now.Add(lqcHeadFutureTolerance).Unix())}, want: true},
		{name: "large future skew", header: &types.Header{Number: big.NewInt(1), Time: uint64(now.Add(lqcHeadFutureTolerance + time.Second).Unix())}, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isLiveLQCHead(test.header, now); got != test.want {
				t.Fatalf("got %t want %t", got, test.want)
			}
		})
	}
}
