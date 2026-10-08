package main

import (
	"context"
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rpc"
)

// Restore only registry membership. Persistent WorkSeat ownership is unchanged.
func restoreWorkSeatRegistry(ctx context.Context, client *rpc.Client, chainID *big.Int, key *keystore.Key) (bool, error) {
	var prepared struct {
		Operation   registryOperationArgs `json:"operation"`
		SigningHash common.Hash           `json:"signingHash"`
	}
	if err := client.CallContext(ctx, &prepared, "lqc_prepareRegistryRegistration", key.Address); err != nil {
		return false, err
	}
	args := prepared.Operation
	if uint64(args.Version) != uint64(lqc.RegistryProtocolVersion) ||
		uint64(args.Action) != uint64(lqc.RegistryActionRegister) || args.Address != key.Address ||
		uint64(args.Sequence) != 1 || uint64(args.ValidUntil) == 0 {
		return false, errors.New("invalid registry restoration operation")
	}
	operation := lqc.RegistryOperation{
		Version: uint8(args.Version), Action: lqc.RegistryAction(args.Action), Address: args.Address,
		Sequence: uint64(args.Sequence), ValidUntil: uint64(args.ValidUntil), ProofNonce: uint64(args.ProofNonce),
	}
	hash := lqc.RegistryOperationWalletSigningHash(chainID, operation)
	if hash != prepared.SigningHash {
		return false, errors.New("registry restoration signing hash mismatch")
	}
	signature, err := crypto.Sign(hash[:], key.PrivateKey)
	if err != nil {
		return false, err
	}
	args.Signature = signature
	var accepted common.Hash
	if err := client.CallContext(ctx, &accepted, "lqc_submitRegistryOperation", args); err != nil {
		return false, err
	}
	if accepted == (common.Hash{}) {
		return false, errors.New("node returned empty registry restoration hash")
	}
	return true, nil
}
