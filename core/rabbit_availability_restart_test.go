package core

import (
	"bytes"
	"encoding/json"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/triedb"
)

// Reproduce Core's repeated `init genesis.json` after public fork activation.
func TestRabbitAvailabilityOriginalGenesisRestartAfterFork(t *testing.T) {
	for _, scheme := range []string{rawdb.HashScheme, rawdb.PathScheme} {
		for _, scheduled := range []bool{true, false} {
			name := scheme + "/missing-schedule"
			if scheduled {
				name = scheme + "/persisted-schedule"
			}
			t.Run(name, func(t *testing.T) {
				original, hash := publicAvailabilityConfig(t)
				original.Config.LQC.VRFAvailabilityV2Block = 0
				db := rawdb.NewMemoryDatabase()
				defer db.Close()
				tdb := triedb.NewDatabase(db, newDbConfig(scheme))
				defer tdb.Close()
				seed, _ := publicAvailabilityConfig(t)
				seed.Config.LQC.VRFAvailabilityV2Block = 0
				if scheduled {
					_, _, compat, err := SetupGenesisBlock(db, tdb, seed)
					if err != nil || compat != nil {
						t.Fatalf("seed: %v %v", err, compat)
					}
				} else {
					seed.MustCommit(db, tdb)
				}
				head := original.ToBlock().Header()
				head.Number = big.NewInt(173058)
				head.ParentHash = hash
				head.Time = 1730580
				rawdb.WriteHeader(db, head)
				rawdb.WriteHeadHeaderHash(db, head.Hash())
				before, _ := json.Marshal(rawdb.ReadChainConfig(db, hash))
				cfg, gotHash, compat, err := SetupGenesisBlock(db, tdb, original)
				after, _ := json.Marshal(rawdb.ReadChainConfig(db, hash))
				if !scheduled {
					if err == nil {
						t.Fatal("accepted post-fork database without persisted schedule")
					}
					if !bytes.Equal(before, after) {
						t.Fatal("rejected setup changed stored configuration")
					}
					return
				}
				if err != nil || compat != nil {
					t.Fatalf("restart with original genesis: %v %v", err, compat)
				}
				if gotHash != hash || cfg.LQC.VRFAvailabilityV2Block != 173057 {
					t.Fatal("restart changed genesis or lost persisted schedule")
				}
				if original.Config.LQC.VRFAvailabilityV2Block != 0 {
					t.Fatal("mutated caller genesis")
				}
				if rawdb.ReadHeadHeaderHash(db) != head.Hash() {
					t.Fatal("rewound canonical head")
				}
				if !bytes.Equal(before, after) {
					t.Fatal("restart altered persisted configuration")
				}
			})
		}
	}
}
