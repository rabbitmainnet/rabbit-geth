package eth

import (
	"github.com/ethereum/go-ethereum/common"
	"testing"
)

func TestLQCFailedExtensionGuardV1(t *testing.T) {
	a := common.HexToHash("0x01")
	b := common.HexToHash("0x02")

	cases := []struct {
		name     string
		failed   bool
		hash     common.Hash
		progress uint64
		want     bool
	}{
		{"first attempt", false, a, 100, false},
		{"same fork no progress", true, a, 100, true},
		{"different fork", true, b, 100, false},
		{"progress resumed", true, a, 101, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := lqcShouldDeferFailedExtensionV1(
				tc.failed, a, tc.hash, 100, tc.progress,
			)
			if got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}
