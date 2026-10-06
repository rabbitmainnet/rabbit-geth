package core

import (
	"encoding/json"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/triedb"
	"math/big"
	"os"
	"testing"
)

func publicAvailabilityConfig(t *testing.T) (*Genesis, common.Hash) {
	t.Helper()
	data, err := os.ReadFile("../networks/rabbit-testnet/genesis.json")
	if err != nil {
		t.Fatal(err)
	}
	var g Genesis
	if err := json.Unmarshal(data, &g); err != nil {
		t.Fatal(err)
	}
	h := g.ToBlock().Hash()
	if h != common.HexToHash("0x9b71d7f2922fdf8383a4a12be5594e25938625195e0d84c05c3bd71b7bcf93f7") {
		t.Fatalf("wrong public genesis: %s", h)
	}
	return &g, h
}

func TestRabbitTestnetAvailabilityForkMigration(t *testing.T) {
	for _, tc := range []struct {
		name               string
		head, existing     uint64
		foreign, wantError bool
	}{
		{name: "fresh"}, {name: "before", head: 173056},
		{name: "at-fork-missing", head: 173057, wantError: true},
		{name: "after-fork-missing", head: 173058, wantError: true},
		{name: "already-scheduled", head: 173058, existing: 173057},
		{name: "different-schedule", existing: 173569, wantError: true},
		{name: "laboratory", foreign: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, hash := publicAvailabilityConfig(t)
			g.Config.LQC.VRFAvailabilityV2Block = tc.existing
			if tc.foreign {
				hash = common.HexToHash("0x9280998")
			}
			db := rawdb.NewMemoryDatabase()
			defer db.Close()
			if tc.head != 0 {
				hh := common.HexToHash("0x1234")
				rawdb.WriteHeadHeaderHash(db, hh)
				rawdb.WriteHeaderNumber(db, hh, tc.head)
			}
			err := ConfigureRabbitTestnetVRFAvailabilityV2(db, hash, g.Config)
			if (err != nil) != tc.wantError {
				t.Fatalf("error=%v expectedError=%v", err, tc.wantError)
			}
			want := uint64(173057)
			if tc.foreign || tc.wantError {
				want = tc.existing
			}
			if g.Config.LQC.VRFAvailabilityV2Block != want {
				t.Fatal("unexpected schedule mutation")
			}
			if !tc.foreign && !tc.wantError {
				if g.ToBlock().Hash() != hash {
					t.Fatal("fork changed genesis block hash")
				}
				if g.Config.LQC.IsVRFAvailabilityV2Preparation(173056) || !g.Config.LQC.IsVRFAvailabilityV2Preparation(173057) {
					t.Fatal("wrong activation boundary")
				}
				if err := ConfigureRabbitTestnetVRFAvailabilityV2(db, hash, g.Config); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestRabbitTestnetAvailabilityForkCompatibility(t *testing.T) {
	g, hash := publicAvailabilityConfig(t)
	var old params.ChainConfig
	data, _ := json.Marshal(g.Config)
	if err := json.Unmarshal(data, &old); err != nil {
		t.Fatal(err)
	}
	db := rawdb.NewMemoryDatabase()
	defer db.Close()
	if err := ConfigureRabbitTestnetVRFAvailabilityV2(db, hash, g.Config); err != nil {
		t.Fatal(err)
	}
	if err := old.CheckCompatible(g.Config, 173056, 0); err != nil {
		t.Fatal(err)
	}
	if err := old.CheckCompatible(g.Config, 173057, 0); err == nil {
		t.Fatal("activated fork was treated as compatible")
	}
	if g.Config.ChainID.Cmp(big.NewInt(9280)) != 0 {
		t.Fatal("chain ID changed")
	}
}

func TestRabbitTestnetAvailabilityForkGenesisPersistence(t *testing.T) {
	for _, scheme := range []string{rawdb.HashScheme, rawdb.PathScheme} {
		for _, existing := range []bool{false, true} {
			name := scheme + "/fresh"
			if existing {
				name = scheme + "/existing"
			}
			t.Run(name, func(t *testing.T) {
				g, hash := publicAvailabilityConfig(t)
				db := rawdb.NewMemoryDatabase()
				defer db.Close()
				tdb := triedb.NewDatabase(db, newDbConfig(scheme))
				defer tdb.Close()
				if existing {
					g.MustCommit(db, tdb)
				}
				config, gotHash, compat, err := SetupGenesisBlock(db, tdb, g)
				if err != nil || compat != nil {
					t.Fatalf("setup error=%v compatibility=%v", err, compat)
				}
				if gotHash != hash || config.LQC.VRFAvailabilityV2Block != 173057 {
					t.Fatal("wrong runtime schedule or genesis hash")
				}
				stored := rawdb.ReadChainConfig(db, hash)
				if stored == nil || stored.LQC.VRFAvailabilityV2Block != 173057 {
					t.Fatal("fork not persisted")
				}
				again, gotHash, compat, err := SetupGenesisBlock(db, tdb, nil)
				if err != nil || compat != nil || gotHash != hash || again.LQC.VRFAvailabilityV2Block != 173057 {
					t.Fatalf("restart lost schedule: %v %v", err, compat)
				}
			})
		}
	}
}
