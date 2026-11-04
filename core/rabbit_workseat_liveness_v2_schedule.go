package core

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/params"
)

// Public testnet activation for WorkSeat liveness V2.
const RabbitTestnetWorkSeatLivenessV2Block uint64 = 323800

func ConfigureRabbitTestnetWorkSeatLivenessV2(db ethdb.Database, genesis common.Hash, config *params.ChainConfig) error {
	return configureRabbitTestnetWorkSeatLivenessV2At(db, genesis, config, RabbitTestnetWorkSeatLivenessV2Block)
}

func configureRabbitTestnetWorkSeatLivenessV2At(db ethdb.Database, genesis common.Hash, config *params.ChainConfig, activation uint64) error {
	official := common.HexToHash("0x9b71d7f2922fdf8383a4a12be5594e25938625195e0d84c05c3bd71b7bcf93f7")
	if activation == 0 || genesis != official {
		return nil
	}
	if db == nil || config == nil || config.ChainID == nil ||
		!config.ChainID.IsUint64() || config.ChainID.Uint64() != 9280 ||
		config.LQC == nil {
		return fmt.Errorf("invalid Rabbit WorkSeat liveness V2 scheduling context")
	}

	candidate := *config
	lqc := *config.LQC
	candidate.LQC = &lqc
	if lqc.WorkSeatAdmissionV2Block != RabbitTestnetFork320000 ||
		activation <= RabbitTestnetFork320000 {
		return fmt.Errorf("WorkSeat liveness V2 must follow the public admission fork")
	}
	if lqc.WorkSeatLivenessV2Block != 0 && lqc.WorkSeatLivenessV2Block != activation {
		return fmt.Errorf("conflicting WorkSeat liveness V2 activation")
	}

	// The original genesis omits this field. Inherit only the exact
	// release schedule already stored for this genesis.
	persisted := rawdb.ReadChainConfig(db, genesis)
	if persisted != nil && persisted.LQC != nil {
		stored := persisted.LQC.WorkSeatLivenessV2Block
		if stored != 0 && stored != activation {
			return fmt.Errorf("conflicting persisted WorkSeat liveness V2 activation")
		}
		if stored == activation {
			lqc.WorkSeatLivenessV2Block = activation
		}
	}

	if lqc.WorkSeatLivenessV2Block == 0 {
		head := rawdb.ReadHeadHeaderHash(db)
		if head != (common.Hash{}) {
			number, found := rawdb.ReadHeaderNumber(db, head)
			if !found {
				return fmt.Errorf("missing head number during WorkSeat liveness V2 scheduling")
			}
			if number >= activation {
				return fmt.Errorf("WorkSeat liveness V2 schedule missing after activation; no automatic rewind")
			}
		}
	}
	lqc.WorkSeatLivenessV2Block = activation
	if err := candidate.CheckConfigForkOrder(); err != nil {
		return err
	}
	config.LQC.WorkSeatLivenessV2Block = activation
	return nil
}
