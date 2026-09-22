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
