package core

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/params"
)

// LoadChainConfigForConsensus resolves startup schedules before the consensus
// engine retains its configuration. SetupGenesisBlock persists them later.
func LoadChainConfigForConsensus(db ethdb.Database, genesis *Genesis) (*params.ChainConfig, common.Hash, error) {
	cfg, hash, err := LoadChainConfig(db, genesis)
	if err != nil {
		return nil, hash, err
	}
	// Avoid modifying a supplied genesis or a shared configuration on failure.
	resolved := *cfg
	if cfg.LQC != nil {
		lqc := *cfg.LQC
		resolved.LQC = &lqc
	}
	if err := ConfigureRabbitTestnetVRFAvailabilityV2(db, hash, &resolved); err != nil {
		return nil, hash, err
	}
	if err := ConfigureRabbitTestnetFork320000(db, hash, &resolved); err != nil {
		return nil, hash, err
	}
	if err := ConfigureRabbitTestnetWorkSeatLivenessV2(db, hash, &resolved); err != nil {
		return nil, hash, err
	}
	return &resolved, hash, nil
}
