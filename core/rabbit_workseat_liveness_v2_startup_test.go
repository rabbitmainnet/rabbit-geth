package core

import (
	"encoding/json"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/triedb"
)

func TestRabbitWorkSeatLivenessV2StartupIntegration(t *testing.T) {
	activation := RabbitTestnetWorkSeatLivenessV2Block
	if activation == 0 {
		t.Skip("run with the isolated scheduling overlay")
	}
	for _, supplied := range []bool{false, true} {
		for _, late := range []bool{false, true} {
			name := "stored"
			if supplied {
				name = "original-genesis"
			}
			if late {
				name += "/late-missing"
			} else {
				name += "/schedule-and-restart"
			}
			t.Run(name, func(t *testing.T) {
				g := fork320000Fixture(t)
				g.Config.LQC.VRFEconomicsV2Block = RabbitTestnetFork320000
				g.Config.LQC.WorkSeatAdmissionV2Block = RabbitTestnetFork320000
				g.Config.LQC.WorkSeatLivenessV2Block = 0
				original, err := json.Marshal(g.Config)
				if err != nil {
					t.Fatal(err)
				}

				db := rawdb.NewMemoryDatabase()
				defer db.Close()
				trie := triedb.NewDatabase(db, nil)
				defer trie.Close()
				block, err := g.Commit(db, trie, nil)
				if err != nil {
					t.Fatal(err)
				}

				writeHead := func(number uint64) *types.Header {
					header := &types.Header{
						Number:     new(big.Int).SetUint64(number),
						Root:       block.Root(),
						Difficulty: big.NewInt(1),
						Time:       1800000000 + number,
					}
					rawdb.WriteHeader(db, header)
					rawdb.WriteCanonicalHash(db, header.Hash(), number)
					rawdb.WriteHeadHeaderHash(db, header.Hash())
					return header
				}
				number := activation - 1
				if late {
					number = activation + 1
				}
				head := writeHead(number)
				var input *Genesis
				if supplied {
					input = g
				}

				cfg, hash, compat, err := SetupGenesisBlock(db, trie, input)
				if late {
					if err == nil {
						t.Fatal("late missing schedule accepted")
					}
					persisted := rawdb.ReadChainConfig(db, block.Hash())
					encoded, marshalErr := json.Marshal(persisted)
					if marshalErr != nil {
						t.Fatal(marshalErr)
					}
					if string(encoded) != string(original) {
						t.Fatal("rejected startup changed persisted configuration")
					}
					if rawdb.ReadHeadHeaderHash(db) != head.Hash() {
						t.Fatal("rejected startup changed head")
					}
					return
				}
				if err != nil || compat != nil {
					t.Fatalf("initial startup: err=%v compat=%v", err, compat)
				}
				if hash != block.Hash() || cfg.LQC.WorkSeatLivenessV2Block != activation {
					t.Fatal("wrong genesis or missing activation")
				}
				persisted := rawdb.ReadChainConfig(db, hash)
				if persisted == nil || persisted.LQC == nil ||
					persisted.LQC.WorkSeatLivenessV2Block != activation {
					t.Fatal("activation not persisted")
				}
				persisted.LQC.WorkSeatLivenessV2Block = 0
				encoded, err := json.Marshal(persisted)
				if err != nil {
					t.Fatal(err)
				}
				if string(encoded) != string(original) {
					t.Fatal("historical or VRF configuration changed")
				}
				if rawdb.ReadHeadHeaderHash(db) != head.Hash() {
					t.Fatal("scheduling changed head")
				}

				head = writeHead(activation + 1)
				cfg, hash, compat, err = SetupGenesisBlock(db, trie, input)
				if err != nil || compat != nil {
					t.Fatalf("post-activation restart: err=%v compat=%v", err, compat)
				}
				if hash != block.Hash() || cfg.LQC.WorkSeatLivenessV2Block != activation {
					t.Fatal("restart lost activation")
				}
				persisted = rawdb.ReadChainConfig(db, hash)
				if persisted == nil || persisted.LQC.WorkSeatLivenessV2Block != activation {
					t.Fatal("restart lost persisted activation")
				}
				if rawdb.ReadHeadHeaderHash(db) != head.Hash() {
					t.Fatal("restart rewound head")
				}
				if g.Config.LQC.WorkSeatLivenessV2Block != 0 {
					t.Fatal("caller's original genesis was mutated")
				}
			})
		}
	}
}
