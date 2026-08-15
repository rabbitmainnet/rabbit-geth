package lqc

import (
	"bytes"
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
)

func TestWorkTicketEnvelopeCanonicalRoundTrip(t *testing.T) {
	chainID := big.NewInt(928)
	anchor := crypto.Keccak256Hash([]byte("ticket-codec-anchor"))
	keyA := "4f3edf983ac63ad7cdbda274f0e29202454b9f6f746b90b5d7a0a8f08c82e55b"
	keyB := "6cbed15c177e12c9e9c1e8f4b9a9e8d728b4d8f6f2e3c5f0b6a7d8e9f1a2b3c4"
	privateA, _ := crypto.HexToECDSA(keyA)
	privateB, _ := crypto.HexToECDSA(keyB)
	stateA := NewWorkTicketLaneState(chainID, anchor, 3, crypto.PubkeyToAddress(privateA.PublicKey))
	stateB := NewWorkTicketLaneState(chainID, anchor, 3, crypto.PubkeyToAddress(privateB.PublicKey))
	ticketA := signedWorkTicket(t, chainID, anchor, 3, keyA, stateA.NextSequence, stateA.Previous)
	ticketB := signedWorkTicket(t, chainID, anchor, 3, keyB, stateB.NextSequence, stateB.Previous)
	states := map[common.Address]WorkTicketLaneState{ticketA.Participant: stateA, ticketB.Participant: stateB}
	next, err := ValidateWorkTicketBatch(chainID, anchor, 3, states, CanonicalWorkTickets([]WorkTicket{ticketA, ticketB}))
	if err != nil {
		t.Fatal(err)
	}
	root, err := WorkTicketStateRoot(chainID, anchor, 3, next)
	if err != nil {
		t.Fatal(err)
	}
	blob, err := EncodeWorkTicketEnvelope(10, 3, anchor, root, []WorkTicket{ticketB, ticketA})
	if err != nil {
		t.Fatal(err)
	}
	envelope, decodedNext, err := ValidateWorkTicketEnvelope(chainID, 10, 3, anchor, states, blob)
	if err != nil {
		t.Fatalf("valid envelope rejected: %v", err)
	}
	if len(envelope.Tickets) != 2 || envelope.StateRoot != root || len(decodedNext) != 2 {
		t.Fatalf("unexpected envelope: %+v", envelope)
	}
	falseRoot := crypto.Keccak256Hash([]byte("false-root"))
	falseBlob, err := EncodeWorkTicketEnvelope(10, 3, anchor, falseRoot, envelope.Tickets)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := ValidateWorkTicketEnvelope(chainID, 10, 3, anchor, states, falseBlob); !errors.Is(err, ErrInvalidWorkTicketStateRoot) {
		t.Fatalf("false root error = %v", err)
	}
	reencoded, err := EncodeWorkTicketEnvelope(envelope.BlockNumber, envelope.Epoch, envelope.Anchor, envelope.StateRoot, envelope.Tickets)
	if err != nil || !bytes.Equal(reencoded, blob) {
		t.Fatal("envelope is not canonically stable")
	}
}

func encodeRawWorkTicketEnvelope(t *testing.T, envelope WorkTicketEnvelope) []byte {
	t.Helper()
	payload, err := rlp.EncodeToBytes(envelope)
	if err != nil {
		t.Fatal(err)
	}
	return append(append([]byte(nil), workTicketEnvelopeMagic...), payload...)
}

func TestWorkTicketEnvelopeRejectsMalformedAndNonCanonical(t *testing.T) {
	chainID := big.NewInt(928)
	anchor := crypto.Keccak256Hash([]byte("ticket-codec-anchor"))
	keyHex := "4f3edf983ac63ad7cdbda274f0e29202454b9f6f746b90b5d7a0a8f08c82e55b"
	key, _ := crypto.HexToECDSA(keyHex)
	state := NewWorkTicketLaneState(chainID, anchor, 4, crypto.PubkeyToAddress(key.PublicKey))
	first := signedWorkTicket(t, chainID, anchor, 4, keyHex, 1, state.Previous)
	second := signedWorkTicket(t, chainID, anchor, 4, keyHex, 2, first.Proof)
	root := crypto.Keccak256Hash([]byte("nonzero-root"))

	nonCanonical := encodeRawWorkTicketEnvelope(t, WorkTicketEnvelope{
		Version: WorkTicketEnvelopeVersion, BlockNumber: 11, Epoch: 4, Anchor: anchor, StateRoot: root,
		Tickets: []WorkTicket{second, first},
	})
	if _, err := DecodeWorkTicketEnvelope(nonCanonical); !errors.Is(err, ErrNonCanonicalWorkTickets) {
		t.Fatalf("non-canonical error = %v", err)
	}
	duplicate := first
	duplicate.Proof[0] ^= 1
	duplicateBlob := encodeRawWorkTicketEnvelope(t, WorkTicketEnvelope{
		Version: WorkTicketEnvelopeVersion, BlockNumber: 11, Epoch: 4, Anchor: anchor, StateRoot: root,
		Tickets: CanonicalWorkTickets([]WorkTicket{first, duplicate}),
	})
	if _, err := DecodeWorkTicketEnvelope(duplicateBlob); !errors.Is(err, ErrDuplicateWorkTicket) {
		t.Fatalf("duplicate error = %v", err)
	}
	unsupported := encodeRawWorkTicketEnvelope(t, WorkTicketEnvelope{
		Version: WorkTicketEnvelopeVersion + 1, BlockNumber: 11, Epoch: 4, Anchor: anchor, StateRoot: root,
	})
	if _, err := DecodeWorkTicketEnvelope(unsupported); !errors.Is(err, ErrUnsupportedWorkTicketEnvelope) {
		t.Fatalf("version error = %v", err)
	}
	if _, err := DecodeWorkTicketEnvelope(make([]byte, MaxWorkTicketEnvelopeSize+1)); !errors.Is(err, ErrInvalidWorkTicketEnvelope) {
		t.Fatalf("oversize error = %v", err)
	}
}

func TestWorkTicketEnvelopeBoundAccepts64AndRejects65(t *testing.T) {
	chainID := big.NewInt(928)
	anchor := crypto.Keccak256Hash([]byte("ticket-codec-anchor"))
	keyHex := "4f3edf983ac63ad7cdbda274f0e29202454b9f6f746b90b5d7a0a8f08c82e55b"
	key, _ := crypto.HexToECDSA(keyHex)
	state := NewWorkTicketLaneState(chainID, anchor, 5, crypto.PubkeyToAddress(key.PublicKey))
	base := signedWorkTicket(t, chainID, anchor, 5, keyHex, 1, state.Previous)
	tickets := make([]WorkTicket, MaxWorkTicketsPerBlock)
	for index := range tickets {
		tickets[index] = cloneWorkTicket(base)
		tickets[index].Sequence = uint64(index + 1)
	}
	root := crypto.Keccak256Hash([]byte("bounded-root"))
	blob, err := EncodeWorkTicketEnvelope(12, 5, anchor, root, tickets)
	if err != nil {
		t.Fatalf("64-ticket envelope rejected: %v", err)
	}
	if len(blob) > MaxWorkTicketEnvelopeSize {
		t.Fatalf("envelope size = %d", len(blob))
	}
	tickets = append(tickets, cloneWorkTicket(base))
	tickets[len(tickets)-1].Sequence = uint64(len(tickets))
	if _, err := EncodeWorkTicketEnvelope(12, 5, anchor, root, tickets); !errors.Is(err, ErrTooManyWorkTickets) {
		t.Fatalf("65-ticket error = %v", err)
	}
}
