package eth

import (
	"github.com/ethereum/go-ethereum/common"
	"testing"
	"time"
)

func TestLQCFailedSyncRecoveryDecisionLab(t *testing.T) {
	a := common.HexToHash("0x01")
	b := common.HexToHash("0x02")
	t0 := time.Unix(1794252000, 0)
	cases := []struct {
		name                            string
		hash                            common.Hash
		seconds                         int
		readable, confirmed, noConflict bool
		want                            bool
	}{
		{"first agreement", a, 0, true, true, true, false},
		{"not stable yet", a, 2, true, true, true, false},
		{"same head stable", a, 3, true, true, true, true},
		{"recovery pending", a, 4, true, true, false, false},
		{"pending cleared starts over", a, 5, true, true, true, false},
		{"peer confirmation disappeared", a, 6, true, false, true, false},
		{"new confirmation starts over", a, 7, true, true, true, false},
		{"fork changes hash", b, 10, true, true, true, false},
		{"new fork stable", b, 13, true, true, true, true},
		{"state missing", b, 14, false, true, true, false},
		{"state returns starts over", b, 15, true, true, true, false},
		{"zero head rejected", common.Hash{}, 19, true, true, true, false},
	}
	var guard lqcFailedSyncRecoveryGuard
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := guard.observe(tc.hash, t0.Add(time.Duration(tc.seconds)*time.Second), tc.readable, tc.confirmed, tc.noConflict)
			if got != tc.want {
				t.Fatalf("allow=%t, want %t", got, tc.want)
			}
		})
	}
	guard.reset() // new SyncStarted / SyncFailed / SyncCompleted
	if guard.observe(a, t0.Add(30*time.Second), true, true, true) {
		t.Fatal("reset gate must not reuse prior stable observation")
	}
}
