package core

import (
	"encoding/json"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func loadRabbitGenesisForValidation(t *testing.T, network string) Genesis {
	t.Helper()
	path := filepath.Join("..", "networks", network, "genesis.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var genesis Genesis
	if err := json.Unmarshal(raw, &genesis); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return genesis
}

func TestRabbitGenesisAllowsOnlyMainnetAndTestnetChainIDs(t *testing.T) {
	mainnet := loadRabbitGenesisForValidation(t, "rabbit-mainnet")
	if err := validateRabbitGenesisConfig(mainnet.Config); err != nil {
		t.Fatalf("mainnet rejected: %v", err)
	}

	testnet := loadRabbitGenesisForValidation(t, "rabbit-testnet")
	if err := validateRabbitGenesisConfig(testnet.Config); err != nil {
		t.Fatalf("testnet rejected: %v", err)
	}

	invalid := *testnet.Config
	invalid.ChainID = big.NewInt(9281)
	err := validateRabbitGenesisConfig(&invalid)
	if err == nil || !strings.Contains(err.Error(), "928 or 9280") {
		t.Fatalf("unexpected invalid chainId result: %v", err)
	}
}
