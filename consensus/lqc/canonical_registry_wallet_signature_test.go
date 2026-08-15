package lqc

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
)

func walletSignatureTestOperation(t *testing.T) (*big.Int, RegistryOperation) {
	t.Helper()

	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	chainID := big.NewInt(928)
	operation := RegistryOperation{
		Version:    RegistryProtocolVersion,
		Action:     RegistryActionRegister,
		Address:    crypto.PubkeyToAddress(key.PublicKey),
		Sequence:   1,
		ValidUntil: 100,
		ProofNonce: 0,
	}

	hash := RegistryOperationWalletSigningHash(chainID, operation)
	signature, err := crypto.Sign(hash[:], key)
	if err != nil {
		t.Fatal(err)
	}
	signature[64] += 27
	operation.Signature = signature

	return chainID, operation
}

func TestRegistryOperationWalletSignatureAccepted(t *testing.T) {
	chainID, operation := walletSignatureTestOperation(t)

	if err := ValidateRegistryOperation(chainID, 10, 1, operation); err != nil {
		t.Fatalf("wallet-signed registration rejected: %v", err)
	}

	signer, err := RecoverRegistryOperationWalletSigner(chainID, operation)
	if err != nil {
		t.Fatal(err)
	}
	if signer != operation.Address {
		t.Fatalf("wallet signer = %s, want %s", signer, operation.Address)
	}
}

func TestRegistryOperationWalletSignatureBindsChain(t *testing.T) {
	_, operation := walletSignatureTestOperation(t)

	if err := ValidateRegistryOperation(big.NewInt(929), 10, 1, operation); err == nil {
		t.Fatal("wallet signature unexpectedly valid on a different chain ID")
	}
}

func TestRegistryOperationWalletSignatureBindsOperation(t *testing.T) {
	chainID, operation := walletSignatureTestOperation(t)
	operation.ValidUntil++

	if err := ValidateRegistryOperation(chainID, 10, 1, operation); err == nil {
		t.Fatal("wallet signature unexpectedly valid after operation mutation")
	}
}

func TestRegistryOperationLegacyRawSignatureStillAccepted(t *testing.T) {
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	chainID := big.NewInt(928)
	operation := RegistryOperation{
		Version:    RegistryProtocolVersion,
		Action:     RegistryActionRegister,
		Address:    crypto.PubkeyToAddress(key.PublicKey),
		Sequence:   1,
		ValidUntil: 100,
		ProofNonce: 0,
	}

	hash := RegistryOperationSigningHash(chainID, operation)
	operation.Signature, err = crypto.Sign(hash[:], key)
	if err != nil {
		t.Fatal(err)
	}

	if err := ValidateRegistryOperation(chainID, 10, 1, operation); err != nil {
		t.Fatalf("legacy raw registry signature regressed: %v", err)
	}
}
