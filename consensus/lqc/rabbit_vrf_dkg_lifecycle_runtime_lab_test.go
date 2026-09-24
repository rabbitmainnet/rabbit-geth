//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package lqc

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/types"
)

func TestRabbitVRFDKGLifecycleRuntimeV1BeforePreparationNoSideEffects(
	t *testing.T,
) {
	config := canonicalRegistryEngineConfig(
		testParticipants(t, 2),
		1,
	)
	config.EpochLength = WorkProtocolEpochLengthV1
	config.ProofDifficulty = 17
	config.RegistryProtocolBlock = 0
	config.VRFProtocolBlock = 385

	db := rawdb.NewMemoryDatabase()
	engine := New(config, db)

	genesis := &types.Header{
		Number:   big.NewInt(0),
		Time:     100,
		GasLimit: 30_000_000,
	}
	chain := canonicalRegistryTestChain(
		config,
		genesis,
	)

	header := &types.Header{
		ParentHash: common.HexToHash("0xdeadbeef"),
		Number:     new(big.Int).SetUint64(256),
		Time:       356,
		GasLimit:   30_000_000,
	}

	if err :=
		engine.maybeEnsureRabbitVRFDKGLifecycleV1(
			chain,
			header,
		); err != nil {
		t.Fatal(err)
	}

	if _, exists :=
		workV1EngineLabRuntimes.Load(engine); exists {
		t.Fatal(
			"pre-preparation lifecycle created Work runtime side effect",
		)
	}

	iterator := db.NewIterator(
		rabbitVRFDKGLifecyclePrefixV1,
		nil,
	)
	defer iterator.Release()

	if iterator.Next() {
		t.Fatal(
			"pre-preparation lifecycle unexpectedly persisted",
		)
	}

	if err := iterator.Error(); err != nil {
		t.Fatal(err)
	}
}
