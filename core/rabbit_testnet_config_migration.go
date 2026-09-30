// Copyright 2026 The Rabbit Chain Authors
// Rabbit Testnet compatibility migration shared by startup and genesis-update paths.
package core

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
)

const (
	rabbitTestnetLivenessV4Block  uint64 = 97991
	rabbitTestnetLivenessV5Block  uint64 = 115000
	rabbitTestnetLivenessV6Block  uint64 = 115022
	rabbitTestnetVRFProtocolBlock uint64 = 136193
)

// MigrateRabbitTestnetStoredChainConfig normalizes only the known Rabbit Testnet
// historical config shapes. In particular, the v2.4.0 V4=136193 mistake is
// accepted only for the exact v2.4.0 shape and only before VRF activation.
func MigrateRabbitTestnetStoredChainConfig(
	chainDb ethdb.Database,
	genesisHash common.Hash,
	chainConfig *params.ChainConfig,
) bool {
	if chainDb == nil ||
		chainConfig == nil ||
		chainConfig.ChainID == nil ||
		chainConfig.LQC == nil {
		return false
	}

	cfg := chainConfig.LQC
	if chainConfig.ChainID.Cmp(big.NewInt(9280)) != 0 ||
		cfg.RegistryProtocolBlock != 1 ||
		cfg.ConsensusHardeningBlock != 50000 ||
		cfg.ConsensusStabilizationBlock != 50500 ||
		cfg.ConsensusFairnessBlock != 73000 ||
		cfg.ConsensusLivenessV3Block != 77000 {
		return false
	}

	brokenV240 := cfg.ConsensusLivenessV4Block == rabbitTestnetVRFProtocolBlock &&
		cfg.ConsensusLivenessV5Block == 0 &&
		cfg.ConsensusLivenessV6Block == 0 &&
		cfg.VRFProtocolBlock == rabbitTestnetVRFProtocolBlock

	if cfg.ConsensusLivenessV4Block != 0 &&
		cfg.ConsensusLivenessV4Block != rabbitTestnetLivenessV4Block &&
		!brokenV240 {
		return false
	}
	if cfg.ConsensusLivenessV5Block != 0 &&
		cfg.ConsensusLivenessV5Block != rabbitTestnetLivenessV5Block {
		return false
	}
	if cfg.ConsensusLivenessV6Block != 0 &&
		cfg.ConsensusLivenessV6Block != rabbitTestnetLivenessV6Block {
		return false
	}

	if brokenV240 {
		headHash := rawdb.ReadHeadHeaderHash(chainDb)
		headNumber, ok := rawdb.ReadHeaderNumber(chainDb, headHash)
		if !ok || headNumber >= rabbitTestnetVRFProtocolBlock {
			return false
		}
	}

	if cfg.ConsensusLivenessV4Block == rabbitTestnetLivenessV4Block &&
		cfg.ConsensusLivenessV5Block == rabbitTestnetLivenessV5Block &&
		cfg.ConsensusLivenessV6Block == rabbitTestnetLivenessV6Block {
		return false
	}

	oldV4 := cfg.ConsensusLivenessV4Block
	cfg.ConsensusLivenessV4Block = rabbitTestnetLivenessV4Block
	cfg.ConsensusLivenessV5Block = rabbitTestnetLivenessV5Block
	cfg.ConsensusLivenessV6Block = rabbitTestnetLivenessV6Block

	rawdb.WriteChainConfig(chainDb, genesisHash, chainConfig)

	log.Warn(
		"Migrated Rabbit Testnet stored chain config",
		"oldConsensusLivenessV4Block", oldV4,
		"consensusLivenessV4Block", rabbitTestnetLivenessV4Block,
		"consensusLivenessV5Block", rabbitTestnetLivenessV5Block,
		"consensusLivenessV6Block", rabbitTestnetLivenessV6Block,
	)
	return true
}
