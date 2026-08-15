package lqc

import (
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethdb/memorydb"
)

func snapshotEnvelope(t *testing.T, chainID *big.Int, snapshot *WorkTicketSnapshot, active []common.Address, tickets []WorkTicket) []byte {
	t.Helper()
	states, err := snapshot.States(chainID)
	if err != nil {
		t.Fatal(err)
	}
	states, _, err = reconcileWorkTicketParticipants(chainID, snapshot.Anchor, snapshot.Epoch, states, active)
	if err != nil {
		t.Fatal(err)
	}
	canonical := CanonicalWorkTickets(tickets)
	next, err := ValidateWorkTicketBatch(chainID, snapshot.Anchor, snapshot.Epoch, states, canonical)
	if err != nil {
		t.Fatal(err)
	}
	root, err := WorkTicketStateRoot(chainID, snapshot.Anchor, snapshot.Epoch, next)
	if err != nil {
		t.Fatal(err)
	}
	blob, err := EncodeWorkTicketEnvelope(snapshot.Number+1, snapshot.Epoch, snapshot.Anchor, root, canonical)
	if err != nil {
		t.Fatal(err)
	}
	return blob
}

func TestWorkTicketSnapshotForkIsolationAndReconstruction(t *testing.T) {
	chainID := big.NewInt(928)
	anchor := crypto.Keccak256Hash([]byte("ticket-snapshot-anchor"))
	genesisHash := crypto.Keccak256Hash([]byte("ticket-snapshot-genesis"))
	keyA := "4f3edf983ac63ad7cdbda274f0e29202454b9f6f746b90b5d7a0a8f08c82e55b"
	keyB := "6cbed15c177e12c9e9c1e8f4b9a9e8d728b4d8f6f2e3c5f0b6a7d8e9f1a2b3c4"
	privateA, _ := crypto.HexToECDSA(keyA)
	privateB, _ := crypto.HexToECDSA(keyB)
	participantA := crypto.PubkeyToAddress(privateA.PublicKey)
	participantB := crypto.PubkeyToAddress(privateB.PublicKey)
	active := []common.Address{participantA, participantB}
	genesis, err := NewWorkTicketSnapshot(20, genesisHash, chainID, anchor, 8, active)
	if err != nil {
		t.Fatal(err)
	}
	states, _ := genesis.States(chainID)
	ticketA := signedWorkTicket(t, chainID, anchor, 8, keyA, 1, states[participantA].Previous)
	ticketB := signedWorkTicket(t, chainID, anchor, 8, keyB, 1, states[participantB].Previous)

	blobA := snapshotEnvelope(t, chainID, genesis, active, []WorkTicket{ticketA})
	blobB := snapshotEnvelope(t, chainID, genesis, active, []WorkTicket{ticketB})
	hashA := crypto.Keccak256Hash([]byte("fork-a"))
	hashB := crypto.Keccak256Hash([]byte("fork-b"))
	forkA, err := genesis.ApplyEnvelope(chainID, hashA, genesisHash, active, blobA)
	if err != nil {
		t.Fatal(err)
	}
	forkB, err := genesis.ApplyEnvelope(chainID, hashB, genesisHash, active, blobB)
	if err != nil {
		t.Fatal(err)
	}
	if forkA.Hash == forkB.Hash || forkA.StateRoot == forkB.StateRoot {
		t.Fatal("fork snapshots were not isolated")
	}
	fresh, err := NewWorkTicketSnapshot(20, genesisHash, chainID, anchor, 8, active)
	if err != nil {
		t.Fatal(err)
	}
	reconstructed, err := fresh.ApplyEnvelope(chainID, hashA, genesisHash, active, blobA)
	if err != nil {
		t.Fatal(err)
	}
	if reconstructed.StateRoot != forkA.StateRoot || !workTicketLaneEntriesEqual(reconstructed.Lanes, forkA.Lanes) {
		t.Fatal("fresh node reconstruction diverged")
	}
}

