package lqc

import (
	"crypto/ecdsa"
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethdb/memorydb"
)

var registrySnapshotTestRules = RegistrySnapshotRules{
	ProofDifficulty: 1,
	ActivationDelay: 0,
	HeartbeatWindow: 64,
	HeartbeatGrace:  16,
	JailBlocks:      256,
	MaxMissedTurns:  3,
}

func buildRegistrySnapshotHeader(t *testing.T, parent *RegistrySnapshot, chainID *big.Int, rules RegistrySnapshotRules, producer common.Address, operations []RegistryOperation) *types.Header {
	t.Helper()

	registry, err := parent.Registry()
	if err != nil {
		t.Fatal(err)
	}

	number := parent.Number + 1
	ordered := registry.OrderedParticipantsForBlock(
		parent.Hash,
		number,
		rules.ActivationDelay,
		rules.HeartbeatWindow,
		rules.HeartbeatGrace,
	)

	selection := HybridSelection{Ordered: ordered}
	allowed, queuePos := IsAuthorAllowed(selection, producer)
	if !allowed {
		t.Fatalf("producer %s not present in canonical queue for block %d", producer, number)
	}

	for index := 0; index < queuePos; index++ {
		if err := registry.ApplyMissedTurn(
			selection.Ordered[index].Address,
			number,
			rules.MaxMissedTurns,
			rules.JailBlocks,
		); err != nil {
			t.Fatal(err)
		}
	}

	if err := registry.MarkProducerHeartbeat(producer, number); err != nil {
		t.Fatal(err)
	}

	for _, operation := range CanonicalRegistryOperations(operations) {
		if err := registry.ApplyOperation(chainID, number, rules.ProofDifficulty, operation); err != nil {
			t.Fatal(err)
		}
	}

	extra, err := EncodeRegistryHeaderExtra(number, registry.Root(), operations)
	if err != nil {
		t.Fatal(err)
	}

	return &types.Header{
		ParentHash: parent.Hash,
		Number:     new(big.Int).SetUint64(number),
		Coinbase:   producer,
		Time:       number,
		Extra:      extra,
	}
}

func TestRegistrySnapshotAppliesHeaderAndProducerHeartbeat(t *testing.T) {
	chainID := big.NewInt(928)
	producerKey := testRegistryKey(t, "1313131313131313131313131313131313131313131313131313131313131313")
	newcomerKey := testRegistryKey(t, "1414141414141414141414141414141414141414141414141414141414141414")
	producer := registryAddress(producerKey)
	genesisHeader := &types.Header{Number: big.NewInt(0), Extra: []byte("genesis")}
	genesis, err := NewGenesisRegistrySnapshot(genesisHeader.Hash(), []common.Address{producer})
	if err != nil {
		t.Fatal(err)
	}
	register := registerOperation(t, chainID, newcomerKey, 1, 20, 1)
	header := buildRegistrySnapshotHeader(t, genesis, chainID, registrySnapshotTestRules, producer, []RegistryOperation{register})

	snapshot, err := genesis.ApplyHeader(chainID, registrySnapshotTestRules, header)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Number != 1 || snapshot.Hash != header.Hash() {
		t.Fatalf("unexpected snapshot head: number=%d hash=%s", snapshot.Number, snapshot.Hash)
	}
	registry, err := snapshot.Registry()
	if err != nil {
		t.Fatal(err)
	}
	participant, ok := registry.Participant(producer)
	if !ok || participant.LastHeartbeat != 1 || participant.Sequence != 0 {
		t.Fatalf("producer heartbeat was not derived correctly: %+v", participant)
	}
	if _, ok := registry.Participant(register.Address); !ok {
		t.Fatal("newcomer missing from post-block snapshot")
	}
}

