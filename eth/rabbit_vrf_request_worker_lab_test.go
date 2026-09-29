//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
)

func TestRabbitVRFRequestScanCursorV1ReorgRewinds(t *testing.T) {
	activation := uint64(100)
	oldHash := common.HexToHash("0x01")
	newHash := common.HexToHash("0x02")

	cursor := rabbitVRFRequestScanCursorV1{
		Next:     151,
		LastHash: oldHash,
	}

	if !cursor.reconcilePreviousV1(activation, newHash) {
		t.Fatal("canonical reorg was not detected")
	}
	if cursor.Next != activation {
		t.Fatalf("reorg cursor next=%d want=%d", cursor.Next, activation)
	}
	if cursor.LastHash != (common.Hash{}) {
		t.Fatal("reorg cursor retained stale canonical hash")
	}
}

func TestRabbitVRFRequestScanCursorV1CanonicalContinuation(t *testing.T) {
	activation := uint64(100)
	canonicalHash := common.HexToHash("0x1234")
	cursor := rabbitVRFRequestScanCursorV1{
		Next:     151,
		LastHash: canonicalHash,
	}

	if cursor.reconcilePreviousV1(activation, canonicalHash) {
		t.Fatal("canonical cursor was incorrectly rewound")
	}
	if cursor.Next != 151 || cursor.LastHash != canonicalHash {
		t.Fatal("canonical cursor changed unexpectedly")
	}
}

func TestRabbitVRFRequestScanCursorV1MissingPreviousRewinds(t *testing.T) {
	activation := uint64(100)
	cursor := rabbitVRFRequestScanCursorV1{
		Next:     151,
		LastHash: common.HexToHash("0x1234"),
	}

	if !cursor.reconcilePreviousV1(activation, common.Hash{}) {
		t.Fatal("missing previous canonical block did not rewind cursor")
	}
	if cursor.Next != activation || cursor.LastHash != (common.Hash{}) {
		t.Fatal("missing previous canonical block did not reset cursor")
	}
}

func TestRabbitVRFRequestIDsFromReceiptsV1MultipleAndDuplicate(t *testing.T) {
	requestA := common.HexToHash("0xaa")
	requestB := common.HexToHash("0xbb")

	requested := func(address common.Address, requestID common.Hash) *types.Log {
		return &types.Log{
			Address: address,
			Topics: []common.Hash{
				rabbitVRFRandomnessRequestedTopicV1,
				requestID,
				common.HexToHash("0x11"),
				common.HexToHash("0x22"),
			},
		}
	}

	receipts := types.Receipts{
		&types.Receipt{Logs: []*types.Log{
			requested(params.RabbitVRFCoordinatorV1Address, requestA),
			requested(params.RabbitVRFCoordinatorV1Address, requestB),
			requested(params.RabbitVRFCoordinatorV1Address, requestA),
		}},
		&types.Receipt{Logs: []*types.Log{
			requested(common.HexToAddress("0x1234"), common.HexToHash("0xcc")),
			requested(params.RabbitVRFCoordinatorV1Address, common.Hash{}),
		}},
	}

	got := rabbitVRFRequestIDsFromReceiptsV1(receipts)
	if len(got) != 2 {
		t.Fatalf("request count=%d want=2", len(got))
	}
	if got[0] != requestA || got[1] != requestB {
		t.Fatalf("request order=%v want=[%s %s]", got, requestA, requestB)
	}
}