func TestWorkTicketSnapshotPreservesExitedLaneButRejectsItsTicket(t *testing.T) {
	chainID := big.NewInt(928)
	anchor := crypto.Keccak256Hash([]byte("ticket-snapshot-anchor"))
	parentHash := crypto.Keccak256Hash([]byte("ticket-snapshot-parent"))
	keyA := "4f3edf983ac63ad7cdbda274f0e29202454b9f6f746b90b5d7a0a8f08c82e55b"
	keyB := "6cbed15c177e12c9e9c1e8f4b9a9e8d728b4d8f6f2e3c5f0b6a7d8e9f1a2b3c4"
	privateA, _ := crypto.HexToECDSA(keyA)
	privateB, _ := crypto.HexToECDSA(keyB)
	participantA := crypto.PubkeyToAddress(privateA.PublicKey)
	participantB := crypto.PubkeyToAddress(privateB.PublicKey)
	all := []common.Address{participantA, participantB}
	snapshot, err := NewWorkTicketSnapshot(30, parentHash, chainID, anchor, 9, all)
	if err != nil {
		t.Fatal(err)
	}
	states, _ := snapshot.States(chainID)
	ticketB := signedWorkTicket(t, chainID, anchor, 9, keyB, 1, states[participantB].Previous)
	blob := snapshotEnvelope(t, chainID, snapshot, all, []WorkTicket{ticketB})
	if _, err := snapshot.ApplyEnvelope(chainID, crypto.Keccak256Hash([]byte("next")), parentHash, []common.Address{participantA}, blob); !errors.Is(err, ErrInactiveWorkTicketParticipant) {
		t.Fatalf("inactive participant error = %v", err)
	}
	preserved, err := snapshot.States(chainID)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := preserved[participantB]; !exists {
		t.Fatal("exited lane history was discarded")
	}
}

func TestWorkTicketSnapshotAddsNewParticipantCanonically(t *testing.T) {
	chainID := big.NewInt(928)
	anchor := crypto.Keccak256Hash([]byte("ticket-snapshot-anchor"))
	parentHash := crypto.Keccak256Hash([]byte("ticket-newcomer-parent"))
	keyA, _ := crypto.HexToECDSA("4f3edf983ac63ad7cdbda274f0e29202454b9f6f746b90b5d7a0a8f08c82e55b")
	keyB, _ := crypto.HexToECDSA("6cbed15c177e12c9e9c1e8f4b9a9e8d728b4d8f6f2e3c5f0b6a7d8e9f1a2b3c4")
	participantA := crypto.PubkeyToAddress(keyA.PublicKey)
	participantB := crypto.PubkeyToAddress(keyB.PublicKey)
	snapshot, err := NewWorkTicketSnapshot(35, parentHash, chainID, anchor, 10, []common.Address{participantA})
	if err != nil {
		t.Fatal(err)
	}
	active := []common.Address{participantB, participantA}
	blob := snapshotEnvelope(t, chainID, snapshot, active, nil)
	next, err := snapshot.ApplyEnvelope(chainID, crypto.Keccak256Hash([]byte("ticket-newcomer-next")), parentHash, active, blob)
	if err != nil {
		t.Fatal(err)
	}
	states, err := next.States(chainID)
	if err != nil {
		t.Fatal(err)
	}
	want := NewWorkTicketLaneState(chainID, anchor, 10, participantB)
	if state, exists := states[participantB]; !exists || state != want {
		t.Fatalf("newcomer lane = %+v exists=%v", state, exists)
	}
}

func TestWorkTicketSnapshotPersistenceRejectsCorruption(t *testing.T) {
	chainID := big.NewInt(928)
	anchor := crypto.Keccak256Hash([]byte("ticket-snapshot-anchor"))
	hash := crypto.Keccak256Hash([]byte("ticket-snapshot-hash"))
	key, _ := crypto.HexToECDSA("4f3edf983ac63ad7cdbda274f0e29202454b9f6f746b90b5d7a0a8f08c82e55b")
	participant := crypto.PubkeyToAddress(key.PublicKey)
	snapshot, err := NewWorkTicketSnapshot(40, hash, chainID, anchor, 10, []common.Address{participant})
	if err != nil {
		t.Fatal(err)
	}
	db := memorydb.New()
	if err := StoreWorkTicketSnapshot(db, chainID, snapshot); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadWorkTicketSnapshot(db, chainID, hash)
	if err != nil || loaded.StateRoot != snapshot.StateRoot {
		t.Fatalf("load snapshot: %v", err)
	}
	if err := db.Put(workTicketSnapshotKey(hash), []byte{0xff}); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadWorkTicketSnapshot(db, chainID, hash); !errors.Is(err, ErrInvalidWorkTicketSnapshot) {
		t.Fatalf("corruption error = %v", err)
	}
}
