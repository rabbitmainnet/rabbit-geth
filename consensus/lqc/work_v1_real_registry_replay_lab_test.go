//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package lqc

import (
	"archive/zip"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
)

func TestRealRabbitRegistryReplayFromGenesis(t *testing.T) {
	path := os.Getenv("RABBIT_REAL_REPLAY_ZIP")
	if path == "" {
		t.Skip("RABBIT_REAL_REPLAY_ZIP not configured")
	}
	archive, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()

	var genesis struct {
		Config *params.ChainConfig `json:"config"`
	}
	headers := make(map[uint64]*types.Header)
	for _, file := range archive.File {
		if file.Name != "lab/genesis.json" &&
			!strings.HasPrefix(file.Name, "headers/") {
			continue
		}
		reader, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, readErr := io.ReadAll(reader)
		reader.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		if file.Name == "lab/genesis.json" {
			if err := json.Unmarshal(data, &genesis); err != nil {
				t.Fatal(err)
			}
			continue
		}
		var batch map[string]json.RawMessage
		if err := json.Unmarshal(data, &batch); err != nil {
			t.Fatal(err)
		}
		for _, raw := range batch {
			var header types.Header
			if err := json.Unmarshal(raw, &header); err != nil {
				t.Fatal(err)
			}
			headers[header.Number.Uint64()] = &header
		}
	}
	if genesis.Config == nil || genesis.Config.LQC == nil {
		t.Fatal("missing laboratory configuration")
	}
	if len(headers) != 41130 || headers[0] == nil {
		t.Fatalf("incomplete archive: headers=%d", len(headers))
	}
	chain := &testHeaderChain{
		config:  genesis.Config,
		headers: make(map[common.Hash]*types.Header),
		current: headers[0],
	}
	for _, header := range headers {
		chain.headers[header.Hash()] = header
	}
	engine := New(genesis.Config.LQC, rawdb.NewMemoryDatabase())
	parent, err := NewBootstrapRegistrySnapshot(
		0, headers[0].Hash(), genesis.Config.LQC.BootstrapParticipants,
	)
	if err != nil {
		t.Fatal(err)
	}
	engine.rememberRegistrySnapshot(parent)

	for number := uint64(1); number <= 41129; number++ {
		header := headers[number]
		if header == nil || header.ParentHash != parent.Hash {
			t.Fatalf("broken sequence at block %d", number)
		}
		snapshot, err := engine.applyRegistrySnapshotHeaderMaybeV3(
			chain, parent, header,
		)
		if err != nil {
			t.Fatalf("real replay failed: block=%d hash=%s err=%v",
				number, header.Hash(), err)
		}
		engine.rememberRegistrySnapshot(snapshot)
		parent = snapshot
		chain.current = header
		if number%2048 == 0 {
			t.Logf("REAL REPLAY PROGRESS: %d/41129", number)
		}

	}
	t.Log("All 41129 real blocks passed registry replay")
}
