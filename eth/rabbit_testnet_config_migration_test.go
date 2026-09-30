package eth

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/params"
)

func TestMigrateRabbitTestnetLivenessV4Config(t *testing.T) {
	db := rawdb.NewMemoryDatabase()
	genesisHash := common.HexToHash("0x9280")

	config := &params.ChainConfig{
		ChainID: big.NewInt(9280),
		LQC: &params.LQCConfig{
			RegistryProtocolBlock:       1,
			ConsensusHardeningBlock:     50000,
			ConsensusStabilizationBlock: 50500,
			ConsensusFairnessBlock:      73000,
			ConsensusLivenessV3Block:    77000,
		},
	}

	if !migrateRabbitTestnetLivenessV4Config(
		db,
		genesisHash,
		config,
	) {
		t.Fatal("migration did not run")
	}

	if config.LQC.ConsensusLivenessV4Block != 97991 {
		t.Fatalf(
			"runtime V4 block=%d want=97991",
			config.LQC.ConsensusLivenessV4Block,
		)
	}

	stored := rawdb.ReadChainConfig(db, genesisHash)

	if stored == nil || stored.LQC == nil {
		t.Fatal("stored config missing")
	}

	if stored.LQC.ConsensusLivenessV4Block != 97991 {
		t.Fatalf(
			"stored V4 block=%d want=97991",
			stored.LQC.ConsensusLivenessV4Block,
		)
	}

	if migrateRabbitTestnetLivenessV4Config(
		db,
		genesisHash,
		config,
	) {
		t.Fatal("migration ran twice")
	}
}

func TestMigrateRabbitTestnetBrokenV240ConfigBeforeVRFFork(t *testing.T) {
	db := rawdb.NewMemoryDatabase()
	genesisHash := common.HexToHash("0x9280")
	headHash := common.HexToHash("0x135729")
	rawdb.WriteHeaderNumber(db, headHash, 135729)
	rawdb.WriteHeadHeaderHash(db, headHash)
	config := &params.ChainConfig{
		ChainID: big.NewInt(9280),
		LQC: &params.LQCConfig{
			RegistryProtocolBlock:       1,
			ConsensusHardeningBlock:     50000,
			ConsensusStabilizationBlock: 50500,
			ConsensusFairnessBlock:      73000,
			ConsensusLivenessV3Block:    77000,
			ConsensusLivenessV4Block:    136193,
			VRFProtocolBlock:            136193,
		},
	}
	if !migrateRabbitTestnetLivenessV4Config(db, genesisHash, config) {
		t.Fatal("broken v2.4.0 config was not migrated before VRF fork")
	}
	if config.LQC.ConsensusLivenessV4Block != 97991 ||
		config.LQC.ConsensusLivenessV5Block != 115000 ||
		config.LQC.ConsensusLivenessV6Block != 115022 ||
		config.LQC.VRFProtocolBlock != 136193 {
		t.Fatalf("unexpected migrated config: %+v", config.LQC)
	}
}

func TestMigrateRabbitTestnetBrokenV240ConfigRejectedAtVRFFork(t *testing.T) {
	db := rawdb.NewMemoryDatabase()
	genesisHash := common.HexToHash("0x9280")
	headHash := common.HexToHash("0x136193")
	rawdb.WriteHeaderNumber(db, headHash, 136193)
	rawdb.WriteHeadHeaderHash(db, headHash)
	config := &params.ChainConfig{
		ChainID: big.NewInt(9280),
		LQC: &params.LQCConfig{
			RegistryProtocolBlock:       1,
			ConsensusHardeningBlock:     50000,
			ConsensusStabilizationBlock: 50500,
			ConsensusFairnessBlock:      73000,
			ConsensusLivenessV3Block:    77000,
			ConsensusLivenessV4Block:    136193,
			VRFProtocolBlock:            136193,
		},
	}
	if migrateRabbitTestnetLivenessV4Config(db, genesisHash, config) {
		t.Fatal("broken v2.4.0 config migrated at or after VRF activation")
	}
}
