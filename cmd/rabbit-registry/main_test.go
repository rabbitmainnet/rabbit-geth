package main

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/crypto"
)

func testPrivateKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := crypto.HexToECDSA("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func testParameters(chainID *big.Int) registryParameters {
	return registryParameters{
		ChainID:              (*hexutil.Big)(new(big.Int).Set(chainID)),
		CurrentBlock:         99,
		NextBlock:            100,
		ProofDifficulty:      8,
		MaxOperationLifetime: hexutil.Uint64(lqc.MaxRegistryOperationLifetime),
		ActiveForNextBlock:   true,
	}
}

func TestBuildRegisterOperation(t *testing.T) {
	key := testPrivateKey(t)
	chainID := big.NewInt(928)
	parameters := testParameters(chainID)
	operation, attempts, err := buildOperation(context.Background(), parameters, registryParticipant{}, lqc.RegistryActionRegister, 64, key)
	if err != nil {
		t.Fatal(err)
	}
	wantAddress := crypto.PubkeyToAddress(key.PublicKey)
	if operation.Address != wantAddress || operation.Sequence != 1 || operation.ValidUntil != 163 || attempts == 0 {
		t.Fatalf("unexpected operation: %+v attempts=%d", operation, attempts)
	}
	hash := lqc.RegistryOperationSigningHash(chainID, operation)
	if !lqc.LightHashMeetsDifficulty(hash, uint64(parameters.ProofDifficulty)) {
		t.Fatal("register operation does not satisfy LightHash")
	}
	signer, err := lqc.RecoverRegistryOperationSigner(chainID, operation)
	if err != nil || signer != wantAddress {
		t.Fatalf("signer = %s err=%v, want %s", signer, err, wantAddress)
	}
	if err := lqc.ValidateRegistryOperation(chainID, uint64(parameters.NextBlock), uint64(parameters.ProofDifficulty), operation); err != nil {
		t.Fatal(err)
	}
}

func TestBuildHeartbeatAndExitSequence(t *testing.T) {
	key := testPrivateKey(t)
	parameters := testParameters(big.NewInt(928))
	participant := registryParticipant{Exists: true, Active: true, Sequence: 7}
	for _, action := range []lqc.RegistryAction{lqc.RegistryActionHeartbeat, lqc.RegistryActionExit} {
		operation, attempts, err := buildOperation(context.Background(), parameters, participant, action, 1, key)
		if err != nil {
			t.Fatalf("action %d: %v", action, err)
		}
		if operation.Sequence != 8 || operation.ValidUntil != 100 || operation.ProofNonce != 0 || attempts != 0 {
			t.Fatalf("action %d operation=%+v attempts=%d", action, operation, attempts)
		}
	}
}

func TestBuildOperationRejectsInvalidStateAndLifetime(t *testing.T) {
	key := testPrivateKey(t)
	parameters := testParameters(big.NewInt(928))
	active := registryParticipant{Exists: true, Active: true, Sequence: 1}
	if _, _, err := buildOperation(context.Background(), parameters, active, lqc.RegistryActionRegister, 1, key); err == nil {
		t.Fatal("active participant registered twice")
	}
	if _, _, err := buildOperation(context.Background(), parameters, registryParticipant{}, lqc.RegistryActionHeartbeat, 1, key); err == nil {
		t.Fatal("inactive participant sent heartbeat")
	}
	if _, _, err := buildOperation(context.Background(), parameters, registryParticipant{}, lqc.RegistryActionRegister, 0, key); err == nil {
		t.Fatal("zero lifetime accepted")
	}
	if _, _, err := buildOperation(context.Background(), parameters, registryParticipant{}, lqc.RegistryActionRegister, lqc.MaxRegistryOperationLifetime+1, key); err == nil {
		t.Fatal("excess lifetime accepted")
	}
}

func TestBuildOperationRejectsHeightOverflow(t *testing.T) {
	key := testPrivateKey(t)
	parameters := testParameters(big.NewInt(928))
	parameters.NextBlock = hexutil.Uint64(^uint64(0))
	if _, _, err := buildOperation(context.Background(), parameters, registryParticipant{}, lqc.RegistryActionRegister, 2, key); err == nil {
		t.Fatal("height overflow accepted")
	}
}

func TestFindLightHashNonceHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	operation := lqc.RegistryOperation{Version: 1, Action: lqc.RegistryActionRegister, Sequence: 1, ValidUntil: 1}
	_, _, err := findLightHashNonce(ctx, big.NewInt(928), operation, ^uint64(0))
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context canceled", err)
	}
}
