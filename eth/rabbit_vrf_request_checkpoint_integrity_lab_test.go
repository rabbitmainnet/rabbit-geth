//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethdb/memorydb"
)

func TestRabbitVRFCheckpointPendingIntegrityV1(t *testing.T) {
	db := memorydb.New()
	defer db.Close()

	a := common.HexToHash("0x1111")
	b := common.HexToHash("0x2222")

	record := rabbitVRFRequestCheckpointV1{
		Version:  1,
		Genesis:  common.HexToHash("0x9280"),
		Next:     1001,
		LastHash: common.HexToHash("0xabcd"),
		Pending:  []common.Hash{a, b},
	}

	if err := rabbitVRFSaveCheckpointV1(db, record, 100); err != nil {
		t.Fatal(err)
	}

	recovered, err := rabbitVRFLoadCheckpointV1(db, 100)
	if err != nil {
		t.Fatal(err)
	}

	if len(recovered.Pending) != 2 ||
		recovered.Pending[0] != a ||
		recovered.Pending[1] != b {
		t.Fatal("pending requests not preserved")
	}
}

func TestRabbitVRFCheckpointRejectsZeroPendingV1(t *testing.T) {
	db := memorydb.New()
	defer db.Close()

	record := rabbitVRFRequestCheckpointV1{
		Version:  1,
		Genesis:  common.HexToHash("0x9280"),
		Next:     200,
		LastHash: common.HexToHash("0xaaaa"),
		Pending:  []common.Hash{common.Hash{}},
	}

	if err := rabbitVRFSaveCheckpointV1(db, record, 100); err == nil {
		t.Fatal("zero pending request accepted")
	}
}

func TestRabbitVRFCheckpointRejectsOldActivationV1(t *testing.T) {
	db := memorydb.New()
	defer db.Close()

	record := rabbitVRFRequestCheckpointV1{
		Version: 1,
		Genesis: common.HexToHash("0x9280"),
		Next:    99,
	}

	if err := rabbitVRFSaveCheckpointV1(db, record, 100); err == nil {
		t.Fatal("checkpoint before activation accepted")
	}
}
