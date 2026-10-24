//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethdb/memorydb"
)

func TestRabbitVRFCheckpointGenesisIdentityV1(t *testing.T) {
	db := memorydb.New()
	defer db.Close()

	genesisA := common.HexToHash("0xaaaa")
	genesisB := common.HexToHash("0xbbbb")

	checkpoint := rabbitVRFRequestCheckpointV1{
		Version:  1,
		Genesis:  genesisA,
		Next:     501,
		LastHash: common.HexToHash("0x1234"),
	}

	if err := rabbitVRFSaveCheckpointV1(db, checkpoint, 100); err != nil {
		t.Fatal(err)
	}

	restored, err := rabbitVRFLoadCheckpointV1(db, 100)
	if err != nil {
		t.Fatal(err)
	}

	if restored.Genesis != genesisA {
		t.Fatal("genesis identity lost")
	}

	if restored.Genesis == genesisB {
		t.Fatal("wrong genesis accepted")
	}
}

func TestRabbitVRFCheckpointRejectsZeroGenesisV1(t *testing.T) {
	db := memorydb.New()
	defer db.Close()

	checkpoint := rabbitVRFRequestCheckpointV1{
		Version:  1,
		Next:     501,
		LastHash: common.HexToHash("0x1234"),
	}

	if err := rabbitVRFSaveCheckpointV1(db, checkpoint, 100); err == nil {
		t.Fatal("zero genesis accepted")
	}
}
