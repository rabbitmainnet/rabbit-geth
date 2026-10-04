package params

import (
	"fmt"
	"math/big"
)

func (c *LQCConfig) vrfAvailabilityV2ForkBlock() *big.Int {
	if c == nil || c.VRFAvailabilityV2Block == 0 {
		return nil
	}
	return new(big.Int).SetUint64(c.VRFAvailabilityV2Block)
}

func (c *LQCConfig) validateVRFAvailabilityV2() error {
	if c == nil || c.VRFAvailabilityV2Block == 0 {
		return nil
	}
	if c.RegistryProtocolBlock == 0 {
		return fmt.Errorf("VRF availability requires canonical registry protocol")
	}
	if c.VRFProtocolBlock == 0 || c.VRFAvailabilityV2Block <= c.VRFProtocolBlock {
		return fmt.Errorf("vrfAvailabilityV2Block must follow vrfProtocolBlock")
	}
	if c.EpochLength == 0 || (c.VRFAvailabilityV2Block-1)%c.EpochLength != 0 {
		return fmt.Errorf("vrfAvailabilityV2Block must start a DKG preparation epoch")
	}
	if c.HeartbeatWindow == 0 || c.HeartbeatGrace > ^uint64(0)-c.HeartbeatWindow {
		return fmt.Errorf("VRF availability requires a bounded nonzero heartbeat window")
	}
	return nil
}

// Select by the frozen preparation start, never by the current head.
func (c *LQCConfig) IsVRFAvailabilityV2Preparation(preparationBlock uint64) bool {
	return c != nil && c.VRFAvailabilityV2Block != 0 && preparationBlock >= c.VRFAvailabilityV2Block
}
