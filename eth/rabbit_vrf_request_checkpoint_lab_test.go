//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethdb/memorydb"
)

func TestRabbitVRFCheckpointPersistenceV1(t *testing.T) {
	db := memorydb.New()
	defer db.Close()

	activation := uint64(100)
	request := common.HexToHash("0x1234")

	original := rabbitVRFRequestCheckpointV1{
		Version:  1,
		Genesis:  common.HexToHash("0x9280"),
		Next:     501,
		LastHash: common.HexToHash("0xabcd"),
		Pending:  []common.Hash{request},
	}

	if err := rabbitVRFSaveCheckpointV1(db, original, activation); err != nil {
		t.Fatal(err)
	}

	restored, err := rabbitVRFLoadCheckpointV1(db, activation)
	if err != nil {
		t.Fatal(err)
	}

	if restored.Next != original.Next ||
		restored.LastHash != original.LastHash ||
		len(restored.Pending) != 1 ||
		restored.Pending[0] != request {
		t.Fatal("checkpoint recovery mismatch")
	}
}

func TestRabbitVRFCheckpointRejectsDuplicatesV1(t *testing.T) {
	db := memorydb.New()
	defer db.Close()

	request := common.HexToHash("0x1234")

	record := rabbitVRFRequestCheckpointV1{
		Version:  1,
		Genesis:  common.HexToHash("0x9280"),
		Next:     101,
		LastHash: common.HexToHash("0xabcd"),
		Pending:  []common.Hash{request, request},
	}

	if err := rabbitVRFSaveCheckpointV1(db, record, 100); err == nil {
		t.Fatal("duplicate request accepted")
	}
}

func TestRabbitVRFCheckpointRejectsCorruptionV1(t *testing.T) {
	db := memorydb.New()
	defer db.Close()

	if err := db.Put(rabbitVRFCheckpointKeyV1, []byte("invalid-json")); err != nil {
		t.Fatal(err)
	}

	if _, err := rabbitVRFLoadCheckpointV1(db, 100); err == nil {
		t.Fatal("corrupted checkpoint accepted")
	}
}

func TestRabbitVRFCheckpointRejectsMissingHashV1(t *testing.T) {
	db := memorydb.New()
	defer db.Close()

	record := rabbitVRFRequestCheckpointV1{
		Version: 1,
		Genesis: common.HexToHash("0x9280"),
		Next:    200,
	}

	if err := rabbitVRFSaveCheckpointV1(db, record, 100); err == nil {
		t.Fatal("missing canonical hash accepted")
	}
}
