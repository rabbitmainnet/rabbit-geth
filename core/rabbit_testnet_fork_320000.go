package core

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/params"
)

const RabbitTestnetFork320000 uint64 = 320000

func ConfigureRabbitTestnetFork320000(db ethdb.Database, genesis common.Hash, config *params.ChainConfig) error {
	official := common.HexToHash("0x9b71d7f2922fdf8383a4a12be5594e25938625195e0d84c05c3bd71b7bcf93f7")
	if genesis != official {
		return nil
	}
	if db == nil || config == nil || config.ChainID == nil ||
		!config.ChainID.IsUint64() || config.ChainID.Uint64() != 9280 ||
		config.LQC == nil {
		return fmt.Errorf("invalid Rabbit Testnet 320000 fork context")
	}

	candidate := *config
	lqc := *config.LQC
	candidate.LQC = &lqc

	// Reuse the existing guard for the exact historical public schedule.
	if err := ConfigureRabbitTestnetVRFAvailabilityV2(db, genesis, &candidate); err != nil {
		return err
	}
	for name, height := range map[string]uint64{
		"VRFEconomicsV2Block":      lqc.VRFEconomicsV2Block,
		"WorkSeatAdmissionV2Block": lqc.WorkSeatAdmissionV2Block,
	} {
		if height != 0 && height != RabbitTestnetFork320000 {
			return fmt.Errorf("refusing conflicting %s=%d; expected %d", name, height, RabbitTestnetFork320000)
		}
	}

	missing := lqc.VRFEconomicsV2Block == 0 || lqc.WorkSeatAdmissionV2Block == 0
	if missing {
		head := rawdb.ReadHeadHeaderHash(db)
		if head != (common.Hash{}) {
			number, found := rawdb.ReadHeaderNumber(db, head)
			if !found {
				return fmt.Errorf("missing head number during Rabbit fork scheduling")
			}
			if number >= RabbitTestnetFork320000 {
				return fmt.Errorf("Rabbit fork 320000 schedule missing after activation; manual recovery review required, no automatic rewind")
			}
		}
	}

	lqc.VRFEconomicsV2Block = RabbitTestnetFork320000
	lqc.WorkSeatAdmissionV2Block = RabbitTestnetFork320000
	if err := candidate.CheckConfigForkOrder(); err != nil {
		return err
	}
	config.LQC.VRFEconomicsV2Block = lqc.VRFEconomicsV2Block
	config.LQC.WorkSeatAdmissionV2Block = lqc.WorkSeatAdmissionV2Block
	return nil
}
