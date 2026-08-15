package eth

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/consensus/lqc"
)

func TestRegistryOperationArgsRoundTrip(t *testing.T) {
	args := RegistryOperationArgs{
		Version:    1,
		Action:     2,
		Address:    common.HexToAddress("0x1000000000000000000000000000000000000001"),
		Sequence:   3,
		ValidUntil: 200,
		ProofNonce: 9,
		Signature:  hexutil.Bytes{1, 2, 3},
	}
	operation := args.operation()
	result := registryOperationResult(big.NewInt(928), operation)
	if operation.Version != 1 || operation.Action != lqc.RegistryActionHeartbeat ||
		operation.Address != args.Address || operation.Sequence != 3 || operation.ValidUntil != 200 ||
		operation.ProofNonce != 9 || result.Hash == (common.Hash{}) {
		t.Fatalf("unexpected operation conversion: operation=%+v result=%+v", operation, result)
	}
	args.Signature[0] = 99
	if operation.Signature[0] != 1 {
		t.Fatal("operation signature aliases RPC input")
	}
}

func TestLQCRegistryAPIUnavailableWithoutBackend(t *testing.T) {
	api := NewLQCRegistryAPI(nil)
	if _, err := api.RegistryPoolStatus(); err == nil {
		t.Fatal("registry status succeeded without backend")
	}
	if _, err := api.PendingRegistryOperations(); err == nil {
		t.Fatal("pending operations succeeded without backend")
	}
	if _, err := api.RegistryParameters(); err == nil {
		t.Fatal("registry parameters succeeded without backend")
	}
	if _, err := api.RegistryParticipant(common.HexToAddress("0x1000000000000000000000000000000000000001")); err == nil {
		t.Fatal("registry participant succeeded without backend")
	}
}
