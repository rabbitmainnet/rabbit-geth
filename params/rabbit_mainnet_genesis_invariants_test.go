package params

import (
	"encoding/json"
	"math/big"
	"os"
	"path/filepath"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

type rabbitMainnetGenesisFile struct {
	Config *ChainConfig `json:"config"`
	Alloc  map[string]struct {
		Balance string `json:"balance"`
	} `json:"alloc"`

	Nonce         string `json:"nonce"`
	Timestamp     string `json:"timestamp"`
	ExtraData     string `json:"extraData"`
	GasLimit      string `json:"gasLimit"`
	Difficulty    string `json:"difficulty"`
	MixHash       string `json:"mixHash"`
	Coinbase      string `json:"coinbase"`
	BaseFeePerGas string `json:"baseFeePerGas"`
}

func loadRabbitMainnetGenesis(t *testing.T) rabbitMainnetGenesisFile {
	t.Helper()

	path := filepath.Join("..", "networks", "rabbit-mainnet", "genesis.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	var genesis rabbitMainnetGenesisFile
	if err := json.Unmarshal(raw, &genesis); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	if genesis.Config == nil {
		t.Fatal("mainnet genesis config is nil")
	}
	return genesis
}

func TestRabbitMainnetGenesisFrozenInvariants(t *testing.T) {
	g := loadRabbitMainnetGenesis(t)
	c := g.Config

	if c.ChainID == nil || c.ChainID.Cmp(big.NewInt(928)) != 0 {
		t.Fatalf("chainId = %v, want 928", c.ChainID)
	}
	if err := c.CheckConfigForkOrder(); err != nil {
		t.Fatalf("invalid fork order/config: %v", err)
	}

	zero := big.NewInt(0)
	for name, block := range map[string]*big.Int{
		"homestead":      c.HomesteadBlock,
		"eip150":         c.EIP150Block,
		"eip155":         c.EIP155Block,
		"eip158":         c.EIP158Block,
		"byzantium":      c.ByzantiumBlock,
		"constantinople": c.ConstantinopleBlock,
		"petersburg":     c.PetersburgBlock,
		"istanbul":       c.IstanbulBlock,
		"berlin":         c.BerlinBlock,
		"london":         c.LondonBlock,
	} {
		if block == nil || block.Cmp(zero) != 0 {
			t.Fatalf("%s fork = %v, want block 0", name, block)
		}
	}

	if c.ShanghaiTime != nil || c.CancunTime != nil || c.PragueTime != nil ||
		c.OsakaTime != nil || c.UBTTime != nil || c.AmsterdamTime != nil ||
		c.BPO1Time != nil || c.BPO2Time != nil || c.BPO3Time != nil ||
		c.BPO4Time != nil || c.BPO5Time != nil {
		t.Fatal("future timestamp EVM fork is scheduled in frozen Rabbit mainnet genesis")
	}
	if c.Ethash != nil || c.Clique != nil {
		t.Fatal("unexpected Ethereum consensus engine configured in Rabbit mainnet")
	}
	if c.LQC == nil {
		t.Fatal("LQC config is nil")
	}

	l := c.LQC
	if l.CommitteeMin != 32 ||
		l.CommitteeMax != 128 ||
		l.CommitteeRatioBps != 3000 ||
		l.FallbackSlots != 5 ||
		l.FallbackWindowMs != 3000 ||
		l.TargetBlockTimeMs != 10000 ||
		l.EraLength != 8409600 ||
		l.ProofType != "lighthash-v1" ||
		l.ProofDifficulty != 100000 ||
		l.ActivityWindow != 128 ||
		l.EpochLength != 128 ||
		l.RegistryMode != "native" ||
		l.RegistryProtocolBlock != 1 ||
		l.BootstrapOnlyUntil != 0 ||
		l.ActivationDelay != 2 ||
		l.HeartbeatWindow != 64 ||
		l.HeartbeatGrace != 16 ||
		l.CommitteeSize != 0 ||
		l.FallbackCount != 5 ||
		l.JailBlocks != 256 ||
		l.MaxMissedTurns != 3 ||
		l.MinorSlashBps != 500 ||
		l.MajorSlashBps != 2000 {
		t.Fatalf("Rabbit mainnet LQC frozen parameters changed: %+v", *l)
	}

	if l.MinBond == nil || l.MinBond.Cmp(big.NewInt(25)) != 0 {
		t.Fatalf("minBond = %v, want 25", l.MinBond)
	}

	wantBootstrap := common.HexToAddress("0xdA5bf4A009e63D6dB4EfFaF5a2D6910f4D5BD2a0")
	if len(l.BootstrapParticipants) != 1 || l.BootstrapParticipants[0] != wantBootstrap {
		t.Fatalf("bootstrapParticipants = %v, want [%s]", l.BootstrapParticipants, wantBootstrap)
	}

	const treasury = "0x7c9AA336B2325C1e34c0d00D9b7d6aaDa61D8080"
	entry, ok := g.Alloc[treasury]
	if !ok {
		t.Fatalf("treasury allocation %s missing", treasury)
	}
	if entry.Balance != "15000000000000000000000000" {
		t.Fatalf("treasury balance = %s, want 15000000000000000000000000", entry.Balance)
	}
	if len(g.Alloc) != 1 {
		t.Fatalf("genesis alloc count = %d, want exactly 1", len(g.Alloc))
	}

	if g.Nonce != "0x0" ||
		g.Timestamp != "0x0" ||
		g.ExtraData != "0x5241424249545f4d41494e4e45545f47454e455349535f5631" ||
		g.GasLimit != "0x1c9c380" ||
		g.Difficulty != "0x1" ||
		g.MixHash != "0x0000000000000000000000000000000000000000000000000000000000000000" ||
		g.Coinbase != "0x0000000000000000000000000000000000000000" ||
		g.BaseFeePerGas != "0x3b9aca00" {
		t.Fatal("Rabbit mainnet genesis header invariant changed")
	}
}
