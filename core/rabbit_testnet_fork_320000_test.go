package core

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
)

func fork320000Fixture(t *testing.T) *Genesis {
	t.Helper()
	encoded, err := os.ReadFile("../networks/rabbit-testnet/genesis.json")
	if err != nil {
		t.Fatal(err)
	}
	var g Genesis
	if err := json.Unmarshal(encoded, &g); err != nil {
		t.Fatal(err)
	}
	expected := common.HexToHash("0x9b71d7f2922fdf8383a4a12be5594e25938625195e0d84c05c3bd71b7bcf93f7")
	if g.ToBlock().Hash() != expected {
		t.Fatal("official genesis identity changed")
	}
	g.Config.LQC.VRFEconomicsV2Block = 0
	g.Config.LQC.WorkSeatAdmissionV2Block = 0
	g.Config.LQC.VRFAvailabilityV2Block = RabbitTestnetVRFAvailabilityV2Block
	return &g
}

func TestRabbitTestnetFork320000Schedule(t *testing.T) {
	for _, height := range []uint64{317418, 319999, 320000, 320001} {
		g := fork320000Fixture(t)
		db := rawdb.NewMemoryDatabase()
		hash := common.HexToHash("0x1234")
		rawdb.WriteHeadHeaderHash(db, hash)
		rawdb.WriteHeaderNumber(db, hash, height)
		before, _ := json.Marshal(g.Config)
		err := ConfigureRabbitTestnetFork320000(db, g.ToBlock().Hash(), g.Config)
		if height >= 320000 {
			if err == nil {
				t.Fatalf("accepted retroactive schedule at %d", height)
			}
			after, _ := json.Marshal(g.Config)
			if string(before) != string(after) {
				t.Fatal("mutated config on rejection")
			}
		} else {
			if err != nil {
				t.Fatal(err)
			}
			if g.Config.LQC.VRFEconomicsV2Block != 320000 ||
				g.Config.LQC.WorkSeatAdmissionV2Block != 320000 {
				t.Fatal("missing scheduled rules")
			}
		}
		db.Close()
	}
}

func TestRabbitTestnetFork320000ExistingAndConflict(t *testing.T) {
	g := fork320000Fixture(t)
	db := rawdb.NewMemoryDatabase()
	defer db.Close()
	g.Config.LQC.VRFEconomicsV2Block = 320000
	g.Config.LQC.WorkSeatAdmissionV2Block = 320000
	hash := common.HexToHash("0x5678")
	rawdb.WriteHeadHeaderHash(db, hash)
	rawdb.WriteHeaderNumber(db, hash, 320001)
	if err := ConfigureRabbitTestnetFork320000(db, g.ToBlock().Hash(), g.Config); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"economics", "admission"} {
		g := fork320000Fixture(t)
		if field == "economics" {
			g.Config.LQC.VRFEconomicsV2Block = 320100
		} else {
			g.Config.LQC.WorkSeatAdmissionV2Block = 320100
		}
		before, _ := json.Marshal(g.Config)
		if err := ConfigureRabbitTestnetFork320000(db, g.ToBlock().Hash(), g.Config); err == nil {
			t.Fatal("accepted conflicting schedule")
		}
		after, _ := json.Marshal(g.Config)
		if string(before) != string(after) {
			t.Fatal("mutated conflicting config")
		}
	}
}

func TestRabbitTestnetFork320000ForeignGenesis(t *testing.T) {
	g := fork320000Fixture(t)
	before, _ := json.Marshal(g.Config)
	db := rawdb.NewMemoryDatabase()
	defer db.Close()
	if err := ConfigureRabbitTestnetFork320000(db, common.HexToHash("0x9999"), g.Config); err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(g.Config)
	if string(before) != string(after) {
		t.Fatal("changed foreign network")
	}
}
