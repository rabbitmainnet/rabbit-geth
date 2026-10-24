package core

import (
	"encoding/json"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/triedb"
)

func TestRabbitFork320000StartupExistingDatabase(t *testing.T) {
	for _, supplied := range []bool{false, true} {
		name := "stored-config"
		if supplied {
			name = "supplied-old-genesis"
		}
		t.Run(name, func(t *testing.T) {
			g := fork320000Fixture(t)
			before, err := json.Marshal(g.Config)
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

			header := &types.Header{
				Number:     big.NewInt(317418),
				Root:       block.Root(),
				Difficulty: big.NewInt(1),
				Time:       1800000000,
			}
			rawdb.WriteHeader(db, header)
			rawdb.WriteCanonicalHash(db, header.Hash(), 317418)
			rawdb.WriteHeadHeaderHash(db, header.Hash())

			var input *Genesis
			if supplied {
				input = g
			}
			cfg, hash, compat, err := SetupGenesisBlock(db, trie, input)
			if err != nil || compat != nil {
				t.Fatalf("startup failed: err=%v compat=%v", err, compat)
			}
			if hash != block.Hash() {
				t.Fatal("genesis identity changed")
			}
			if cfg.LQC.VRFEconomicsV2Block != 320000 ||
				cfg.LQC.WorkSeatAdmissionV2Block != 320000 {
				t.Fatal("startup omitted fork schedule")
			}

			persisted := rawdb.ReadChainConfig(db, hash)
			if persisted == nil || persisted.LQC == nil ||
				persisted.LQC.VRFEconomicsV2Block != 320000 ||
				persisted.LQC.WorkSeatAdmissionV2Block != 320000 {
				t.Fatal("schedule not persisted")
			}

			persisted.LQC.VRFEconomicsV2Block = 0
			persisted.LQC.WorkSeatAdmissionV2Block = 0
			after, err := json.Marshal(persisted)
			if err != nil {
				t.Fatal(err)
			}
			if string(before) != string(after) {
				t.Fatal("unrelated historical configuration changed")
			}

			if rawdb.ReadHeadHeaderHash(db) != header.Hash() {
				t.Fatal("startup rewound or changed the head")
			}
			cfg, _, compat, err = SetupGenesisBlock(db, trie, input)
			if err != nil || compat != nil ||
				cfg.LQC.VRFEconomicsV2Block != 320000 {
				t.Fatalf("second startup failed: err=%v compat=%v", err, compat)
			}
		})
	}
}

func TestRabbitFork320000StartupRejectsLateMissingSchedule(t *testing.T) {
	g := fork320000Fixture(t)
	db := rawdb.NewMemoryDatabase()
	defer db.Close()
	trie := triedb.NewDatabase(db, nil)
	defer trie.Close()
	block, err := g.Commit(db, trie, nil)
	if err != nil {
		t.Fatal(err)
	}

	header := &types.Header{
		Number:     big.NewInt(320000),
		Root:       block.Root(),
		Difficulty: big.NewInt(1),
		Time:       1800000000,
	}
	rawdb.WriteHeader(db, header)
	rawdb.WriteCanonicalHash(db, header.Hash(), 320000)
	rawdb.WriteHeadHeaderHash(db, header.Hash())

	_, _, _, err = SetupGenesisBlock(db, trie, nil)
	if err == nil {
		t.Fatal("accepted late missing fork schedule")
	}
	if rawdb.ReadHeadHeaderHash(db) != header.Hash() {
		t.Fatal("automatic rewind occurred")
	}
	persisted := rawdb.ReadChainConfig(db, block.Hash())
	if persisted.LQC.VRFEconomicsV2Block != 0 ||
		persisted.LQC.WorkSeatAdmissionV2Block != 0 {
		t.Fatal("rejected startup persisted retroactive schedule")
	}
}