func TestFreshNodeReconstructsRegistryOnlyFromHeaders(t *testing.T) {
	chainID := big.NewInt(928)
	keyA := testRegistryKey(t, "1515151515151515151515151515151515151515151515151515151515151515")
	keyB := testRegistryKey(t, "1616161616161616161616161616161616161616161616161616161616161616")
	keyC := testRegistryKey(t, "1717171717171717171717171717171717171717171717171717171717171717")
	addressA, addressB := registryAddress(keyA), registryAddress(keyB)
	genesisHeader := &types.Header{Number: big.NewInt(0), Extra: []byte("genesis-replay")}
	genesis, err := NewGenesisRegistrySnapshot(genesisHeader.Hash(), []common.Address{addressA, addressB})
	if err != nil {
		t.Fatal(err)
	}

	registerC := registerOperation(t, chainID, keyC, 1, 20, 1)
	header1 := buildRegistrySnapshotHeader(t, genesis, chainID, registrySnapshotTestRules, addressA, []RegistryOperation{registerC})
	snapshot1, err := genesis.ApplyHeader(chainID, registrySnapshotTestRules, header1)
	if err != nil {
		t.Fatal(err)
	}
	heartbeatC := signRegistryOperation(t, chainID, keyC, RegistryOperation{
		Version:    RegistryProtocolVersion,
		Action:     RegistryActionHeartbeat,
		Sequence:   2,
		ValidUntil: 20,
	})
	header2 := buildRegistrySnapshotHeader(t, snapshot1, chainID, registrySnapshotTestRules, addressB, []RegistryOperation{heartbeatC})
	snapshot2, err := snapshot1.ApplyHeader(chainID, registrySnapshotTestRules, header2)
	if err != nil {
		t.Fatal(err)
	}
	header3 := buildRegistrySnapshotHeader(t, snapshot2, chainID, registrySnapshotTestRules, registerC.Address, nil)
	want, err := snapshot2.ApplyHeader(chainID, registrySnapshotTestRules, header3)
	if err != nil {
		t.Fatal(err)
	}

	got, err := ReconstructRegistrySnapshot(genesis, chainID, registrySnapshotTestRules, []*types.Header{header1, header2, header3})
	if err != nil {
		t.Fatal(err)
	}
	if got.Hash != want.Hash || got.RegistryRoot != want.RegistryRoot || !canonicalParticipantsEqual(got.Participants, want.Participants) {
		t.Fatal("fresh node reconstructed a different registry snapshot")
	}
}

func TestRegistrySnapshotsAreForkAndHashIsolated(t *testing.T) {
	chainID := big.NewInt(928)
	producerKey := testRegistryKey(t, "1818181818181818181818181818181818181818181818181818181818181818")
	keyB := testRegistryKey(t, "1919191919191919191919191919191919191919191919191919191919191919")
	keyC := testRegistryKey(t, "1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a")
	producer := registryAddress(producerKey)
	genesisHeader := &types.Header{Number: big.NewInt(0), Extra: []byte("genesis-fork")}
	genesis, err := NewGenesisRegistrySnapshot(genesisHeader.Hash(), []common.Address{producer})
	if err != nil {
		t.Fatal(err)
	}
	headerB := buildRegistrySnapshotHeader(t, genesis, chainID, registrySnapshotTestRules, producer, []RegistryOperation{registerOperation(t, chainID, keyB, 1, 20, 1)})
	headerC := buildRegistrySnapshotHeader(t, genesis, chainID, registrySnapshotTestRules, producer, []RegistryOperation{registerOperation(t, chainID, keyC, 1, 20, 1)})
	snapshotB, err := genesis.ApplyHeader(chainID, registrySnapshotTestRules, headerB)
	if err != nil {
		t.Fatal(err)
	}
	snapshotC, err := genesis.ApplyHeader(chainID, registrySnapshotTestRules, headerC)
	if err != nil {
		t.Fatal(err)
	}
	if snapshotB.Hash == snapshotC.Hash || snapshotB.RegistryRoot == snapshotC.RegistryRoot {
		t.Fatal("fork snapshots were not isolated")
	}
	db := memorydb.New()
	if err := StoreRegistrySnapshot(db, snapshotB); err != nil {
		t.Fatal(err)
	}
	if err := StoreRegistrySnapshot(db, snapshotC); err != nil {
		t.Fatal(err)
	}
	loadedB, err := LoadRegistrySnapshot(db, snapshotB.Hash)
	if err != nil {
		t.Fatal(err)
	}
	loadedC, err := LoadRegistrySnapshot(db, snapshotC.Hash)
	if err != nil {
		t.Fatal(err)
	}
	if loadedB.RegistryRoot != snapshotB.RegistryRoot || loadedC.RegistryRoot != snapshotC.RegistryRoot {
		t.Fatal("hash-indexed snapshot cache returned the wrong fork")
	}
}

