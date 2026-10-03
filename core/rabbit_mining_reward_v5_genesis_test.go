package core

import (
	"encoding/json"
	"math/big"
	"os"
	"testing"

	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/triedb"
)

func TestRabbitMiningRewardV5GenesisConfigUpdate(t *testing.T) {
	data, err := os.ReadFile("../networks/rabbit-testnet/genesis.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, height := range []uint64{153600, 153601, 153602} {
		t.Run(new(big.Int).SetUint64(height).String(), func(t *testing.T) {
			var original, updated Genesis
			if err := json.Unmarshal(data, &original); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(data, &updated); err != nil {
				t.Fatal(err)
			}
			original.Config.LQC.MiningRewardV5Block = 0
			updated.Config.LQC.MiningRewardV5Block = 153601

			db := rawdb.NewMemoryDatabase()
			defer db.Close()
			trie := triedb.NewDatabase(db, nil)
			defer trie.Close()

			_, hash, compat, err := SetupGenesisBlock(db, trie, &original)
			if err != nil || compat != nil {
				t.Fatalf("initial setup: compat=%v err=%v", compat, err)
			}
			if updated.ToBlock().Hash() != hash {
				t.Fatal("configuration update changed genesis block hash")
			}

			head := &types.Header{Number: new(big.Int).SetUint64(height)}
			rawdb.WriteHeader(db, head)
			rawdb.WriteHeadHeaderHash(db, head.Hash())

			_, nextHash, compat, err := SetupGenesisBlock(db, trie, &updated)
			if err != nil || nextHash != hash {
				t.Fatalf("update: hash=%s compat=%v err=%v", nextHash, compat, err)
			}
			stored := rawdb.ReadChainConfig(db, hash)
			if stored == nil || stored.LQC == nil {
				t.Fatal("stored configuration missing")
			}
			if height < 153601 {
				if compat != nil || stored.LQC.MiningRewardV5Block != 153601 {
					t.Fatalf("future activation not stored: compat=%v", compat)
				}
			} else {
				if compat == nil || compat.What != "LQC mining reward V5 fork block" {
					t.Fatalf("past activation not rejected: %v", compat)
				}
				if stored.LQC.MiningRewardV5Block != original.Config.LQC.MiningRewardV5Block {
					t.Fatal("incompatible update modified stored activation")
				}
			}
		})
	}
}
