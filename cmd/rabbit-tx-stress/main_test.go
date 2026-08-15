package main

import (
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestRejected(t *testing.T) {
	if got := rejected("invalid", errors.New("nonce too low"), "nonce too low"); got.Status != "PASS" {
		t.Fatalf("rejected transaction status = %s", got.Status)
	}
	if got := rejected("accepted", nil, "nonce too low"); got.Status != "FAIL" {
		t.Fatalf("accepted invalid transaction status = %s", got.Status)
	}
	if got := rejected("transport", errors.New("context deadline exceeded"), "nonce too low"); got.Status != "FAIL" {
		t.Fatalf("unexpected error status = %s", got.Status)
	}
}

func TestExpectedDeltaAndRoles(t *testing.T) {
	address := common.HexToAddress("0x0000000000000000000000000000000000000001")
	values := make(map[common.Address]*big.Int)
	addDelta(values, address, big.NewInt(10))
	addDelta(values, address, big.NewInt(-3))
	if values[address].Cmp(big.NewInt(7)) != 0 {
		t.Fatalf("delta = %s, want 7", values[address])
	}
	roles := make(map[common.Address]map[string]bool)
	addRole(roles, address, "sender")
	addRole(roles, address, "producer")
	addRole(roles, address, "sender")
	if got := roleString(roles[address]); got != "producer+sender" {
		t.Fatalf("roles = %q", got)
	}
}
