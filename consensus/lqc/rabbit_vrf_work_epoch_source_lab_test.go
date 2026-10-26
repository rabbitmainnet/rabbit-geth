//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package lqc

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/types"
)

func TestRabbitVRFActiveBridgeUsesWorkEpochDespiteLegacyEightV1(
	t *testing.T,
) {
	config := canonicalRegistryEngineConfig(
		testParticipants(t, 2),
		1,
	)
	config.EpochLength = 8
	config.ProofDifficulty = 17
	config.RegistryProtocolBlock = 0
	config.VRFProtocolBlock = 25

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

	length, err := engine.rabbitVRFCanonicalWorkEpochLengthV1(chain, 256, chain.current.Hash())
	if err != nil || length != 128 {
		t.Fatalf("canonical Work epoch length=%d err=%v; want128", length, err)
	}
	if chain.Config().LQC.EpochLength != 8 {
		t.Fatal("legacy configuration was mutated")
	}
	// At block257 Work is in epoch3, before the first possible target4.
	// Using legacy length8 instead incorrectly requests target33.
	_, exists, err := engine.RabbitVRFActiveBridgeContextV1(chain, 256, chain.current.Hash(), 257)
	if err != nil || exists {
		t.Fatalf("before first Work target: exists=%v err=%v", exists, err)
	}
	future, exists, err := engine.RabbitVRFDKGBridgeContextV1(chain, 256, chain.current.Hash(), 257)
	if err != nil || !exists || future.TargetVRFEpoch != 4 {
		t.Fatalf("preparation must remain bound to target4: exists=%v target=%d err=%v", exists, future.TargetVRFEpoch, err)
	}
}
