package core

import (
	"context"
	"github.com/ethereum/go-ethereum/consensus/ethash"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
	"math/big"
	"testing"
	"time"
)

func TestLQCLocalAtomicInsertRejectsRemoteAdvanceV1(t *testing.T) {
	for _, scheme := range []string{rawdb.HashScheme, rawdb.PathScheme} {
		t.Run(scheme, func(t *testing.T) {
			config := *params.AllEthashProtocolChanges
			config.ChainID = big.NewInt(9280)
			config.Ethash = nil
			config.Clique = nil
			config.LQC = &params.LQCConfig{CommitteeMin: 32, CommitteeMax: 128, FallbackSlots: 5, TargetBlockTimeMs: 1000}
			genesis := &Genesis{Config: &config, BaseFee: big.NewInt(params.InitialBaseFee)}
			_, local, _ := GenerateChainWithGenesis(genesis, ethash.NewFaker(), 1, func(_ int, b *BlockGen) { b.SetExtra([]byte("local-built")) })
			_, remote, _ := GenerateChainWithGenesis(genesis, ethash.NewFaker(), 1, func(_ int, b *BlockGen) { b.SetExtra([]byte("remote-in-flight")) })
			for _, race := range []bool{false, true} {
				chain, err := NewBlockChain(rawdb.NewMemoryDatabase(), genesis, ethash.NewFaker(), DefaultConfig().WithStateScheme(scheme))
				if err != nil {
					t.Fatal(err)
				}
				func() {
					defer chain.Stop()
					parent := chain.CurrentBlock().Hash()
					if !race {
						inserted, err := chain.InsertLQCBlockIfParentCurrent(parent, local[0])
						if err != nil || !inserted {
							t.Fatalf("current local child rejected: %t %v", inserted, err)
						}
						return
					}
					// Reproduce a remote import already holding chainmu when the producer
					// has checked the old head and attempts to insert its built sibling.
					if !chain.chainmu.TryLock() {
						t.Fatal("lock unavailable")
					}
					type result struct {
						inserted bool
						err      error
					}
					done := make(chan result, 1)
					go func() { ok, err := chain.InsertLQCBlockIfParentCurrent(parent, local[0]); done <- result{ok, err} }()
					_, _, err = chain.insertChain(context.Background(), types.Blocks{remote[0]}, true, false)
					chain.chainmu.Unlock()
					if err != nil {
						t.Fatal(err)
					}
					select {
					case got := <-done:
						if got.err != nil || got.inserted {
							t.Fatalf("stale local payload imported: %t %v", got.inserted, got.err)
						}
					case <-time.After(5 * time.Second):
						t.Fatal("local insertion did not finish")
					}
					if chain.CurrentBlock().Hash() != remote[0].Hash() {
						t.Fatal("stale local payload reorged remote head")
					}
					if chain.GetBlockByHash(local[0].Hash()) != nil {
						t.Fatal("stale local payload entered the block database")
					}
				}()
			}
		})
	}
}
