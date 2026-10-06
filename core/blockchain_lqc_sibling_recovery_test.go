package core

import (
	"math/big"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/beacon"
	"github.com/ethereum/go-ethereum/consensus/ethash"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/ethdb/pebble"
	"github.com/ethereum/go-ethereum/params"
)

// Exercise the importer with deterministic execution. Real LQC headers remain
// subject to the supplied consensus engine's validation in production.
func TestLQCSiblingPrunedParentRecovery(t *testing.T) {
	for _, scheme := range []string{rawdb.HashScheme, rawdb.PathScheme} {
		for _, name := range []string{"valid-sibling", "forged-state-root", "legacy-guard"} {
			t.Run(scheme+"/"+name, func(t *testing.T) {
				invalid := name == "forged-state-root"
				config := *params.TestChainConfig
				engine := beacon.New(ethash.NewFaker())
				genesis := &Genesis{Config: &config, BaseFee: big.NewInt(params.InitialBaseFee)}
				generatorDB, canonical, _ := GenerateChainWithGenesis(
					genesis, engine, 2*state.TriesInMemory, nil,
				)
				defer generatorDB.Close()
				datadir := t.TempDir()
				disk, err := pebble.New(datadir, 0, 0, "", false)
				if err != nil {
					t.Fatal(err)
				}
				database, err := rawdb.Open(disk, rawdb.OpenOptions{Ancient: filepath.Join(datadir, "ancient")})
				if err != nil {
					disk.Close()
					t.Fatal(err)
				}
				defer database.Close()
				chain, err := NewBlockChain(database, genesis, engine, DefaultConfig().WithStateScheme(scheme))
				if err != nil {
					t.Fatal(err)
				}
				defer chain.Stop()
				if _, err := chain.InsertChain(canonical); err != nil {
					t.Fatal(err)
				}
				states := state.TriesInMemory
				if scheme == rawdb.PathScheme {
					states++
				}
				parentIndex := len(canonical) - states - 1
				parent := canonical[parentIndex]
				if chain.HasState(parent.Root()) {
					t.Fatal("fixture requires a pruned common parent")
				}
				fork, _ := GenerateChain(genesis.Config, parent, engine, generatorDB,
					2*state.TriesInMemory, func(i int, block *BlockGen) {
						block.SetExtra([]byte("different authenticated header"))
						if invalid && i == 0 {
							block.SetCoinbase(common.Address{2})
						}
					})
				localSibling := canonical[parentIndex+1]
				if invalid {
					if fork[0].Root() == localSibling.Root() {
						t.Fatal("fixture requires different actual execution state")
					}
					header := fork[0].Header()
					header.Root = localSibling.Root()
					fork[0] = fork[0].WithSeal(header)
				}
				if fork[0].Root() != localSibling.Root() || fork[0].Hash() == localSibling.Hash() {
					t.Fatal("fixture requires different hashes with identical advertised roots")
				}
				// Activate only the importer recovery policy after generation. The
				// execution engine is unchanged, so its state-root checks are real.
				if name == "legacy-guard" {
					if _, err := chain.InsertChain(fork[:1]); err == nil || !strings.Contains(err.Error(), "sidechain ghost-state attack") {
						t.Fatalf("legacy ghost-state protection changed: %v", err)
					}
					return
				}
				chain.chainConfig.LQC = &params.LQCConfig{}
				if chain.chainConfig.LQC == nil {
					t.Fatal("fixture did not enable LQC policy")
				}
				if invalid {
					head := chain.CurrentBlock().Hash()
					if _, err := chain.InsertChain(fork[:1]); err == nil {
						t.Fatal("forged sibling state root was accepted")
					}
					if chain.CurrentBlock().Hash() != head {
						t.Fatal("invalid sibling changed the canonical head")
					}
					if chain.HasBlock(fork[0].Hash(), fork[0].NumberU64()) {
						t.Fatal("invalid sibling was persisted as a valid block")
					}
					return
				}
				if n, err := chain.InsertChain(fork); err != nil {
					t.Fatalf("sibling recovery failed at index %d: %v", n, err)
				}
				if chain.CurrentBlock().Hash() != fork[len(fork)-1].Hash() {
					t.Fatal("longer fully executed branch did not become canonical")
				}
				if !chain.HasBlockAndState(fork[0].Hash(), fork[0].NumberU64()) {
					// Historical states may be pruned again by the longer import;
					// its receipts still prove that normal block execution ran.
					if len(rawdb.ReadReceiptsRLP(chain.db, fork[0].Hash(), fork[0].NumberU64())) == 0 {
						t.Fatal("reexecuted sibling receipts missing")
					}
				}
			})
		}
	}
}
