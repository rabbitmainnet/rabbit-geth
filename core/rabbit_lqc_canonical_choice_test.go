package core

import (
	"bytes"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/ethash"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
	"math/big"
	"testing"
)

// Real block execution/import exercises both new-state and known-block paths.
// Ethash's test engine isolates fork choice from LQC ticket generation.
func TestLQCCanonicalImportConvergesAcrossArrivalOrdersV1(t *testing.T) {
	for _, scheme := range []string{rawdb.HashScheme, rawdb.PathScheme} {
		t.Run(scheme, func(t *testing.T) {
			config := *params.AllEthashProtocolChanges
			config.ChainID = big.NewInt(9280)
			config.Ethash = nil
			config.Clique = nil
			config.LQC = &params.LQCConfig{
				CommitteeMin: 32, CommitteeMax: 128, FallbackSlots: 5, TargetBlockTimeMs: 1000,
			}
			genesis := &Genesis{Config: &config, BaseFee: big.NewInt(params.InitialBaseFee)}
			engine := ethash.NewFaker()
			_, a, _ := GenerateChainWithGenesis(genesis, engine, 1, func(_ int, b *BlockGen) { b.SetExtra([]byte("branch-a")) })
			_, b, _ := GenerateChainWithGenesis(genesis, engine, 1, func(_ int, b *BlockGen) { b.SetExtra([]byte("branch-b")) })
			winner, loser := a[0], b[0]
			ah, bh := winner.Hash(), loser.Hash()
			if bytes.Compare(ah[:], bh[:]) > 0 {
				winner, loser = loser, winner
			}
			orders := [][]*types.Block{{winner, loser}, {loser, winner}, {loser, winner, loser, winner}}
			for node, order := range orders {
				db := rawdb.NewMemoryDatabase()
				chain, err := NewBlockChain(db, genesis, ethash.NewFaker(), DefaultConfig().WithStateScheme(scheme))
				if err != nil {
					t.Fatal(err)
				}
				func() {
					defer chain.Stop()
					for _, block := range order {
						if _, err := chain.InsertChain(types.Blocks{block}); err != nil {
							t.Fatalf("node %d: %v", node, err)
						}
					}
					if chain.CurrentBlock().Hash() != winner.Hash() {
						t.Fatalf("node %d chose arrival order instead of deterministic winner", node)
					}
					if chain.GetBlockByHash(loser.Hash()) == nil {
						t.Fatal("valid side branch was not retained")
					}
					// Replay the losing known block repeatedly: it must never reverse the head.
					for i := 0; i < 3; i++ {
						if _, err := chain.InsertChain(types.Blocks{loser}); err != nil {
							t.Fatal(err)
						}
					}
					if chain.CurrentBlock().Hash() != winner.Hash() {
						t.Fatal("known losing block replaced the head")
					}
					// A longer branch must still win even if its first block lost the tie.
					_, longer, _ := GenerateChainWithGenesis(genesis, ethash.NewFaker(), 2, func(_ int, g *BlockGen) { g.SetExtra(loser.Extra()) })
					if longer[0].Hash() != loser.Hash() {
						t.Fatal("longer branch fixture parent mismatch")
					}
					if _, err := chain.InsertChain(longer); err != nil {
						t.Fatal(err)
					}
					if chain.CurrentBlock().Hash() != longer[1].Hash() {
						t.Fatal("longer branch did not become canonical")
					}
					if _, err := chain.InsertChain(types.Blocks{winner}); err != nil {
						t.Fatal(err)
					}
					if chain.CurrentBlock().Hash() != longer[1].Hash() {
						t.Fatal("shorter known branch rewound canonical head")
					}
					// A taller header with an invalid execution root must still be rejected.
					badHeader := longer[1].Header()
					badHeader.Root = common.HexToHash("0xdead")
					bad := types.NewBlockWithHeader(badHeader).WithBody(*longer[1].Body())
					if _, err := chain.InsertChain(types.Blocks{bad}); err == nil {
						t.Fatal("invalid execution root accepted")
					}
					if chain.CurrentBlock().Hash() != longer[1].Hash() {
						t.Fatal("invalid block changed canonical head")
					}
				}()
			}
		})
	}
}
