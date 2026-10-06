//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package lqc

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
)

func TestWorkSeatRegistryReplayMatchesProductionV5(t *testing.T) {
	chainID := big.NewInt(9280)
	anchor := common.HexToAddress("0x00000000000000000000000000000000000000f0")
	seatAddresses := []common.Address{
		common.HexToAddress("0x00000000000000000000000000000000000000a1"),
		common.HexToAddress("0x00000000000000000000000000000000000000b2"),
		common.HexToAddress("0x00000000000000000000000000000000000000c3"),
		common.HexToAddress("0x00000000000000000000000000000000000000d4"),
	}

	config := canonicalRegistryEngineConfig(append([]common.Address{anchor}, seatAddresses...), 1)
	config.EpochLength = 8
	config.CommitteeSize = 2
	config.FallbackCount = 5
	config.ConsensusLivenessV3Block = 5

	config.ConsensusHardeningBlock = 1
	config.ConsensusStabilizationBlock = 2
	config.ConsensusFairnessBlock = 3
	config.ConsensusLivenessV4Block = 6
	config.ConsensusLivenessV5Block = 7
	config.VRFProtocolBlock = 25
	engine := New(config, rawdb.NewMemoryDatabase())

	genesis := &types.Header{
		Number:   big.NewInt(0),
		Time:     1_000,
		GasLimit: 30_000_000,
		Extra:    []byte("block-0"),
	}
	chain := &testHeaderChain{
		config: &params.ChainConfig{
			ChainID: new(big.Int).Set(chainID),
			LQC:     config,
		},
		headers: map[common.Hash]*types.Header{genesis.Hash(): genesis},
		current: genesis,
	}
	parentHeader := genesis
	for number := uint64(1); number <= 56; number++ {
		header := &types.Header{
			ParentHash: parentHeader.Hash(),
			Number:     new(big.Int).SetUint64(number),
			Time:       1_000 + number*10,
			GasLimit:   genesis.GasLimit,
			Extra:      []byte{byte(number), byte(number >> 8)},
		}
		chain.headers[header.Hash()] = header
		chain.current = header
		parentHeader = header
	}

	sourceEpoch, hasSource, err := WorkSelectionSourceEpochV1(57, 8)
	if err != nil || !hasSource || sourceEpoch != 6 {
		t.Fatalf("block57 source epoch=%d has=%v err=%v want=6/true/nil", sourceEpoch, hasSource, err)
	}

	seats := make([]WorkSeatV1, 0, len(seatAddresses))
	for index, address := range seatAddresses {
		seats = append(seats, WorkSeatV1{
			TicketHash:  crypto.Keccak256Hash([]byte{byte(index + 1)}),
			Participant: address,
		})
	}
	closed, err := NewWorkEpochSnapshotV1(
		chainID,
		sourceEpoch,
		crypto.Keccak256Hash([]byte("epoch-6-anchor")),
		big.NewInt(1),
		seats,
	)
	if err != nil {
		t.Fatal(err)
	}
	parentRuntime := &CanonicalWorkRuntimeStateV1{
		Work: &WorkChainSnapshotV1{
			Number:              56,
			Hash:                parentHeader.Hash(),
			EpochLength:         8,
			SelectionEpoch:      sourceEpoch,
			SelectionAnchor:     closed.Anchor,
			SelectionDifficulty: new(big.Int).Set(closed.Difficulty),
			SelectionRoot:       closed.Root,
			SelectionSeats:      cloneWorkSeatsV1(closed.Seats),
		},
	}

	registry := NewCanonicalRegistry()
	_ = anchor
	for _, address := range seatAddresses {
		registry.entries[address] = CanonicalParticipant{
			Address:       address,
			RegisteredAt:  1,
			LastHeartbeat: 56,
			Sequence:      1,
			Active:        true,
		}
	}
	parentRegistry := newRegistrySnapshot(56, parentHeader.Hash(), registry)

	workV1EngineLabRuntimes.Store(engine, &workV1EngineLabRuntime{
		hasher: func(datasetKey common.Hash, input []byte) (common.Hash, error) {
			return crypto.Keccak256Hash(datasetKey.Bytes(), input), nil
		},
		runtimes:               make(map[common.Hash]*CanonicalWorkRuntimeStateV1),
		claimLedgers:           make(map[common.Hash]*CommitteeClaimLedgerV1),
		validatedFinalizations: make(map[common.Hash][]RabbitVRFFinalizationV1),
		selectionBeaconCache:   make(map[workV1SelectionBeaconCacheKey]common.Hash),
	})
	t.Cleanup(func() {
		workV1EngineLabRuntimes.Delete(engine)
	})

	engine.rememberRegistrySnapshot(parentRegistry)
	runtimeState, err := workV1EngineLabRuntimeFor(engine)
	if err != nil {
		t.Fatal(err)
	}
	runtimeState.mu.Lock()
	runtimeState.runtimes[parentHeader.Hash()] = parentRuntime
	runtimeState.claimLedgers[parentHeader.Hash()] = &CommitteeClaimLedgerV1{}
	runtimeState.mu.Unlock()

	skeleton := &types.Header{
		ParentHash: parentHeader.Hash(),
		Number:     big.NewInt(57),
		Time:       parentHeader.Time + 10,
		GasLimit:   genesis.GasLimit,
	}
	selection, active, err := engine.workV1EngineLabSelectionForHeader(
		chain, parentRuntime, parentRegistry, skeleton,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !active {
		t.Fatal("fixture did not activate normal WorkSeat selection")
	}
	if len(selection.Ordered) < 2 {
		t.Fatal("fixture requires a producer and fallback")
	}

	tested := 0
	for _, participant := range selection.Ordered {
		allowed, _ := engine.isAuthorAllowedAt(57, selection, participant.Address)
		if !allowed {
			continue
		}
		tested++

		header := types.CopyHeader(skeleton)
		header.Coinbase = participant.Address
		registry, err := parentRegistry.Registry()
		if err != nil {
			t.Fatal(err)
		}
		if err := engine.workV1EngineLabApplySeatLiveness(
			registry, 57, selection, header.Coinbase, engine.registryRules(),
		); err != nil {
			t.Fatal(err)
		}
		expectedRoot := registry.Root()
		header.Extra, err = EncodeLQCHeaderExtraV5(
			57, expectedRoot,
			crypto.Keccak256Hash([]byte("test-work-root")),
			crypto.Keccak256Hash([]byte("test-claim-root")),
			nil, nil, nil, nil, MaxWorkTicketsPerBlockV1,
		)
		if err != nil {
			t.Fatal(err)
		}
		snapshot, err := engine.applyRegistrySnapshotHeaderMaybeV3(
			chain, parentRegistry, header,
		)
		if err != nil {
			t.Fatalf("replay diverged from WorkSeat production for %s: %v",
				header.Coinbase, err)
		}
		if snapshot.RegistryRoot != expectedRoot {
			t.Fatalf("replay root=%s production root=%s",
				snapshot.RegistryRoot, expectedRoot)
		}
	}

	if tested < 2 {
		t.Fatal("fixture must test producer and authorized fallback")
	}
}
