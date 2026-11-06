package params

import (
	"fmt"
	"math/big"
)

func (c *LQCConfig) workSeatLivenessV2ForkBlock() *big.Int {
	if c == nil || c.WorkSeatLivenessV2Block == 0 {
		return nil
	}
	return new(big.Int).SetUint64(c.WorkSeatLivenessV2Block)
}

func (c *LQCConfig) validateWorkSeatLivenessV2() error {
	if c == nil || c.WorkSeatLivenessV2Block == 0 {
		return nil
	}
	if c.WorkSeatAdmissionV2Block == 0 || c.WorkSeatLivenessV2Block <= c.WorkSeatAdmissionV2Block {
		return fmt.Errorf("workSeatLivenessV2Block must follow workSeatAdmissionV2Block")
	}
	if c.ConsensusLivenessV5Block == 0 || c.WorkSeatLivenessV2Block <= c.ConsensusLivenessV5Block {
		return fmt.Errorf("workSeatLivenessV2Block must follow consensusLivenessV5Block")
	}
	return nil
}
