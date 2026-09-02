package params

import (
	"encoding/json"
	"math/big"
	"os"
	"path/filepath"
	"testing"
)

func loadRabbitTestnetGenesis(t *testing.T) rabbitMainnetGenesisFile {
	t.Helper()
	path := filepath.Join("..", "networks", "rabbit-testnet", "genesis.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var genesis rabbitMainnetGenesisFile
	if err := json.Unmarshal(raw, &genesis); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	if genesis.Config == nil {
		t.Fatal("testnet genesis config is nil")
	}
	return genesis
}

func TestRabbitTestnetGenesisFrozenInvariants(t *testing.T) {
	g := loadRabbitTestnetGenesis(t)
	c := g.Config
	if c.ChainID == nil || c.ChainID.Cmp(big.NewInt(9280)) != 0 {
		t.Fatalf("chainId = %v, want 9280", c.ChainID)
	}
	if err := c.CheckConfigForkOrder(); err != nil {
		t.Fatalf("invalid fork order/config: %v", err)
	}
	wantLQC := rabbitMainnetLQCImmutabilityFixture()
	wantLQC.ProofDifficulty = 10000
	wantLQC.RecoveryTimeoutMs = 120000
	if c.LQC == nil || !wantLQC.registryProtocolRulesEqual(c.LQC) {
		t.Fatalf("testnet must exercise frozen Work V2 LQC rules: %+v", c.LQC)
	}
	if len(c.LQC.BootstrapParticipants) != 0 {
		t.Fatalf("bootstrapParticipants = %v, want permissionless empty genesis", c.LQC.BootstrapParticipants)
	}

	const treasury = "0xdA5bf4A009e63D6dB4EfFaF5a2D6910f4D5BD2a0"
	entry, ok := g.Alloc[treasury]
	if !ok || entry.Balance != "15000000000000000000000000" {
		t.Fatalf("testnet treasury allocation invalid: present=%v balance=%s", ok, entry.Balance)
	}
	if len(g.Alloc) != 1 {
		t.Fatalf("genesis alloc count = %d, want exactly 1", len(g.Alloc))
	}
	if g.Nonce != "0x0" ||
		g.Timestamp != "0x0" ||
		g.ExtraData != "0x5241424249545f544553544e45545f47454e455349535f5632" ||
		g.GasLimit != "0x1c9c380" ||
		g.Difficulty != "0x1" ||
		g.MixHash != "0x0000000000000000000000000000000000000000000000000000000000000000" ||
		g.Coinbase != "0x0000000000000000000000000000000000000000" ||
		g.BaseFeePerGas != "0x3b9aca00" {
		t.Fatal("Rabbit testnet genesis header invariant changed")
	}
}
