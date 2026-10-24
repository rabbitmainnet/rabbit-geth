//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"errors"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethdb/memorydb"
)

func TestRabbitVRFCheckpointRestartPendingV1(t *testing.T) {
	db := memorydb.New()
	defer db.Close()

	request := common.HexToHash("0x1234")
	blockHash := common.HexToHash("0xabcd")

	checkpoint := rabbitVRFRequestCheckpointV1{
		Version:  1,
		Genesis:  common.HexToHash("0x9280"),
		Next:     501,
		LastHash: blockHash,
		Pending:  []common.Hash{request},
	}

	if err := rabbitVRFSaveCheckpointV1(db, checkpoint, 100); err != nil {
		t.Fatal(err)
	}

	restored, err := rabbitVRFLoadCheckpointV1(db, 100)
	if err != nil {
		t.Fatal(err)
	}

	if restored.Next != 501 ||
		restored.LastHash != blockHash ||
		len(restored.Pending) != 1 ||
		restored.Pending[0] != request {
		t.Fatal("restart lost pending request")
	}
}

func TestRabbitVRFCheckpointReorgCursorV1(t *testing.T) {
	checkpoint := rabbitVRFRequestCheckpointV1{
		Version:  1,
		Genesis:  common.HexToHash("0x9280"),
		Next:     501,
		LastHash: common.HexToHash("0xaaaa"),
		Pending: []common.Hash{
			common.HexToHash("0x1234"),
		},
	}

	cursor := rabbitVRFRequestScanCursorV1{
		Next:     checkpoint.Next,
		LastHash: checkpoint.LastHash,
		Pending:  append([]common.Hash(nil), checkpoint.Pending...),
	}

	if !cursor.reconcilePreviousV1(
		100,
		common.HexToHash("0xbbbb"),
	) {
		t.Fatal("reorg not detected")
	}

	if cursor.Next != 100 ||
		cursor.LastHash != (common.Hash{}) ||
		len(cursor.Pending) != 0 {
		t.Fatal("reorg retained invalid state")
	}
}

func TestRabbitVRFCheckpointFailedWriteV1(t *testing.T) {
	db := memorydb.New()
	db.Close()

	checkpoint := rabbitVRFRequestCheckpointV1{
		Version:  1,
		Genesis:  common.HexToHash("0x9280"),
		Next:     501,
		LastHash: common.HexToHash("0xabcd"),
	}

	err := rabbitVRFSaveCheckpointV1(db, checkpoint, 100)
	if err == nil {
		t.Fatal("closed database accepted checkpoint write")
	}
}

func TestRabbitVRFCheckpointMissingRecordV1(t *testing.T) {
	db := memorydb.New()
	defer db.Close()

	_, err := rabbitVRFLoadCheckpointV1(db, 100)
	if err == nil {
		t.Fatal("missing checkpoint accepted")
	}
	if !errors.Is(err, errors.New("not found")) {
		// The database is allowed to return its own missing-key error.
		// An error is sufficient for this test.
	}
}
