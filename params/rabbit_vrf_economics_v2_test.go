package params

import (
	"math/big"
	"testing"
)

func TestRabbitVRFEconomicsV2Boundary(t *testing.T) {
	c := &ChainConfig{LQC: &LQCConfig{
		VRFProtocolBlock:    136193,
		VRFEconomicsV2Block: 312000,
	}}
	for _, item := range []struct {
		height int64
		active bool
	}{
		{0, false}, {136193, false}, {311999, false},
		{312000, true}, {312001, true},
	} {
		if got := c.IsRabbitVRFEconomicsV2(big.NewInt(item.height)); got != item.active {
			t.Fatalf("height=%d active=%v", item.height, got)
		}
	}
	c.LQC.VRFEconomicsV2Block = 0
	if c.IsRabbitVRFEconomicsV2(big.NewInt(999999)) {
		t.Fatal("omitted activation enabled V2")
	}
	if c.IsRabbitVRFEconomicsV2(nil) {
		t.Fatal("nil height enabled V2")
	}
}
