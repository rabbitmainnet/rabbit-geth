package params

import "math/big"

// An omitted height keeps historical economics unchanged.
func (c *ChainConfig) IsRabbitVRFEconomicsV2(number *big.Int) bool {
	return c != nil && c.LQC != nil && number != nil &&
		number.Sign() >= 0 &&
		c.LQC.VRFEconomicsV2Block != 0 &&
		c.IsRabbitVRF(number) &&
		number.Cmp(new(big.Int).SetUint64(c.LQC.VRFEconomicsV2Block)) >= 0
}
