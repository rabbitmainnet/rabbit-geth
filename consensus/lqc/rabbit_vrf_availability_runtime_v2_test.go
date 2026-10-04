//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package lqc

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/types"
)

func TestRabbitVRFAvailabilityV2RuntimeFreezesPreparationParent(
	t *testing.T,
) {
	config := canonicalRegistryEngineConfig(
		testParticipants(t, 2),
		1,
	)
	config.EpochLength = WorkProtocolEpochLengthV1
	config.ProofDifficulty = 17
	config.RegistryProtocolBlock = 1
	config.VRFAvailabilityV2Block = 513
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
	for number := uint64(1); number <= 512; number++ {
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

	if len(runtime.Work.SelectionSeats) != 2 {
		t.Fatal("expected two historical seats")
	}
	if err := engine.workV1EngineLabRemember(chain.current.Hash(), runtime); err != nil {
		t.Fatal(err)
	}
	registry := NewCanonicalRegistry()
	for i, seat := range runtime.Work.SelectionSeats {
		registry.entries[seat.Participant] = CanonicalParticipant{Address: seat.Participant, Active: i == 0, LastHeartbeat: 511}
	}
	frozen := &RegistrySnapshot{Number: 512, Hash: chain.current.Hash(), RegistryRoot: registry.Root(), Participants: registry.Participants()}
	engine.rememberRegistrySnapshot(frozen)
	first, exists, err := engine.RabbitVRFDKGBridgeContextV1(chain, 512, chain.current.Hash(), 513)
	if err != nil || !exists || len(first.Members) != 1 {
		t.Fatalf("bad integrated bridge: %v %v %d", exists, err, len(first.Members))
	}
	if first.Members[0].Participant != runtime.Work.SelectionSeats[0].Participant {
		t.Fatal("inactive member selected")
	}
	header := &types.Header{ParentHash: chain.current.Hash(), Number: big.NewInt(513), Time: 613, GasLimit: 30_000_000}
	runtime, err = runtime.ApplyVerifiedBlockV1(chain.Config().ChainID, 513, header.Hash(), runtime.Work.Hash, genesis.Hash(), nil)
	if err != nil {
		t.Fatal(err)
	}
	chain.headers[header.Hash()] = header
	chain.current = header
	if err := engine.workV1EngineLabRemember(header.Hash(), runtime); err != nil {
		t.Fatal(err)
	}
	// The current registry now says both are active. The frozen session must not.
	for address, participant := range registry.entries {
		participant.Active = true
		participant.LastHeartbeat = 513
		registry.entries[address] = participant
	}
	engine.rememberRegistrySnapshot(&RegistrySnapshot{Number: 513, Hash: header.Hash(), RegistryRoot: registry.Root(), Participants: registry.Participants()})
	second, exists, err := engine.RabbitVRFDKGBridgeContextV1(chain, 513, header.Hash(), 514)
	if err != nil || !exists || second.SessionID != first.SessionID {
		t.Fatalf("head advancement changed frozen session: %v", err)
	}
}
