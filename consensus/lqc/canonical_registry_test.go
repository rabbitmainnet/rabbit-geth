package lqc

import (
	"crypto/ecdsa"
	"errors"
	"math/big"
	"reflect"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

func testRegistryKey(t *testing.T, encoded string) *ecdsa.PrivateKey {
	t.Helper()
	key, err := crypto.HexToECDSA(encoded)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func signRegistryOperation(t *testing.T, chainID *big.Int, key *ecdsa.PrivateKey, operation RegistryOperation) RegistryOperation {
	t.Helper()
	operation.Address = crypto.PubkeyToAddress(key.PublicKey)
	hash := RegistryOperationSigningHash(chainID, operation)
	signature, err := crypto.Sign(hash[:], key)
	if err != nil {
		t.Fatal(err)
	}
	operation.Signature = signature
	return operation
}

func registerOperation(t *testing.T, chainID *big.Int, key *ecdsa.PrivateKey, sequence, validUntil, difficulty uint64) RegistryOperation {
	t.Helper()
	operation := RegistryOperation{
		Version:    RegistryProtocolVersion,
		Action:     RegistryActionRegister,
		Sequence:   sequence,
		ValidUntil: validUntil,
	}
	operation.Address = crypto.PubkeyToAddress(key.PublicKey)
	for {
		hash := RegistryOperationSigningHash(chainID, operation)
		if LightHashMeetsDifficulty(hash, difficulty) {
			break
		}
		operation.ProofNonce++
	}
	return signRegistryOperation(t, chainID, key, operation)
}

func TestCanonicalRegistryIndependentNodesConverge(t *testing.T) {
	chainID := big.NewInt(928)
	keyA := testRegistryKey(t, "0101010101010101010101010101010101010101010101010101010101010101")
	keyB := testRegistryKey(t, "0202020202020202020202020202020202020202020202020202020202020202")
	operationA := registerOperation(t, chainID, keyA, 1, 200, 1)
	operationB := registerOperation(t, chainID, keyB, 1, 200, 1)

	nodeA := NewCanonicalRegistry()
	nodeB := NewCanonicalRegistry()
	for _, operation := range []RegistryOperation{operationA, operationB} {
		if err := nodeA.ApplyOperation(chainID, 100, 1, operation); err != nil {
			t.Fatal(err)
		}
		if err := nodeB.ApplyOperation(chainID, 100, 1, operation); err != nil {
			t.Fatal(err)
		}
	}

	if nodeA.Root() != nodeB.Root() {
		t.Fatalf("registry roots differ: %s != %s", nodeA.Root(), nodeB.Root())
	}
	if !reflect.DeepEqual(nodeA.Participants(), nodeB.Participants()) {
		t.Fatal("independent nodes derived different participant sets")
	}
}

func TestCanonicalRegistryRootDoesNotDependOnInsertionOrder(t *testing.T) {
	chainID := big.NewInt(928)
	keyA := testRegistryKey(t, "0303030303030303030303030303030303030303030303030303030303030303")
	keyB := testRegistryKey(t, "0404040404040404040404040404040404040404040404040404040404040404")
	operationA := registerOperation(t, chainID, keyA, 1, 200, 1)
	operationB := registerOperation(t, chainID, keyB, 1, 200, 1)

	left := NewCanonicalRegistry()
	right := NewCanonicalRegistry()
	for _, operation := range []RegistryOperation{operationA, operationB} {
		if err := left.ApplyOperation(chainID, 100, 1, operation); err != nil {
			t.Fatal(err)
		}
	}
	for _, operation := range []RegistryOperation{operationB, operationA} {
		if err := right.ApplyOperation(chainID, 100, 1, operation); err != nil {
			t.Fatal(err)
		}
	}

	if left.Root() != right.Root() {
		t.Fatalf("registry root depends on insertion order: %s != %s", left.Root(), right.Root())
	}
}

func TestCanonicalRegistryRejectsInvalidLightHash(t *testing.T) {
	chainID := big.NewInt(928)
	key := testRegistryKey(t, "0505050505050505050505050505050505050505050505050505050505050505")
	operation := registerOperation(t, chainID, key, 1, 200, 1)

	maxDifficulty := ^uint64(0)
	for LightHashMeetsDifficulty(RegistryOperationSigningHash(chainID, operation), maxDifficulty) {
		operation.ProofNonce++
		operation = signRegistryOperation(t, chainID, key, operation)
	}
	err := NewCanonicalRegistry().ApplyOperation(chainID, 100, maxDifficulty, operation)
	if !errors.Is(err, ErrInvalidLightHashProof) {
		t.Fatalf("error = %v, want %v", err, ErrInvalidLightHashProof)
	}
}

func TestCanonicalRegistryRejectsReplayAndWrongSigner(t *testing.T) {
	chainID := big.NewInt(928)
	keyA := testRegistryKey(t, "0606060606060606060606060606060606060606060606060606060606060606")
	keyB := testRegistryKey(t, "0707070707070707070707070707070707070707070707070707070707070707")
	operation := registerOperation(t, chainID, keyA, 1, 200, 1)
	registry := NewCanonicalRegistry()
	if err := registry.ApplyOperation(chainID, 100, 1, operation); err != nil {
		t.Fatal(err)
	}
	if err := registry.ApplyOperation(chainID, 101, 1, operation); !errors.Is(err, ErrInvalidRegistrySequence) {
		t.Fatalf("replay error = %v, want %v", err, ErrInvalidRegistrySequence)
	}

	wrongSigner := operation
	wrongSigner.Sequence = 2
	wrongSigner.Action = RegistryActionExit
	wrongSigner = signRegistryOperation(t, chainID, keyB, wrongSigner)
	wrongSigner.Address = operation.Address
	if err := registry.ApplyOperation(chainID, 102, 1, wrongSigner); !errors.Is(err, ErrInvalidRegistrySignature) {
		t.Fatalf("signature error = %v, want %v", err, ErrInvalidRegistrySignature)
	}
}

func TestCanonicalRegistryHeartbeatExitAndFreshReplay(t *testing.T) {
	chainID := big.NewInt(928)
	key := testRegistryKey(t, "0808080808080808080808080808080808080808080808080808080808080808")
	register := registerOperation(t, chainID, key, 1, 200, 1)
	heartbeat := signRegistryOperation(t, chainID, key, RegistryOperation{
		Version:    RegistryProtocolVersion,
		Action:     RegistryActionHeartbeat,
		Sequence:   2,
		ValidUntil: 220,
	})
	exit := signRegistryOperation(t, chainID, key, RegistryOperation{
		Version:    RegistryProtocolVersion,
		Action:     RegistryActionExit,
		Sequence:   3,
		ValidUntil: 240,
	})

	canonical := NewCanonicalRegistry()
	fresh := NewCanonicalRegistry()
	operations := []struct {
		block     uint64
		operation RegistryOperation
	}{{100, register}, {120, heartbeat}, {130, exit}}
	for _, item := range operations {
		if err := canonical.ApplyOperation(chainID, item.block, 1, item.operation); err != nil {
			t.Fatal(err)
		}
		if err := fresh.ApplyOperation(chainID, item.block, 1, item.operation); err != nil {
			t.Fatal(err)
		}
	}

	if canonical.Root() != fresh.Root() {
		t.Fatalf("fresh replay root differs: %s != %s", canonical.Root(), fresh.Root())
	}
	if active := canonical.ActiveParticipants(130, 64, 16); len(active) != 0 {
		t.Fatalf("active participants after exit = %d, want 0", len(active))
	}
}

func TestProducerBlockRefreshesHeartbeatWithoutChangingSequence(t *testing.T) {
	chainID := big.NewInt(928)
	key := testRegistryKey(t, "0909090909090909090909090909090909090909090909090909090909090909")
	operation := registerOperation(t, chainID, key, 1, 200, 1)
	registry := NewCanonicalRegistry()

	if err := registry.ApplyOperation(chainID, 100, 1, operation); err != nil {
		t.Fatal(err)
	}
	if err := registry.MarkProducerHeartbeat(operation.Address, 150); err != nil {
		t.Fatal(err)
	}

	participant, ok := registry.Participant(operation.Address)
	if !ok {
		t.Fatal("participant not found")
	}
	if participant.LastHeartbeat != 150 || participant.Sequence != 1 {
		t.Fatalf("participant = %+v, want heartbeat 150 and sequence 1", participant)
	}

	if len(registry.ActiveParticipants(230, 64, 16)) != 1 {
		t.Fatal("active producer disappeared before old heartbeat deadline")
	}
	if len(registry.ActiveParticipants(231, 64, 16)) != 1 {
		t.Fatal("active producer must not expire only because heartbeat time elapsed")
	}
	if len(registry.EligibleParticipants(10000, 0, 64, 16)) != 1 {
		t.Fatal("active producer must remain eligible without periodic heartbeat")
	}
}

func TestRegistryOperationExpires(t *testing.T) {
	chainID := big.NewInt(928)
	key := testRegistryKey(t, "0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a")
	operation := registerOperation(t, chainID, key, 1, 99, 1)
	err := NewCanonicalRegistry().ApplyOperation(chainID, 100, 1, operation)
	if !errors.Is(err, ErrExpiredRegistryOperation) {
		t.Fatalf("error = %v, want %v", err, ErrExpiredRegistryOperation)
	}
}

func TestRegistryOperationRejectsExcessiveFutureValidity(t *testing.T) {
	chainID := big.NewInt(928)
	key := testRegistryKey(t, "2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a")
	operation := registerOperation(t, chainID, key, 1, 100+MaxRegistryOperationLifetime+1, 1)
	err := NewCanonicalRegistry().ApplyOperation(chainID, 100, 1, operation)
	if !errors.Is(err, ErrRegistryOperationTooFar) {
		t.Fatalf("error = %v, want %v", err, ErrRegistryOperationTooFar)
	}
}

func TestCanonicalRegistryZeroRootIsStable(t *testing.T) {
	left := NewCanonicalRegistry().Root()
	right := NewCanonicalRegistry().Root()
	if left == (common.Hash{}) || left != right {
		t.Fatalf("invalid empty registry root: %s %s", left, right)
	}
}
