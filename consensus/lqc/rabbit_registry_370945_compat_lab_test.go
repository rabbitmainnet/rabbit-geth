//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package lqc

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rlp"
	"os"
	"testing"
)

func TestRabbitTestnet370945HistoricalRegistryRoot(t *testing.T) {
	data, err := os.ReadFile("testdata/rabbit-registry-370944.rlp")
	if err != nil {
		t.Fatal(err)
	}
	var snapshot RegistrySnapshot
	if err := rlp.DecodeBytes(data, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Number != 370944 || snapshot.RegistryRoot != common.HexToHash("0xb12d610a3ff385bc967a1863fb11252581e5b2ce8ef8d712363c15d73462a027") {
		t.Fatal("unexpected fixture")
	}
	registry, err := snapshot.Registry()
	if err != nil {
		t.Fatal(err)
	}
	producer := common.HexToAddress("0xCd744c0a331eC18b243aD64B689da99cc840fA50")
	missed := common.HexToAddress("0x2210697faa410100bb5e8dcfedc568b85916c2a4")
	rules := RegistrySnapshotRules{MaxMissedTurns: 3, JailBlocks: 256}
	if err := registry.MarkWorkSeatProducerHeartbeat(producer, 370945); err != nil {
		t.Fatal(err)
	}
	calculated := common.HexToHash("0x48bb5936f50f247f45e063a9c8efb797701185d2e5fae93e27b902e0eec81e1c")
	expected := common.HexToHash("0x17acf122ce53455d27fbabd76c0661edb75791d69163059f677072a9295cf057")
	if registry.Root() != calculated {
		t.Fatalf("unexpected pre-compat root %s", registry.Root())
	}
	if err := registry.ApplyWorkSeatMissedTurn(missed, 370945, rules.MaxMissedTurns, rules.JailBlocks); err != nil {
		t.Fatal(err)
	}
	if registry.Root() != expected {
		t.Fatalf("historical transition mismatch: %s", registry.Root())
	}
}
