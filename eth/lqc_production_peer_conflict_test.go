package eth

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestLQCProductionPeerChainConflict(t *testing.T) {
	head := common.HexToHash("0x11")
	ancestor := common.HexToHash("0x22")
	fork := common.HexToHash("0x33")
	cases := []struct {
		name      string
		number    uint64
		hash      common.Hash
		canonical common.Hash
		want      bool
	}{
		{"same-head", 100, head, head, false},
		{"same-height-losing-fork", 100, fork, head, false},
		{"same-height-winning-fork", 100, common.HexToHash("0x01"), head, true},
		{"higher-peer", 101, fork, common.Hash{}, true},
		{"behind-same-chain", 99, ancestor, ancestor, false},
		{"behind-conflicting-chain", 99, fork, ancestor, true},
		{"behind-pruned-unknown", 99, ancestor, common.Hash{}, false},
		{"missing-peer-hash", 101, common.Hash{}, common.Hash{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := lqcPeerHeadRequiresSync(
				100, head, tc.number, tc.hash, tc.canonical,
			)
			if got != tc.want {
				t.Fatalf("requires sync=%v want=%v", got, tc.want)
			}
		})
	}
}

func TestLQCProductionNeedsSyncWithoutBackend(t *testing.T) {
	var backend *Ethereum
	if !backend.lqcProductionNeedsSync(nil) {
		t.Fatal("missing backend must not authorize production")
	}
}
