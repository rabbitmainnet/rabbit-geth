package lqc

import (
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

func signedWorkTicket(t *testing.T, chainID *big.Int, anchor common.Hash, epoch uint64, keyHex string, sequence uint64, previous common.Hash) WorkTicket {
	t.Helper()
	key, err := crypto.HexToECDSA(keyHex)
	if err != nil {
		t.Fatal(err)
	}
	ticket := WorkTicket{
		Version:     WorkTicketProtocolVersion,
		Epoch:       epoch,
		Anchor:      anchor,
		Participant: crypto.PubkeyToAddress(key.PublicKey),
		Sequence:    sequence,
		Previous:    previous,
	}
	ticket.Proof, err = GenerateWorkTicketProof(chainID, ticket)
	if err != nil {
		t.Fatal(err)
	}
	signingHash := WorkTicketSigningHash(chainID, ticket)
	ticket.Signature, err = crypto.Sign(signingHash[:], key)
	if err != nil {
		t.Fatal(err)
	}
	return ticket
}

func TestWorkTicketPortableParametersAreFrozenInFoundation(t *testing.T) {
	if WorkTicketMemoryKiB != 8*1024 || WorkTicketIterations != 1 || WorkTicketParallelism != 1 || WorkTicketOutputBytes != 32 {
		t.Fatalf("unexpected Argon2id parameters: memory=%d iterations=%d parallelism=%d output=%d", WorkTicketMemoryKiB, WorkTicketIterations, WorkTicketParallelism, WorkTicketOutputBytes)
	}
	if MaxWorkTicketsPerBlock != 64 || WorkTicketVerificationWorkers != 2 {
		t.Fatalf("unexpected batch limits: tickets=%d workers=%d", MaxWorkTicketsPerBlock, WorkTicketVerificationWorkers)
	}
}

// This vector was produced independently with canonical RLP + Keccak-256 and
// libargon2.so.1. It detects accidental drift in either the payload or Argon2id
// parameters, rather than merely comparing the Go implementation with itself.
func TestWorkTicketIndependentKnownVector(t *testing.T) {
	chainID := big.NewInt(928)
	anchor := common.HexToHash("0x1111111111111111111111111111111111111111111111111111111111111111")
	participant := common.HexToAddress("0x2222222222222222222222222222222222222222")
	previous := common.HexToHash("0x3333333333333333333333333333333333333333333333333333333333333333")

	initial := InitialWorkTicketPrevious(chainID, anchor, 7, participant)
	wantInitial := common.HexToHash("0xadf20ccff13f4bad27660b189639edd7ed890cb4ae5f4d5fe25a58682378175c")
	if initial != wantInitial {
		t.Fatalf("initial predecessor = %s want %s", initial, wantInitial)
	}

	ticket := WorkTicket{
		Version:     WorkTicketProtocolVersion,
		Epoch:       7,
		Anchor:      anchor,
		Participant: participant,
		Sequence:    1,
		Previous:    previous,
	}
	proof, err := GenerateWorkTicketProof(chainID, ticket)
	if err != nil {
		t.Fatal(err)
	}
	wantProof := common.HexToHash("0xdc3df0290bff5e98c6d7f327720ff2d16ea4c8805f77ea6189a42c7a6156f04b")
	if proof != wantProof {
		t.Fatalf("proof = %s want %s", proof, wantProof)
	}
}

func TestWorkTicketDeterministicSignedAndSequential(t *testing.T) {
	chainID := big.NewInt(928)
	anchor := crypto.Keccak256Hash([]byte("rabbit-work-anchor"))
	keyHex := "4f3edf983ac63ad7cdbda274f0e29202454b9f6f746b90b5d7a0a8f08c82e55b"
	key, err := crypto.HexToECDSA(keyHex)
	if err != nil {
		t.Fatal(err)
	}
	participant := crypto.PubkeyToAddress(key.PublicKey)
	state := NewWorkTicketLaneState(chainID, anchor, 7, participant)
	ticket := signedWorkTicket(t, chainID, anchor, 7, keyHex, state.NextSequence, state.Previous)

	repeated, err := GenerateWorkTicketProof(chainID, ticket)
	if err != nil {
		t.Fatal(err)
	}
	if repeated != ticket.Proof {
		t.Fatalf("proof is not deterministic: %s != %s", repeated, ticket.Proof)
	}
	if err := ValidateWorkTicket(chainID, anchor, 7, state, ticket); err != nil {
		t.Fatalf("valid ticket rejected: %v", err)
	}
}

func TestWorkTicketRejectsReplayAndIdentityCopy(t *testing.T) {
	chainID := big.NewInt(928)
	anchor := crypto.Keccak256Hash([]byte("rabbit-work-anchor"))
	keyA := "4f3edf983ac63ad7cdbda274f0e29202454b9f6f746b90b5d7a0a8f08c82e55b"
	keyB := "6cbed15c177e12c9e9c1e8f4b9a9e8d728b4d8f6f2e3c5f0b6a7d8e9f1a2b3c4"
	privateA, _ := crypto.HexToECDSA(keyA)
	participantA := crypto.PubkeyToAddress(privateA.PublicKey)
	state := NewWorkTicketLaneState(chainID, anchor, 9, participantA)
	ticket := signedWorkTicket(t, chainID, anchor, 9, keyA, state.NextSequence, state.Previous)

	crossChain := ticket
	crossChainHash := WorkTicketSigningHash(big.NewInt(929), crossChain)
	crossChain.Signature, _ = crypto.Sign(crossChainHash[:], privateA)
	if err := ValidateWorkTicketCryptography(big.NewInt(929), crossChain); !errors.Is(err, ErrInvalidWorkTicketProof) {
		t.Fatalf("cross-chain replay error = %v", err)
	}
	replayed := ticket
	replayed.Anchor = crypto.Keccak256Hash([]byte("other-anchor"))
	replayedHash := WorkTicketSigningHash(chainID, replayed)
	replayed.Signature, _ = crypto.Sign(replayedHash[:], privateA)
	if err := ValidateWorkTicketCryptography(chainID, replayed); !errors.Is(err, ErrInvalidWorkTicketProof) {
		t.Fatalf("cross-anchor replay error = %v", err)
	}
	privateB, err := crypto.HexToECDSA(keyB)
	if err != nil {
		t.Fatal(err)
	}
	copied := ticket
	copied.Participant = crypto.PubkeyToAddress(privateB.PublicKey)
	signingHash := WorkTicketSigningHash(chainID, copied)
	copied.Signature, err = crypto.Sign(signingHash[:], privateB)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateWorkTicketCryptography(chainID, copied); !errors.Is(err, ErrInvalidWorkTicketProof) {
		t.Fatalf("identity copy error = %v", err)
	}
}

func TestWorkTicketRejectsWrongSequencePreviousAndSigner(t *testing.T) {
	chainID := big.NewInt(928)
	anchor := crypto.Keccak256Hash([]byte("rabbit-work-anchor"))
	keyA := "4f3edf983ac63ad7cdbda274f0e29202454b9f6f746b90b5d7a0a8f08c82e55b"
	keyB := "6cbed15c177e12c9e9c1e8f4b9a9e8d728b4d8f6f2e3c5f0b6a7d8e9f1a2b3c4"
	privateA, _ := crypto.HexToECDSA(keyA)
	participant := crypto.PubkeyToAddress(privateA.PublicKey)
	state := NewWorkTicketLaneState(chainID, anchor, 11, participant)
	ticket := signedWorkTicket(t, chainID, anchor, 11, keyA, state.NextSequence, state.Previous)

	wrongSequence := state
	wrongSequence.NextSequence++
	if err := ValidateWorkTicket(chainID, anchor, 11, wrongSequence, ticket); !errors.Is(err, ErrInvalidWorkTicketSequence) {
		t.Fatalf("sequence error = %v", err)
	}
	wrongPrevious := state
	wrongPrevious.Previous = crypto.Keccak256Hash([]byte("wrong"))
	if err := ValidateWorkTicket(chainID, anchor, 11, wrongPrevious, ticket); !errors.Is(err, ErrInvalidWorkTicketPrevious) {
		t.Fatalf("previous error = %v", err)
	}
	privateB, err := crypto.HexToECDSA(keyB)
	if err != nil {
		t.Fatal(err)
	}
	signingHash := WorkTicketSigningHash(chainID, ticket)
	ticket.Signature, err = crypto.Sign(signingHash[:], privateB)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateWorkTicketCryptography(chainID, ticket); !errors.Is(err, ErrInvalidWorkTicketSignature) {
		t.Fatalf("signer error = %v", err)
	}
}

func TestWorkTicketRejectsMalformedAndHighSSignatures(t *testing.T) {
	chainID := big.NewInt(928)
	anchor := crypto.Keccak256Hash([]byte("rabbit-work-anchor"))
	keyHex := "4f3edf983ac63ad7cdbda274f0e29202454b9f6f746b90b5d7a0a8f08c82e55b"
	key, err := crypto.HexToECDSA(keyHex)
	if err != nil {
		t.Fatal(err)
	}
	participant := crypto.PubkeyToAddress(key.PublicKey)
	state := NewWorkTicketLaneState(chainID, anchor, 12, participant)
	ticket := signedWorkTicket(t, chainID, anchor, 12, keyHex, state.NextSequence, state.Previous)

	malformed := ticket
	malformed.Signature = malformed.Signature[:64]
	if err := ValidateWorkTicketCryptography(chainID, malformed); !errors.Is(err, ErrInvalidWorkTicketSignature) {
		t.Fatalf("malformed signature error = %v", err)
	}

	highS := ticket
	highS.Signature = append([]byte(nil), ticket.Signature...)
	s := new(big.Int).SetBytes(highS.Signature[32:64])
	s.Sub(crypto.S256().Params().N, s).FillBytes(highS.Signature[32:64])
	if err := ValidateWorkTicketCryptography(chainID, highS); !errors.Is(err, ErrInvalidWorkTicketSignature) {
		t.Fatalf("high-S signature error = %v", err)
	}
}

func TestWorkTicketBatchCanonicalParallelAndAtomic(t *testing.T) {
	chainID := big.NewInt(928)
	anchor := crypto.Keccak256Hash([]byte("rabbit-work-anchor"))
	epoch := uint64(13)
	keys := []string{
		"4f3edf983ac63ad7cdbda274f0e29202454b9f6f746b90b5d7a0a8f08c82e55b",
		"6cbed15c177e12c9e9c1e8f4b9a9e8d728b4d8f6f2e3c5f0b6a7d8e9f1a2b3c4",
	}
	states := make(map[common.Address]WorkTicketLaneState)
	keysByAddress := make(map[common.Address]string)
	var tickets []WorkTicket
	for _, keyHex := range keys {
		key, err := crypto.HexToECDSA(keyHex)
		if err != nil {
			t.Fatal(err)
		}
		participant := crypto.PubkeyToAddress(key.PublicKey)
		keysByAddress[participant] = keyHex
		state := NewWorkTicketLaneState(chainID, anchor, epoch, participant)
		states[participant] = state
		first := signedWorkTicket(t, chainID, anchor, epoch, keyHex, state.NextSequence, state.Previous)
		second := signedWorkTicket(t, chainID, anchor, epoch, keyHex, state.NextSequence+1, first.Proof)
		tickets = append(tickets, first, second)
	}
	tickets = CanonicalWorkTickets(tickets)
	next, err := ValidateWorkTicketBatch(chainID, anchor, epoch, states, tickets)
	if err != nil {
		t.Fatalf("valid batch rejected: %v", err)
	}
	for address, state := range next {
		if state.NextSequence != 3 || state.Previous == states[address].Previous {
			t.Fatalf("lane did not advance: %+v", state)
		}
	}
	for address, state := range states {
		if state.NextSequence != 1 {
			t.Fatalf("input state mutated for %s: %+v", address, state)
		}
	}

	reversed := append([]WorkTicket(nil), tickets...)
	reversed[0], reversed[len(reversed)-1] = reversed[len(reversed)-1], reversed[0]
	if _, err := ValidateWorkTicketBatch(chainID, anchor, epoch, states, reversed); !errors.Is(err, ErrNonCanonicalWorkTickets) {
		t.Fatalf("non-canonical error = %v", err)
	}
	broken := append([]WorkTicket(nil), tickets...)
	for index := range broken {
		if broken[index].Sequence == 2 {
			broken[index].Proof[0] ^= 1
			key, keyErr := crypto.HexToECDSA(keysByAddress[broken[index].Participant])
			if keyErr != nil {
				t.Fatal(keyErr)
			}
			hash := WorkTicketSigningHash(chainID, broken[index])
			broken[index].Signature, keyErr = crypto.Sign(hash[:], key)
			if keyErr != nil {
				t.Fatal(keyErr)
			}
			break
		}
	}
	broken = CanonicalWorkTickets(broken)
	if _, err := ValidateWorkTicketBatch(chainID, anchor, epoch, states, broken); !errors.Is(err, ErrInvalidWorkTicketProof) {
		t.Fatalf("invalid proof error = %v", err)
	}
}

func TestWorkTicketBatchLimit(t *testing.T) {
	tickets := make([]WorkTicket, MaxWorkTicketsPerBlock+1)
	if _, err := ValidateWorkTicketBatch(big.NewInt(928), common.HexToHash("0x01"), 1, nil, tickets); !errors.Is(err, ErrTooManyWorkTickets) {
		t.Fatalf("limit error = %v", err)
	}
}
