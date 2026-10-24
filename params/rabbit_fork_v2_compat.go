package params

import "math/big"

// Zero means disabled and must remain distinguishable from block zero.
func rabbitNewForkHeightV2(c *LQCConfig, economics bool) *big.Int {
	if c == nil {
		return nil
	}
	height := c.WorkSeatAdmissionV2Block
	if economics {
		height = c.VRFEconomicsV2Block
	}
	if height == 0 {
		return nil
	}
	return new(big.Int).SetUint64(height)
}
