package core

import (
	"bytes"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/consensus/beacon"
	"github.com/ethereum/go-ethereum/consensus/ethash"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
)

// Two arrival orders converge, and a delayed known loser cannot undo the choice.
func TestLQCEqualHeightDelayedImport(t *testing.T) {
	for _, scheme := range []string{rawdb.HashScheme, rawdb.PathScheme} {
		for _, winnerFirst := range []bool{false, true} {
			name := "loser-first"
			if winnerFirst {
				name = "winner-first"
			}
			t.Run(scheme+"/"+name, func(t *testing.T) {
				config := *params.TestChainConfig
				engine := beacon.New(ethash.NewFaker())
				genesis := &Genesis{Config: &config, BaseFee: big.NewInt(params.InitialBaseFee)}
				db, a, _ := GenerateChainWithGenesis(genesis, engine, 1, func(i int, b *BlockGen) { b.SetExtra([]byte("a")) })
				defer db.Close()
				b, _ := GenerateChain(&config, genesis.ToBlock(), engine, db, 1, func(i int, b *BlockGen) { b.SetExtra([]byte("b")) })
				winner, loser := a[0], b[0]
				wh, lh := winner.Hash(), loser.Hash()
				if bytes.Compare(wh[:], lh[:]) > 0 {
					winner, loser = loser, winner
				}
				chain, err := NewBlockChain(rawdb.NewMemoryDatabase(), genesis, engine, DefaultConfig().WithStateScheme(scheme))
				if err != nil {
					t.Fatal(err)
				}
				defer chain.Stop()
				chain.chainConfig.LQC = &params.LQCConfig{}
				order := types.Blocks{loser, winner}
				if winnerFirst {
					order = types.Blocks{winner, loser}
				}
				for _, block := range order {
					if _, err := chain.InsertChain(types.Blocks{block}); err != nil {
						t.Fatal(err)
					}
				}
				if chain.CurrentBlock().Hash() != winner.Hash() {
					t.Fatal("arrival order changed equal-height winner")
				}
				if !chain.HasBlock(loser.Hash(), loser.NumberU64()) {
					t.Fatal("validated losing side block was not retained")
				}
				// Reproduce the stale downloader import observed at real block 57739.
				for i := 0; i < 3; i++ {
					if _, err := chain.InsertChain(types.Blocks{loser}); err != nil {
						t.Fatal(err)
					}
					if chain.CurrentBlock().Hash() != winner.Hash() {
						t.Fatal("delayed known loser reverted canonical head")
					}
				}
				longer, _ := GenerateChain(&config, loser, engine, db, 1, nil)
				if _, err := chain.InsertChain(longer); err != nil {
					t.Fatal(err)
				}
				if chain.CurrentBlock().Hash() != longer[0].Hash() {
					t.Fatal("longer validated branch was blocked")
				}
			})
		}
	}
}
