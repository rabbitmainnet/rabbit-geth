//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package lqc

import (
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

func TestWorkV1EngineLabHistoricalEligibilityIsRegistryBound(
	t *testing.T,
) {
	a := common.HexToAddress(
		"0x00000000000000000000000000000000000000a1",
	)
	b := common.HexToAddress(
		"0x00000000000000000000000000000000000000b2",
	)
	snapshot, err := NewBootstrapRegistrySnapshot(
		1,
		crypto.Keccak256Hash([]byte("challenge-block")),
		[]common.Address{a},
	)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := snapshot.Registry()
	if err != nil {
		t.Fatal(err)
	}
	check := workV1EngineLabHistoricalEligibilityFromRegistry(
		registry,
		1,
		RegistrySnapshotRules{
			ActivationDelay: 64,
			HeartbeatWindow: 128,
			HeartbeatGrace:  16,
		},
	)
	if err := check(a); err != nil {
		t.Fatalf("historically eligible bootstrap rejected: %v", err)
	}
	if err := check(b); !errors.Is(
		err,
		ErrWorkV1EngineLabHistoricalIneligible,
	) {
		t.Fatalf("outsider error=%v", err)
	}
}

func TestWorkV1EngineLabSeatSelectionRejectsRepeatedWallet(
	t *testing.T,
) {
	a := common.HexToAddress(
		"0x00000000000000000000000000000000000000a1",
	)
	b := common.HexToAddress(
		"0x00000000000000000000000000000000000000b2",
	)
	c := common.HexToAddress(
		"0x00000000000000000000000000000000000000c3",
	)

	snapshot, err := NewBootstrapRegistrySnapshot(
		256,
		crypto.Keccak256Hash([]byte("registry-256")),
		[]common.Address{a, b, c},
	)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := snapshot.Registry()
	if err != nil {
		t.Fatal(err)
	}

	engine := &LQC{
		config: canonicalRegistryEngineConfig(
			[]common.Address{a, b, c},
			1,
		),
	}
	engine.config.FallbackCount = 1
	engine.config.CommitteeSize = 2

	seats := []WorkSeatV1{
		{
			TicketHash:  crypto.Keccak256Hash([]byte("seat-a-1")),
			Participant: a,
		},
		{
			TicketHash:  crypto.Keccak256Hash([]byte("seat-a-2")),
			Participant: a,
		},
		{
			TicketHash:  crypto.Keccak256Hash([]byte("seat-b-1")),
			Participant: b,
		},
		{
			TicketHash:  crypto.Keccak256Hash([]byte("seat-c-1")),
			Participant: c,
		},
		{
			TicketHash:  crypto.Keccak256Hash([]byte("seat-a-3")),
			Participant: a,
		},
	}

	_, active, err := engine.workV1EngineLabBuildSeatSelection(
		big.NewInt(928),
		1,
		crypto.Keccak256Hash([]byte("closed-selection-root")),
		seats,
		crypto.Keccak256Hash([]byte("dataset-key")),
		257,
		registry,
		RegistrySnapshotRules{
			ActivationDelay: 0,
			HeartbeatWindow: 128,
			HeartbeatGrace:  16,
		},
		func(key common.Hash, input []byte) (common.Hash, error) {
			return crypto.Keccak256Hash(
				key.Bytes(),
				input,
			), nil
		},
	)
	if err != ErrDuplicateWorkParticipantV1 {
		t.Fatalf("error=%v want=%v", err, ErrDuplicateWorkParticipantV1)
	}
	if active {
		t.Fatal("invalid repeated-wallet seats became active")
	}
}

func TestWorkV1EngineLabZeroSeatsUsesRegistryFallbackOnly(
	t *testing.T,
) {
	a := common.HexToAddress(
		"0x00000000000000000000000000000000000000a1",
	)
	snapshot, err := NewBootstrapRegistrySnapshot(
		256,
		crypto.Keccak256Hash([]byte("registry-zero-seat")),
		[]common.Address{a},
	)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := snapshot.Registry()
	if err != nil {
		t.Fatal(err)
	}
	engine := &LQC{
		config: canonicalRegistryEngineConfig(
			[]common.Address{a},
			1,
		),
	}

	selection, active, err := engine.workV1EngineLabBuildSeatSelection(
		big.NewInt(928),
		1,
		crypto.Keccak256Hash([]byte("closed-root")),
		nil,
		crypto.Keccak256Hash([]byte("dataset")),
		257,
		registry,
		engine.registryRules(),
		func(common.Hash, []byte) (common.Hash, error) {
			return crypto.Keccak256Hash([]byte("entropy")), nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if active || len(selection.Ordered) != 0 {
		t.Fatal("zero seats incorrectly activated WorkSeat selection")
	}
}

func TestWorkV1EngineLabV3RegistrySnapshotUsesRealHeaderHash(
	t *testing.T,
) {
	participants := testParticipants(t, 4)
	config := canonicalRegistryEngineConfig(participants, 1)
	config.EpochLength = WorkProtocolEpochLengthV1

	engine := New(config, rawdb.NewMemoryDatabase())
	genesis := &types.Header{
		Number:   big.NewInt(0),
		Time:     100,
		GasLimit: 30_000_000,
	}
	chain := canonicalRegistryTestChain(config, genesis)

	header1 := prepareCanonicalTestHeader(
		t,
		engine,
		chain,
		genesis,
	)
	if err := engine.VerifyHeader(chain, header1); err != nil {
		t.Fatal(err)
	}

	snapshot, ok := engine.cachedRegistrySnapshot(
		1,
		header1.Hash(),
	)
	if !ok || snapshot == nil {
		t.Fatal("real Header V3 hash is not the registry snapshot key")
	}
	if snapshot.Hash != header1.Hash() {
		t.Fatalf(
			"snapshot hash=%s real header=%s",
			snapshot.Hash,
			header1.Hash(),
		)
	}

	chain.headers[header1.Hash()] = header1
	chain.current = header1
	header2 := prepareCanonicalTestHeader(
		t,
		engine,
		chain,
		header1,
	)
	if err := engine.VerifyHeader(chain, header2); err != nil {
		t.Fatalf("second Header V3 failed after re-key: %v", err)
	}
}

func TestWorkV1EngineLabZeroWorkIgnoresRegisteredSybilIdentities(
	t *testing.T,
) {
	anchor := common.HexToAddress(
		"0xffffffffffffffffffffffffffffffffffffffff",
	)
	registry := NewCanonicalRegistry()
	if err := registry.ActivatePermissionlessProducer(anchor, 1); err != nil {
		t.Fatal(err)
	}

	const sybilCount = 5000
	for i := 1; i <= sybilCount; i++ {
		address := common.BigToAddress(big.NewInt(int64(i)))
		registry.entries[address] = CanonicalParticipant{
			Address:       address,
			RegisteredAt:  2,
			LastHeartbeat: 2,
			Sequence:      1,
			Active:        true,
		}
	}
	parentHash := crypto.Keccak256Hash([]byte("zero-work-parent"))
	parent := newRegistrySnapshot(9, parentHash, registry)
	header := &types.Header{
		Number:     big.NewInt(10),
		ParentHash: parentHash,
	}

	selection, err := workV1EngineLabActivationFallback(parent, header)
	if err != nil {
		t.Fatal(err)
	}
	if len(selection.Ordered) != 1 ||
		selection.Producer == nil ||
		selection.Producer.Address != anchor {
		t.Fatalf(
			"zero-work selection=%+v, want only activation anchor %s",
			selection,
			anchor,
		)
	}
	if allowed, _ := IsAuthorAllowed(
		selection,
		common.BigToAddress(big.NewInt(1)),
	); allowed {
		t.Fatal("registered Sybil identity received a free zero-work seat")
	}
}
