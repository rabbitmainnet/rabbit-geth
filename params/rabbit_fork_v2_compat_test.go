package params

import (
	"math/big"
	"testing"
)

func TestRabbitNewForkV2ConfigCompatibility(t *testing.T) {
	for _, economics := range []bool{true, false} {
		set := func(c *ChainConfig, height uint64) {
			if economics {
				c.LQC.VRFEconomicsV2Block = height
			} else {
				c.LQC.WorkSeatAdmissionV2Block = height
			}
		}
		for _, item := range []struct {
			old, next, head uint64
			incompatible    bool
		}{
			{0, 312000, 311999, false},
			{312000, 312001, 311999, false},
			{0, 312000, 312000, true},
			{312000, 0, 312000, true},
			{312000, 312001, 312000, true},
			{312000, 312000, 400000, false},
			{0, 311999, 311999, true},
		} {
			old := *TestChainConfig
			next := *TestChainConfig
			old.LQC = &LQCConfig{}
			next.LQC = &LQCConfig{}
			set(&old, item.old)
			set(&next, item.next)
			err := old.checkCompatible(&next,
				new(big.Int).SetUint64(item.head), 0)
			if (err != nil) != item.incompatible {
				t.Fatalf("economics=%v old=%d new=%d head=%d err=%v",
					economics, item.old, item.next, item.head, err)
			}
		}
	}
}