func TestRegistrySnapshotRejectsWrongRootUnknownProducerAndActivationBypass(t *testing.T) {
	chainID := big.NewInt(928)
	producerKey := testRegistryKey(t, "1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b")
	newcomerKey := testRegistryKey(t, "1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c")
	producer := registryAddress(producerKey)
	genesisHeader := &types.Header{Number: big.NewInt(0), Extra: []byte("genesis-invalid")}
	genesis, err := NewGenesisRegistrySnapshot(genesisHeader.Hash(), []common.Address{producer})
	if err != nil {
		t.Fatal(err)
	}
	header := buildRegistrySnapshotHeader(t, genesis, chainID, registrySnapshotTestRules, producer, nil)
	badRoot, err := EncodeRegistryHeaderExtra(1, common.HexToHash("0x01"), nil)
	if err != nil {
		t.Fatal(err)
	}
	header.Extra = badRoot
	if _, err := genesis.ApplyHeader(chainID, registrySnapshotTestRules, header); !errors.Is(err, ErrRegistryRootMismatch) {
		t.Fatalf("root error = %v, want %v", err, ErrRegistryRootMismatch)
	}

	unknownExtra, err := EncodeRegistryHeaderExtra(1, genesis.RegistryRoot, nil)
	if err != nil {
		t.Fatal(err)
	}
	unknown := &types.Header{ParentHash: genesis.Hash, Number: big.NewInt(1), Coinbase: registryAddress(newcomerKey), Time: 1, Extra: unknownExtra}
	if _, err := genesis.ApplyHeader(chainID, registrySnapshotTestRules, unknown); !errors.Is(err, ErrUnauthorizedRegistryProducer) {
		t.Fatalf("producer error = %v, want %v", err, ErrUnauthorizedRegistryProducer)
	}

	register := registerOperation(t, chainID, newcomerKey, 1, 20, 1)
	header1 := buildRegistrySnapshotHeader(t, genesis, chainID, registrySnapshotTestRules, producer, []RegistryOperation{register})
	snapshot1, err := genesis.ApplyHeader(chainID, registrySnapshotTestRules, header1)
	if err != nil {
		t.Fatal(err)
	}
	delayedRules := registrySnapshotTestRules
	delayedRules.ActivationDelay = 2
	number2 := snapshot1.Number + 1
	extra2, err := EncodeRegistryHeaderExtra(number2, snapshot1.RegistryRoot, nil)
	if err != nil {
		t.Fatal(err)
	}
	header2 := &types.Header{
		ParentHash: snapshot1.Hash,
		Number:     new(big.Int).SetUint64(number2),
		Coinbase:   register.Address,
		Time:       number2,
		Extra:      extra2,
	}
	if _, err := snapshot1.ApplyHeader(chainID, delayedRules, header2); !errors.Is(err, ErrUnauthorizedRegistryProducer) {
		t.Fatalf("activation error = %v, want %v", err, ErrUnauthorizedRegistryProducer)
	}
}

func TestBootstrapSnapshotEligibleAtBlockOneWithActivationDelay(t *testing.T) {
	participants := testParticipants(t, 3)
	genesisHeader := &types.Header{Number: big.NewInt(0), Extra: []byte("genesis-bootstrap-delay")}
	snapshot, err := NewGenesisRegistrySnapshot(genesisHeader.Hash(), participants)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := snapshot.Registry()
	if err != nil {
		t.Fatal(err)
	}
	eligible := registry.EligibleParticipants(1, 2, 64, 16)
	if len(eligible) != len(participants) {
		t.Fatalf("eligible bootstraps = %d, want %d", len(eligible), len(participants))
	}
}

func TestRegistrySnapshotRejectsCorruptedCache(t *testing.T) {
	key := testRegistryKey(t, "1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d")
	genesisHeader := &types.Header{Number: big.NewInt(0), Extra: []byte("genesis-cache")}
	genesis, err := NewGenesisRegistrySnapshot(genesisHeader.Hash(), []common.Address{registryAddress(key)})
	if err != nil {
		t.Fatal(err)
	}
	db := memorydb.New()
	if err := StoreRegistrySnapshot(db, genesis); err != nil {
		t.Fatal(err)
	}
	if err := db.Put(registrySnapshotKey(genesis.Hash), []byte{0x01, 0x02, 0x03}); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRegistrySnapshot(db, genesis.Hash); !errors.Is(err, ErrInvalidRegistrySnapshot) {
		t.Fatalf("cache error = %v, want %v", err, ErrInvalidRegistrySnapshot)
	}
}

