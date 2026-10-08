package main

import (
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rpc"
)

type registryRecoveryRPC struct {
	operation registryOperationArgs
	hash      common.Hash
	submitted *registryOperationArgs
	accepted  common.Hash
}

func (s *registryRecoveryRPC) RegistryParameters() registryParameters { return registryParameters{} }
func (s *registryRecoveryRPC) RegistryParticipant(common.Address) registryParticipant {
	return registryParticipant{}
}
func (s *registryRecoveryRPC) PrepareRegistryRegistration(common.Address) map[string]interface{} {
	return map[string]interface{}{"operation": s.operation, "signingHash": s.hash}
}
func (s *registryRecoveryRPC) SubmitRegistryOperation(args registryOperationArgs) common.Hash {
	s.submitted = &args
	return s.accepted
}

func TestActiveWorkSeatRestoresMissingRegistry(t *testing.T) {
	private, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	key := &keystore.Key{Address: crypto.PubkeyToAddress(private.PublicKey), PrivateKey: private}
	chainID := big.NewInt(9280)
	op := lqc.RegistryOperation{Version: lqc.RegistryProtocolVersion, Action: lqc.RegistryActionRegister,
		Address: key.Address, Sequence: 1, ValidUntil: 200, ProofNonce: 3}
	for _, tc := range []struct {
		name                               string
		badHash, badAddress, emptyAccepted bool
	}{
		{"missing registry signs registration", false, false, false},
		{"wrong signing hash", true, false, false},
		{"wrong wallet", false, true, false},
		{"empty acceptance", false, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &registryRecoveryRPC{operation: registryOperationArgs{Version: hexutil.Uint64(op.Version),
				Action: hexutil.Uint64(op.Action), Address: op.Address, Sequence: 1, ValidUntil: 200, ProofNonce: 3},
				hash: lqc.RegistryOperationWalletSigningHash(chainID, op), accepted: common.HexToHash("0x01")}
			if tc.badHash {
				s.hash = common.Hash{}
			}
			if tc.badAddress {
				s.operation.Address = common.Address{}
			}
			if tc.emptyAccepted {
				s.accepted = common.Hash{}
			}
			server := rpc.NewServer()
			if err := server.RegisterName("lqc", s); err != nil {
				t.Fatal(err)
			}
			defer server.Stop()
			client := rpc.DialInProc(server)
			defer client.Close()
			ok, err := refreshWorkSeatAvailability(context.Background(), client, chainID, key)
			if tc.badHash || tc.badAddress || tc.emptyAccepted {
				if err == nil || ok {
					t.Fatal("invalid restoration accepted")
				}
				if (tc.badHash || tc.badAddress) && s.submitted != nil {
					t.Fatal("unsafe operation signed/submitted")
				}
				return
			}
			if err != nil || !ok || s.submitted == nil {
				t.Fatalf("missing registry was not restored: %v", err)
			}
			pub, err := crypto.SigToPub(s.hash[:], s.submitted.Signature)
			if err != nil || crypto.PubkeyToAddress(*pub) != key.Address {
				t.Fatal("restoration signature does not belong to wallet")
			}
		})
	}
}
