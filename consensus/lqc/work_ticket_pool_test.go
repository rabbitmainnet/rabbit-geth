package lqc

import (
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

func TestWorkTicketPoolRoundRobinAndPruning(t *testing.T) {
	chainID := big.NewInt(928)
	anchor := crypto.Keccak256Hash([]byte("ticket-pool-anchor"))
	epoch := uint64(6)
	keys := []string{
		"4f3edf983ac63ad7cdbda274f0e29202454b9f6f746b90b5d7a0a8f08c82e55b",
		"6cbed15c177e12c9e9c1e8f4b9a9e8d728b4d8f6f2e3c5f0b6a7d8e9f1a2b3c4",
	}
	states := make(map[common.Address]WorkTicketLaneState)
	pool := NewWorkTicketPool()
	var all []WorkTicket
	for _, keyHex := range keys {
		key, _ := crypto.HexToECDSA(keyHex)
		participant := crypto.PubkeyToAddress(key.PublicKey)
		state := NewWorkTicketLaneState(chainID, anchor, epoch, participant)
		states[participant] = state
		first := signedWorkTicket(t, chainID, anchor, epoch, keyHex, 1, state.Previous)
		second := signedWorkTicket(t, chainID, anchor, epoch, keyHex, 2, first.Proof)
		for _, ticket := range []WorkTicket{second, first} {
			if _, err := pool.Add(chainID, ticket); err != nil {
				t.Fatalf("add ticket: %v", err)
			}
		}
		all = append(all, first, second)
	}
	if _, err := pool.Add(chainID, all[0]); !errors.Is(err, ErrWorkTicketKnown) {
		t.Fatalf("duplicate error = %v", err)
	}
	pending := pool.Pending(states, 2)
	if len(pending) != 2 || pending[0].Sequence != 1 || pending[1].Sequence != 1 || pending[0].Participant == pending[1].Participant {
		t.Fatalf("round-robin pending = %+v", pending)
	}
	pool.RemoveIncluded(pending)
	if status := pool.Status(); status.Pending != 2 || status.Participants != 2 {
		t.Fatalf("status after inclusion = %+v", status)
	}

	next, err := ValidateWorkTicketBatch(chainID, anchor, epoch, states, pending)
	if err != nil {
		t.Fatal(err)
	}
	pool.Prune(anchor, epoch, next)
	remaining := pool.Pending(next, 2)
	if len(remaining) != 2 || remaining[0].Sequence != 2 || remaining[1].Sequence != 2 {
		t.Fatalf("remaining pending = %+v", remaining)
	}
}

func TestWorkTicketPoolRejectsInvalidProofBeforeRetention(t *testing.T) {
	chainID := big.NewInt(928)
	anchor := crypto.Keccak256Hash([]byte("ticket-pool-anchor"))
	keyHex := "4f3edf983ac63ad7cdbda274f0e29202454b9f6f746b90b5d7a0a8f08c82e55b"
	key, _ := crypto.HexToECDSA(keyHex)
	state := NewWorkTicketLaneState(chainID, anchor, 7, crypto.PubkeyToAddress(key.PublicKey))
	ticket := signedWorkTicket(t, chainID, anchor, 7, keyHex, 1, state.Previous)
	ticket.Proof[0] ^= 1
	hash := WorkTicketSigningHash(chainID, ticket)
	ticket.Signature, _ = crypto.Sign(hash[:], key)
	pool := NewWorkTicketPool()
	if _, err := pool.Add(chainID, ticket); !errors.Is(err, ErrInvalidWorkTicketProof) {
		t.Fatalf("invalid proof error = %v", err)
	}
	if status := pool.Status(); status.Pending != 0 {
		t.Fatalf("invalid ticket retained: %+v", status)
	}
}

func TestWorkTicketPoolAllIsCanonicalBoundedAndCloned(t *testing.T) {
	chainID := big.NewInt(928)
	anchor := crypto.Keccak256Hash([]byte("ticket-pool-all-anchor"))
	keys := []string{
		"6cbed15c177e12c9e9c1e8f4b9a9e8d728b4d8f6f2e3c5f0b6a7d8e9f1a2b3c4",
		"4f3edf983ac63ad7cdbda274f0e29202454b9f6f746b90b5d7a0a8f08c82e55b",
	}
	pool := NewWorkTicketPool()
	for _, keyHex := range keys {
		key, _ := crypto.HexToECDSA(keyHex)
		participant := crypto.PubkeyToAddress(key.PublicKey)
		state := NewWorkTicketLaneState(chainID, anchor, 8, participant)
		ticket := signedWorkTicket(t, chainID, anchor, 8, keyHex, 1, state.Previous)
		if _, err := pool.Add(chainID, ticket); err != nil {
			t.Fatal(err)
		}
	}
	all := pool.All(1)
	if len(all) != 1 {
		t.Fatalf("all length = %d, want 1", len(all))
	}
	want := pool.All(MaxWorkTicketPoolEntries)
	if len(want) != 2 || want[0].Participant.Cmp(want[1].Participant) >= 0 {
		t.Fatalf("non-canonical all result: %+v", want)
	}
	want[0].Signature[0] ^= 1
	again := pool.All(MaxWorkTicketPoolEntries)
	if want[0].Signature[0] == again[0].Signature[0] {
		t.Fatal("All returned an aliased signature")
	}
}
