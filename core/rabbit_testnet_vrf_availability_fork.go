package core

import (
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/params"
)

const RabbitTestnetVRFAvailabilityV2Block uint64 = 173057

// Configure only the official public testnet. Never reschedule an existing fork
// or install a new rule retroactively on a database that has crossed activation.
func ConfigureRabbitTestnetVRFAvailabilityV2(db ethdb.Database, genesis common.Hash, config *params.ChainConfig) error {
	if genesis != common.HexToHash("0x9b71d7f2922fdf8383a4a12be5594e25938625195e0d84c05c3bd71b7bcf93f7") {
		return nil
	}
	if db == nil || config == nil || config.ChainID == nil || config.ChainID.Uint64() != 9280 || config.LQC == nil {
		return fmt.Errorf("invalid official Rabbit Testnet fork context")
	}
	c := config.LQC
	if c.RegistryProtocolBlock != 1 || c.ConsensusHardeningBlock != 50000 || c.ConsensusStabilizationBlock != 50500 || c.ConsensusFairnessBlock != 73000 || c.ConsensusLivenessV3Block != 77000 || c.ConsensusLivenessV4Block != 97991 || c.ConsensusLivenessV5Block != 115000 || c.ConsensusLivenessV6Block != 115022 || c.VRFProtocolBlock != 136193 || c.MiningRewardV5Block != 153601 || c.EpochLength != 128 {
		return fmt.Errorf("unexpected official Rabbit Testnet historical fork schedule")
	}
	if c.VRFAvailabilityV2Block == RabbitTestnetVRFAvailabilityV2Block {
		return config.CheckConfigForkOrder()
	}
	if c.VRFAvailabilityV2Block != 0 {
		return fmt.Errorf("refusing to replace Rabbit Testnet VRF availability fork %d", c.VRFAvailabilityV2Block)
	}
	headHash := rawdb.ReadHeadHeaderHash(db)
	if headHash != (common.Hash{}) {
		number, found := rawdb.ReadHeaderNumber(db, headHash)
		if !found {
			return fmt.Errorf("missing Rabbit Testnet head number during fork configuration")
		}
		if number >= RabbitTestnetVRFAvailabilityV2Block {
			return fmt.Errorf("Rabbit Testnet passed fork %d without its schedule; recovery review required, no automatic rewind", RabbitTestnetVRFAvailabilityV2Block)
		}
	}
	c.VRFAvailabilityV2Block = RabbitTestnetVRFAvailabilityV2Block
	if err := config.CheckConfigForkOrder(); err != nil {
		c.VRFAvailabilityV2Block = 0
		return err
	}
	return nil
}
