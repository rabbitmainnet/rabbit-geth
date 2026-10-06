//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package lqc

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/types"
)

func TestWorkV1CheckpointV5Restart(t *testing.T) {
	config := canonicalRegistryEngineConfig(testParticipants(t, 4), 1)
	config.ConsensusHardeningBlock = 1
	config.ConsensusStabilizationBlock = 1
	config.ConsensusFairnessBlock = 1
	config.ConsensusLivenessV3Block = 2
	config.ConsensusLivenessV4Block = 3
	config.ConsensusLivenessV5Block = 4
	config.ConsensusLivenessV6Block = 5
	config.EpochLength = 128
	config.VRFProtocolBlock = 385

	db := rawdb.NewMemoryDatabase()
	builder := New(config, db)
	genesis := &types.Header{
		Number:   big.NewInt(0),
		Time:     100,
		GasLimit: 30_000_000,
	}
	chain := canonicalRegistryTestChain(config, genesis)
	parent := genesis

	for number := uint64(1); number <= 512; number++ {
		header := prepareCanonicalTestHeader(t, builder, chain, parent)
		if err := builder.VerifyHeader(chain, header); err != nil {
			t.Fatalf("verify block %d: %v", number, err)
		}
		chain.headers[header.Hash()] = header
		chain.current = header
		parent = header
	}

	if _, err := ValidateLQCHeaderExtraV5(
		512, MaxWorkTicketsPerBlockV1, parent.Extra,
	); err != nil {
		t.Fatalf("checkpoint header is not valid V5: %v", err)
	}

	builder.persistWorkV1EngineCheckpointIfReady(parent.Hash())
	restarted := New(config, db)
	restored, err := restarted.workV1EngineLabRestoreCheckpoint(
		chain, 512, parent.Hash(),
	)
	if err != nil || !restored {
		t.Fatalf("V5 checkpoint not restored: restored=%v err=%v", restored, err)
	}
	runtime, found, err := restarted.workV1EngineLabCached(parent.Hash())
	if err != nil || !found || runtime == nil {
		t.Fatalf("restored runtime missing: found=%v err=%v", found, err)
	}
	t.Log("Checkpoint V5 do bloco 512 restaurado sem replay desde o bloco 384.")
}
