package core

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/triedb"
)

func TestRabbitConsensusStartupConfigBeforeEngine(t *testing.T) {
	for _, supplied := range []bool{false, true} {
		name := "stored-config"
		if supplied {
			name = "original-genesis"
		}
		t.Run(name, func(t *testing.T) {
			g := fork320000Fixture(t)
			g.Config.LQC.VRFEconomicsV2Block = RabbitTestnetFork320000
			g.Config.LQC.WorkSeatAdmissionV2Block = RabbitTestnetFork320000
			g.Config.LQC.WorkSeatLivenessV2Block = 0

			db := rawdb.NewMemoryDatabase()
			defer db.Close()
			trie := triedb.NewDatabase(db, nil)
			defer trie.Close()
			block, err := g.Commit(db, trie, nil)
			if err != nil {
				t.Fatal(err)
			}
			head := &types.Header{
				Number:     big.NewInt(int64(RabbitTestnetWorkSeatLivenessV2Block - 1)),
				Root:       block.Root(),
				Difficulty: big.NewInt(1),
				Time:       1800000000,
			}
			rawdb.WriteHeader(db, head)
			rawdb.WriteCanonicalHash(db, head.Hash(), head.Number.Uint64())
			rawdb.WriteHeadHeaderHash(db, head.Hash())

			var input *Genesis
			if supplied {
				input = g
			}
			early, hash, err := LoadChainConfigForConsensus(db, input)
			if err != nil {
				t.Fatal(err)
			}
			if hash != block.Hash() ||
				early.LQC.WorkSeatLivenessV2Block != RabbitTestnetWorkSeatLivenessV2Block {
				t.Fatal("engine would receive a missing or incorrect schedule")
			}
			if g.Config.LQC.WorkSeatLivenessV2Block != 0 {
				t.Fatal("supplied genesis was modified")
			}
			stored := rawdb.ReadChainConfig(db, hash)
			if stored.LQC.WorkSeatLivenessV2Block != 0 {
				t.Fatal("early configuration loader wrote to the database")
			}

			final, _, compat, err := SetupGenesisBlock(db, trie, input)
			if err != nil || compat != nil {
				t.Fatalf("startup: err=%v compat=%v", err, compat)
			}
			if final.LQC.WorkSeatLivenessV2Block != early.LQC.WorkSeatLivenessV2Block {
				t.Fatal("engine and blockchain startup schedules disagree")
			}
			restarted, _, err := LoadChainConfigForConsensus(db, input)
			if err != nil {
				t.Fatal(err)
			}
			if restarted.LQC.WorkSeatLivenessV2Block != early.LQC.WorkSeatLivenessV2Block {
				t.Fatal("restart changed the schedule")
			}
			if rawdb.ReadHeadHeaderHash(db) != head.Hash() {
				t.Fatal("startup changed the head")
			}
		})
	}
}
