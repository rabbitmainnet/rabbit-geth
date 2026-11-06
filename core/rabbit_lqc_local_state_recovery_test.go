package core

import (
	"math/big"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/ethash"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/ethdb/pebble"
	"github.com/ethereum/go-ethereum/params"
)

func TestLQCLocalStateRecoveryAfterCrashV1(t *testing.T) {
	for _, scheme := range []string{rawdb.HashScheme, rawdb.PathScheme} {
		for _, fault := range []string{"valid", "missing-body", "invalid-body", "deep"} {
			name := scheme + "/" + fault
			t.Run(name, func(t *testing.T) {
				dir := t.TempDir()
				open := func() ethdb.Database {
					pdb, err := pebble.New(dir, 0, 0, "", false)
					if err != nil {
						t.Fatal(err)
					}
					db, err := rawdb.Open(pdb, rawdb.OpenOptions{Ancient: filepath.Join(dir, "ancient")})
					if err != nil {
						t.Fatal(err)
					}
					return db
				}
				config := *params.AllEthashProtocolChanges
				config.ChainID = big.NewInt(9280)
				config.Ethash = nil
				config.Clique = nil
				config.LQC = &params.LQCConfig{CommitteeMin: 32, CommitteeMax: 128, FallbackSlots: 5, TargetBlockTimeMs: 10000}
				key, err := crypto.GenerateKey()
				if err != nil {
					t.Fatal(err)
				}
				owner := crypto.PubkeyToAddress(key.PublicKey)
				recipient := common.Address{0x99}
				genesis := &Genesis{Config: &config, BaseFee: big.NewInt(params.InitialBaseFee), Alloc: types.GenesisAlloc{owner: {Balance: new(big.Int).Exp(big.NewInt(10), big.NewInt(20), nil)}}}
				engine := ethash.NewFullFaker()
				defer engine.Close()
				var tx *types.Transaction
				blockCount := 70
				if fault == "deep" {
					blockCount = 4101
				}
				_, blocks, _ := GenerateChainWithGenesis(genesis, engine, blockCount, func(i int, b *BlockGen) {
					if i == 5 {
						var err error
						tx, err = types.SignTx(types.NewTransaction(0, recipient, big.NewInt(7), 21000, big.NewInt(2*params.InitialBaseFee), nil), types.LatestSigner(&config), key)
						if err != nil {
							t.Fatal(err)
						}
						b.AddTx(tx)
					}
				})
				option := &BlockChainConfig{TrieCleanLimit: 16, TrieDirtyLimit: 16, TrieTimeLimit: 5 * time.Minute, TxLookupLimit: -1, StateScheme: scheme}
				db := open()
				chain, err := NewBlockChain(db, genesis, engine, option)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := chain.InsertChain(blocks[:4]); err != nil {
					t.Fatal(err)
				}
				if err := chain.triedb.Commit(blocks[3].Root(), false); err != nil {
					t.Fatal(err)
				}
				if fault == "deep" {
					// Persist blocks but deliberately omit their state.
					for _, block := range blocks[4:] {
						rawdb.WriteBlock(db, block)
						rawdb.WriteCanonicalHash(db, block.Hash(), block.NumberU64())
					}
					tip := blocks[len(blocks)-1]
					rawdb.WriteHeadBlockHash(db, tip.Hash())
					rawdb.WriteHeadHeaderHash(db, tip.Hash())
					rawdb.WriteHeadFastBlockHash(db, tip.Hash())
				} else if _, err := chain.InsertChain(blocks[4:]); err != nil {
					t.Fatal(err)
				}
				target := blocks[len(blocks)-1].Hash()
				// Crash without Stop's state flush, exactly as the repair suite does.
				chain.triedb.Close()
				db.Close()
				chain.stopWithoutSaving()
				db = open()
				defer db.Close()
				if fault == "missing-body" {
					rawdb.DeleteBody(db, blocks[5].Hash(), blocks[5].NumberU64())
				}
				if fault == "invalid-body" {
					rawdb.WriteBody(db, blocks[5].Hash(), blocks[5].NumberU64(), &types.Body{})
				}
				restarted, err := NewBlockChain(db, genesis, engine, option)
				if err != nil {
					t.Fatal(err)
				}
				defer restarted.Stop()
				captured, err := ReadLQCLocalStateRecoveryV1(db)
				if err != nil || captured != target {
					t.Fatalf("startup did not preserve target before repair: %s %v", captured, err)
				}
				if restarted.CurrentBlock().Number.Uint64() != 4 {
					t.Fatalf("fixture did not repair to persisted state: %d", restarted.CurrentBlock().Number.Uint64())
				}
				// A second attempt must not replace the pending target with block 4.
				again, err := PrepareLQCLocalStateRecoveryV1(db)
				if err != nil || again != target {
					t.Fatalf("retry forgot original head: %s %v", again, err)
				}
				err = restarted.RecoverLQCLocalStateV1(target)
				if fault == "missing-body" {
					if err == nil || !strings.Contains(err.Error(), "unavailable") {
						t.Fatalf("missing body accepted: %v", err)
					}
					if restarted.CurrentBlock().Number.Uint64() != 4 {
						t.Fatal("incomplete recovery changed head")
					}
					pending, _ := db.Has(lqcLocalRecoveryKeyV1)
					if !pending {
						t.Fatal("failed recovery discarded retry target")
					}
					return
				}
				if fault == "invalid-body" {
					if err == nil || !strings.Contains(err.Error(), "validation failed") {
						t.Fatalf("invalid transaction body accepted: %v", err)
					}
					if restarted.CurrentBlock().Number.Uint64() >= 6 {
						t.Fatal("invalid block advanced the chain")
					}
					pending, _ := db.Has(lqcLocalRecoveryKeyV1)
					if !pending {
						t.Fatal("invalid recovery discarded retry target")
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				if restarted.CurrentBlock().Hash() != target || !restarted.HasState(blocks[len(blocks)-1].Root()) {
					t.Fatal("original head state not restored")
				}
				statedb, err := restarted.State()
				if err != nil {
					t.Fatal(err)
				}
				if statedb.GetBalance(recipient).ToBig().Cmp(big.NewInt(7)) != 0 {
					t.Fatal("transaction balance not restored")
				}
				receipts := restarted.GetReceiptsByHash(blocks[5].Hash())
				if len(receipts) != 1 || receipts[0].TxHash != tx.Hash() || receipts[0].Status != types.ReceiptStatusSuccessful {
					t.Fatal("transaction receipt not restored")
				}
				pending, _ := db.Has(lqcLocalRecoveryKeyV1)
				if pending {
					t.Fatal("completed recovery retained pending target")
				}

				if fault == "deep" {
					return
				}

				// LQC missing-target safety regression:
				// Never discard an unknown, equal-height or future recovery marker.
				missing := common.HexToHash("0xfc51711299588c311d31133e1d661830cb9d8497581580f3ed01b344e4c15ea6")
				for _, tc := range []struct {
					name    string
					known   bool
					number  uint64
					allowed bool
				}{
					{"unknown", false, 0, false},
					{"future", true, 71, false},
					{"equal", true, 70, false},
					{"older-noncanonical", true, 5, false},
				} {
					t.Run("missing-target-"+tc.name, func(t *testing.T) {
						if tc.known {
							rawdb.WriteHeaderNumber(db, missing, tc.number)
						} else {
							rawdb.DeleteHeaderNumber(db, missing)
						}
						if err := db.Put(lqcLocalRecoveryKeyV1, missing.Bytes()); err != nil {
							t.Fatal(err)
						}
						err := restarted.RecoverLQCLocalStateV1(missing)
						if (err == nil) != tc.allowed {
							t.Fatalf("allowed=%v err=%v", tc.allowed, err)
						}
						pending, err := ReadLQCLocalStateRecoveryV1(db)
						if err != nil {
							t.Fatal(err)
						}
						if tc.allowed && pending != (common.Hash{}) {
							t.Fatal("obsolete marker retained")
						}
						if !tc.allowed && pending != missing {
							t.Fatal("unresolved recovery marker discarded")
						}
					})
				}
				rawdb.DeleteHeaderNumber(db, missing)
				if err := db.Delete(lqcLocalRecoveryKeyV1); err != nil {
					t.Fatal(err)
				}
				if err := restarted.RecoverLQCLocalStateV1(target); err != nil {
					t.Fatalf("idempotent recovery: %v", err)
				}
			})
		}
	}
}
