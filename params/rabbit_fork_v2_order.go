package params

import "fmt"

func (c *LQCConfig) validateRabbitForkV2Order() error {
	if c == nil {
		return nil
	}
	if c.VRFEconomicsV2Block != 0 {
		if c.VRFProtocolBlock == 0 ||
			c.VRFEconomicsV2Block <= c.VRFProtocolBlock {
			return fmt.Errorf("VRF economics V2 must activate after VRF protocol")
		}
	}
	if c.WorkSeatAdmissionV2Block != 0 {
		if c.ConsensusLivenessV5Block == 0 ||
			c.WorkSeatAdmissionV2Block <= c.ConsensusLivenessV5Block ||
			(c.ConsensusLivenessV6Block != 0 &&
				c.WorkSeatAdmissionV2Block <= c.ConsensusLivenessV6Block) {
			return fmt.Errorf("WorkSeat admission V2 must activate after liveness V5/V6")
		}
	}
	return nil
}