func TestRegistrySnapshotAppliesExitAndRejectsFormerParticipant(t *testing.T) {
	chainID := big.NewInt(928)
	keyA := testRegistryKey(t, "1e1e1e1e1e1e1e1e1e1e1e1e1e1e1e1e1e1e1e1e1e1e1e1e1e1e1e1e1e1e1e1e")
	keyB := testRegistryKey(t, "1f1f1f1f1f1f1f1f1f1f1f1f1f1f1f1f1f1f1f1f1f1f1f1f1f1f1f1f1f1f1f1f")
	addressA, addressB := registryAddress(keyA), registryAddress(keyB)
	genesisHeader := &types.Header{Number: big.NewInt(0), Extra: []byte("genesis-exit")}
	genesis, err := NewGenesisRegistrySnapshot(genesisHeader.Hash(), []common.Address{addressA, addressB})
	if err != nil {
		t.Fatal(err)
	}
	exitB := signRegistryOperation(t, chainID, keyB, RegistryOperation{
		Version:    RegistryProtocolVersion,
		Action:     RegistryActionExit,
		Sequence:   1,
		ValidUntil: 20,
	})
	header1 := buildRegistrySnapshotHeader(t, genesis, chainID, registrySnapshotTestRules, addressA, []RegistryOperation{exitB})
	snapshot1, err := genesis.ApplyHeader(chainID, registrySnapshotTestRules, header1)
	if err != nil {
		t.Fatal(err)
	}
	extra, err := EncodeRegistryHeaderExtra(2, snapshot1.RegistryRoot, nil)
	if err != nil {
		t.Fatal(err)
	}
	attempt := &types.Header{ParentHash: snapshot1.Hash, Number: big.NewInt(2), Coinbase: addressB, Time: 2, Extra: extra}
	if _, err := snapshot1.ApplyHeader(chainID, registrySnapshotTestRules, attempt); !errors.Is(err, ErrUnauthorizedRegistryProducer) {
		t.Fatalf("former participant error = %v, want %v", err, ErrUnauthorizedRegistryProducer)
	}
}

func TestRegistrySnapshotDoesNotExpireParticipantByElapsedHeartbeat(t *testing.T) {
	chainID := big.NewInt(928)
	keyA := testRegistryKey(t, "2020202020202020202020202020202020202020202020202020202020202020")
	keyB := testRegistryKey(t, "2121212121212121212121212121212121212121212121212121212121212121")
	addressA, addressB := registryAddress(keyA), registryAddress(keyB)

	genesisHeader := &types.Header{Number: big.NewInt(0), Extra: []byte("genesis-no-expiry")}
	snapshot, err := NewGenesisRegistrySnapshot(genesisHeader.Hash(), []common.Address{addressA, addressB})
	if err != nil {
		t.Fatal(err)
	}

	// Passa deliberadamente da antiga janela heartbeat+grace.
	limit := registrySnapshotTestRules.HeartbeatWindow +
		registrySnapshotTestRules.HeartbeatGrace + 1

	for block := uint64(1); block <= limit; block++ {
		header := buildRegistrySnapshotHeader(
			t,
			snapshot,
			chainID,
			registrySnapshotTestRules,
			addressA,
			nil,
		)
		snapshot, err = snapshot.ApplyHeader(
			chainID,
			registrySnapshotTestRules,
			header,
		)
		if err != nil {
			t.Fatalf("block %d: %v", block, err)
		}
	}

	registry, err := snapshot.Registry()
	if err != nil {
		t.Fatal(err)
	}

	participantB, exists := registry.Participant(addressB)
	if !exists || !participantB.Active {
		t.Fatalf("participant B disappeared after elapsed heartbeat window: %+v", participantB)
	}

	// Mesmo depois da antiga janela de expiração, B continua fazendo parte
	// da fila e deve conseguir assumir um bloco quando sua janela chegar.
	headerB := buildRegistrySnapshotHeader(
		t,
		snapshot,
		chainID,
		registrySnapshotTestRules,
		addressB,
		nil,
	)

	nextSnapshot, err := snapshot.ApplyHeader(
		chainID,
		registrySnapshotTestRules,
		headerB,
	)
	if err != nil {
		t.Fatalf("participant B should remain recoverable after old heartbeat deadline: %v", err)
	}

	nextRegistry, err := nextSnapshot.Registry()
	if err != nil {
		t.Fatal(err)
	}

	participantB, exists = nextRegistry.Participant(addressB)
	if !exists || participantB.LastHeartbeat != nextSnapshot.Number {
		t.Fatalf("participant B production was not recorded: %+v", participantB)
	}
}

func registryAddress(key *ecdsa.PrivateKey) common.Address {
	return crypto.PubkeyToAddress(key.PublicKey)
}
