//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package lqc

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/types"
)

func TestRabbitVRFServingBridgeUsesBlockEpochV1(
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

	engine := New(config, rawdb.NewMemoryDatabase())
	genesis := &types.Header{
		Number:   big.NewInt(0),
		Time:     100,
		GasLimit: 30_000_000,
	}
	chain := canonicalRegistryTestChain(config, genesis)

	runtime, err := NewCanonicalWorkRuntimeStateV1(
		chain.Config().ChainID,
		0,
		genesis.Hash(),
		WorkProtocolEpochLengthV1,
		big.NewInt(17),
	)
	if err != nil {
		t.Fatal(err)
	}

	parentHash := genesis.Hash()
	for number := uint64(1); number <= 256; number++ {
		header := &types.Header{
			ParentHash: parentHash,
			Number:     new(big.Int).SetUint64(number),
			Time:       100 + number,
			GasLimit:   30_000_000,
		}

		var challenge common.Hash
		var verified []VerifiedRandomXWorkTicketV1
		if number >= 129 {
			challenge = genesis.Hash()
		}
		if number == 129 {
			verified = []VerifiedRandomXWorkTicketV1{
				runtimeVerifiedV1(1, 0),
				runtimeVerifiedV1(1, 1),
			}
		}

		runtime, err = runtime.ApplyVerifiedBlockV1(
			chain.Config().ChainID,
			number,
			header.Hash(),
			runtime.Work.Hash,
			challenge,
			verified,
		)
		if err != nil {
			t.Fatalf("apply block %d: %v", number, err)
		}

		chain.headers[header.Hash()] = header
		chain.current = header
		parentHash = header.Hash()
	}

	if runtime.Work.SelectionEpoch != 1 {
		t.Fatalf(
			"selection epoch=%d want=1",
			runtime.Work.SelectionEpoch,
		)
	}
	if len(runtime.Work.SelectionSeats) != 2 {
		t.Fatalf(
			"selection seats=%d want=2",
			len(runtime.Work.SelectionSeats),
		)
	}

	if err := engine.workV1EngineLabRemember(
		chain.current.Hash(),
		runtime,
	); err != nil {
		t.Fatal(err)
	}

	prepared, exists, err := engine.RabbitVRFDKGBridgeContextV1(chain, 256, chain.current.Hash(), 257)
	if err != nil || !exists {
		t.Fatalf("preparation bridge: exists=%v err=%v", exists, err)
	}
	earlyEpoch, err := RabbitVRFEpochForBlockV1(257, 128)
	if err != nil || prepared.TargetVRFEpoch != earlyEpoch+1 {
		t.Fatalf("prepared target=%d preparation block epoch=%d err=%v", prepared.TargetVRFEpoch, earlyEpoch, err)
	}
	for number := uint64(257); number <= 384; number++ {
		header := &types.Header{
			ParentHash: parentHash,
			Number:     new(big.Int).SetUint64(number),
			Time:       100 + number,
			GasLimit:   30_000_000,
		}
		chain.headers[header.Hash()] = header
		chain.current = header
		parentHash = header.Hash()
	}
	serving, exists, err := engine.RabbitVRFDKGBridgeForTargetBlockV1(chain, 384, parentHash, 385, 128)
	if err != nil || !exists {
		t.Fatalf("serving bridge: exists=%v err=%v", exists, err)
	}
	epoch, err := RabbitVRFEpochForBlockV1(385, 128)
	if err != nil || serving.TargetVRFEpoch != epoch || serving.SessionID != prepared.SessionID {
		t.Fatalf("historical serving session mismatch: serving target=%d block epoch=%d err=%v", serving.TargetVRFEpoch, epoch, err)
	}
	if _, exists, err := engine.RabbitVRFDKGBridgeForTargetBlockV1(chain, 384, common.HexToHash("0xdead"), 385, 128); err == nil || exists {
		t.Fatal("serving bridge accepted a missing parent branch")
	}
}
