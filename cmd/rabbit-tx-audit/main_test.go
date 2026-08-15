package main

import (
	"encoding/json"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

func TestDecodeBalanceDeltasIsolatesTransactionState(t *testing.T) {
	sender := common.HexToAddress("0x0000000000000000000000000000000000000001")
	recipient := common.HexToAddress("0x0000000000000000000000000000000000000002")
	producer := common.HexToAddress("0x0000000000000000000000000000000000000003")
	raw := json.RawMessage(`{
		"pre": {
			"0x0000000000000000000000000000000000000001": {"balance":"0x64"},
			"0x0000000000000000000000000000000000000003": {"balance":"0x1"}
		},
		"post": {
			"0x0000000000000000000000000000000000000001": {"balance":"0x59"},
			"0x0000000000000000000000000000000000000002": {"balance":"0xa"},
			"0x0000000000000000000000000000000000000003": {"balance":"0x2"}
		}
	}`)
	deltas, err := decodeBalanceDeltas(raw)
	if err != nil {
		t.Fatalf("decodeBalanceDeltas: %v", err)
	}
	wants := map[common.Address]int64{
		sender:    -11,
		recipient: 10,
		producer:  1,
	}
	for address, want := range wants {
		if got := deltas[address]; got == nil || got.Cmp(big.NewInt(want)) != 0 {
			t.Fatalf("delta %s = %v, want %d", address, got, want)
		}
	}
}

func TestDecodeBalanceDeltasRejectsMalformedTrace(t *testing.T) {
	if _, err := decodeBalanceDeltas(json.RawMessage(`{"pre":`)); err == nil {
		t.Fatal("malformed trace accepted")
	}
}

func TestParseNodeSelection(t *testing.T) {
	tests := []struct {
		name    string
		spec    string
		total   int
		want    []int
		wantErr bool
	}{
		{name: "all by default", total: 4, want: []int{1, 2, 3, 4}},
		{name: "selected nodes sorted", spec: "20, 1,3", total: 20, want: []int{1, 3, 20}},
		{name: "duplicate", spec: "1,1", total: 2, wantErr: true},
		{name: "outside range", spec: "3", total: 2, wantErr: true},
		{name: "invalid", spec: "one", total: 2, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseNodeSelection(test.spec, test.total)
			if (err != nil) != test.wantErr {
				t.Fatalf("parseNodeSelection error = %v, wantErr %t", err, test.wantErr)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("parseNodeSelection = %v, want %v", got, test.want)
			}
		})
	}
}

func TestSignTransactionWithLaboratoryKeystore(t *testing.T) {
	const (
		node     = 20
		password = "testtest"
	)
	base := t.TempDir()
	keyDir := filepath.Join(base, "node20", "keystore")
	store := keystore.NewKeyStore(keyDir, keystore.LightScryptN, keystore.LightScryptP)
	account, err := store.NewAccount(password)
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	if err := os.WriteFile(filepath.Join(base, "password.txt"), []byte(password+"\n"), 0o600); err != nil {
		t.Fatalf("write password file: %v", err)
	}

	chainID := big.NewInt(928)
	recipient := common.HexToAddress("0x0000000000000000000000000000000000000002")
	unsigned := types.NewTx(&types.DynamicFeeTx{
		ChainID:   chainID,
		Nonce:     0,
		GasTipCap: big.NewInt(2_000_000_000),
		GasFeeCap: big.NewInt(4_000_000_000),
		Gas:       21_000,
		To:        &recipient,
		Value:     big.NewInt(1),
	})
	signed, err := signTransaction(base, node, account.Address, unsigned, chainID)
	if err != nil {
		t.Fatalf("sign transaction: %v", err)
	}
	sender, err := types.Sender(types.LatestSignerForChainID(chainID), signed)
	if err != nil {
		t.Fatalf("recover sender: %v", err)
	}
	if sender != account.Address {
		t.Fatalf("sender mismatch: got %s want %s", sender, account.Address)
	}
}
