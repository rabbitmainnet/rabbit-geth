package lqc

import (
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestRegistryOperationPoolBoundsDuplicatesReplacementAndExpiry(t *testing.T) {
	chainID := big.NewInt(928)
	key := testRegistryKey(t, "2626262626262626262626262626262626262626262626262626262626262626")
	pool := NewRegistryOperationPool()
	register := registerOperation(t, chainID, key, 1, 20, 1)
	hash, err := pool.Add(chainID, register)
	if err != nil || !pool.Has(hash) {
		t.Fatalf("add register: hash=%s err=%v", hash, err)
	}
	if _, err := pool.Add(chainID, register); !errors.Is(err, ErrRegistryOperationKnown) {
		t.Fatalf("duplicate error = %v, want %v", err, ErrRegistryOperationKnown)
	}
	conflict := register
	conflict.Action = RegistryActionExit
	conflict.Signature = append([]byte(nil), register.Signature...)
	if _, err := pool.Add(chainID, conflict); !errors.Is(err, ErrRegistryOperationPending) {
		t.Fatalf("same-sequence conflict = %v, want %v", err, ErrRegistryOperationPending)
	}
	replacement := register
	replacement.Sequence = 2
	replacement.Action = RegistryActionHeartbeat
	replacement = signRegistryOperation(t, chainID, key, replacement)
	replacementHash, err := pool.Add(chainID, replacement)
	if err != nil {
		t.Fatal(err)
	}
	if pool.Has(hash) || !pool.Has(replacementHash) || pool.Status().Pending != 1 {
		t.Fatal("higher-sequence replacement was not atomic")
	}
	pool.PruneExpired(21)
	if pool.Status().Pending != 0 {
		t.Fatal("expired operation remained in pool")
	}
}

func TestRegistryOperationPoolCanonicalPendingOrder(t *testing.T) {
	chainID := big.NewInt(928)
	pool := NewRegistryOperationPool()
	keys := []string{
		"2929292929292929292929292929292929292929292929292929292929292929",
		"2727272727272727272727272727272727272727272727272727272727272727",
		"2828282828282828282828282828282828282828282828282828282828282828",
	}
	for _, encoded := range keys {
		operation := registerOperation(t, chainID, testRegistryKey(t, encoded), 1, 20, 1)
		if _, err := pool.Add(chainID, operation); err != nil {
			t.Fatal(err)
		}
	}
	pending := pool.Pending(1)
	if len(pending) != len(keys) {
		t.Fatalf("pending = %d, want %d", len(pending), len(keys))
	}
	for index := 1; index < len(pending); index++ {
		if pending[index-1].Address.Cmp(pending[index].Address) >= 0 {
			t.Fatal("pending operations are not canonically ordered")
		}
	}
}

func TestRegistryOperationPoolCapacity(t *testing.T) {
	pool := NewRegistryOperationPool()
	pool.byHash = make(map[common.Hash]RegistryOperation, MaxRegistryPoolOperations)
	pool.byAddress = make(map[common.Address]common.Hash, MaxRegistryPoolOperations)
	for index := 0; index < MaxRegistryPoolOperations; index++ {
		address := common.BigToAddress(big.NewInt(int64(index + 1)))
		hash := common.BigToHash(big.NewInt(int64(index + 1)))
		pool.byHash[hash] = RegistryOperation{Address: address, Sequence: 1, ValidUntil: 20}
		pool.byAddress[address] = hash
	}
	operation := RegistryOperation{Address: common.HexToAddress("0xffffffffffffffffffffffffffffffffffffffff"), Sequence: 1}
	if _, err := pool.Add(big.NewInt(928), operation); !errors.Is(err, ErrRegistryPoolFull) {
		t.Fatalf("capacity error = %v, want %v", err, ErrRegistryPoolFull)
	}
}
