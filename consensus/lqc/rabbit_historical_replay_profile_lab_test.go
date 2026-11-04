//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package lqc

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rlp"
)

// Real canonical headers and local side-state checkpoint. No network access,
// synthetic hasher, height exceptions, or guessed Registry transitions.
func TestRabbitHistoricalReplay370944To371072(t *testing.T) {
	read := func(name string) []byte {
		t.Helper()
		b, err := os.ReadFile("testdata/rabbit-replay-370944/" + name)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	var cfg params.ChainConfig
	if err := json.Unmarshal(read("config.json"), &cfg); err != nil {
		t.Fatal(err)
	}
	var rawHeaders []json.RawMessage
	if err := json.Unmarshal(read("headers.json"), &rawHeaders); err != nil {
		t.Fatal(err)
	}
	chain := &testHeaderChain{config: &cfg, headers: make(map[common.Hash]*types.Header)}
	var parent, anchor, target *types.Header
	for _, raw := range rawHeaders {
		var h types.Header
		var advertised struct {
			Hash common.Hash `json:"hash"`
		}
		if err := json.Unmarshal(raw, &h); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, &advertised); err != nil {
			t.Fatal(err)
		}
		if h.Hash() != advertised.Hash {
			t.Fatalf("header hash mismatch at %d", h.Number.Uint64())
		}
		if parent != nil && (h.Number.Uint64() != parent.Number.Uint64()+1 || h.ParentHash != parent.Hash()) {
			t.Fatal("broken fixture ancestry")
		}
		header := types.CopyHeader(&h)
		chain.headers[header.Hash()] = header
		chain.current = header
		parent = header
		switch header.Number.Uint64() {
		case 370944:
			anchor = header
		case 371072:
			target = header
		}
	}
	if anchor == nil || target == nil {
		t.Fatal("incomplete fixture")
	}
	db := rawdb.NewMemoryDatabase()
	defer db.Close()
	if err := db.Put(workV1EngineCheckpointKeyV1(anchor.Hash()), read("work.gob")); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile("testdata/rabbit-registry-370944.rlp")
	if err != nil {
		t.Fatal(err)
	}
	var registry RegistrySnapshot
	if err := rlp.DecodeBytes(b, &registry); err != nil {
		t.Fatal(err)
	}
	if registry.Hash != anchor.Hash() || registry.Number != 370944 {
		t.Fatal("registry anchor mismatch")
	}
	if err := StoreRegistrySnapshot(db, &registry); err != nil {
		t.Fatal(err)
	}
	engine := New(cfg.LQC, db)
	defer func() {
		if s, ok := workV1EngineLabRuntimes.Load(engine); ok {
			s.(*workV1EngineLabRuntime).close()
			workV1EngineLabRuntimes.Delete(engine)
		}
	}()
	runtime, err := engine.workV1EngineLabRuntimeAt(chain, 371072, target.Hash())
	if err != nil {
		t.Fatal(err)
	}
	if runtime.Work.Number != 371072 || runtime.Work.Hash != target.Hash() {
		t.Fatal("replay did not reach target")
	}
	// Validate each reconstructed Registry against its own committed header.
	for _, raw := range rawHeaders {
		var h types.Header
		json.Unmarshal(raw, &h)
		if h.Number.Uint64() <= 370944 {
			continue
		}
		snapshot, err := engine.registrySnapshotAt(chain, h.Number.Uint64(), h.Hash())
		if err != nil {
			t.Fatal(err)
		}
		envelope, err := DecodeLQCHeaderExtraV5(h.Extra, MaxWorkTicketsPerBlockV1)
		if err != nil {
			t.Fatal(err)
		}
		if snapshot.RegistryRoot != envelope.RegistryRoot {
			t.Fatalf("Registry mismatch at %d", h.Number.Uint64())
		}
	}
	t.Log("128 real historical transitions passed, including block 371008, with the published Rabbit RandomX profile")
}
