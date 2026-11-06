//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package lqc

import (
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/types"
)

func TestRabbitReconstructionCancellation(t *testing.T) {
	config := canonicalRegistryEngineConfig(testParticipants(t, 4), 1)
	db := rawdb.NewMemoryDatabase()
	defer db.Close()
	engine := New(config, db)
	genesis := &types.Header{
		Number: big.NewInt(0), Time: 100, GasLimit: 30000000,
	}
	chain := canonicalRegistryTestChain(config, genesis)

	engine.StopLocalReconstruction()
	engine.StopLocalReconstruction()
	_, err := engine.workV1EngineLabRuntimeAt(chain, 0, genesis.Hash())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("reconstruction after shutdown: got %v, want cancellation", err)
	}
}
