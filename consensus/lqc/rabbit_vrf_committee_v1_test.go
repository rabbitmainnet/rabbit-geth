package lqc

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func rabbitVRFCommitteeTestSnapshotV1(
	t *testing.T,
	chainID *big.Int,
	count int,
) *WorkEpochSnapshotV1 {
	t.Helper()

	seats := make([]WorkSeatV1, 0, count)
	for i := 0; i < count; i++ {
		seats = append(seats, WorkSeatV1{
			Participant: common.BigToAddress(
				new(big.Int).SetUint64(uint64(i + 1)),
			),
			TicketHash: common.BigToHash(
				new(big.Int).SetUint64(uint64(i + 1001)),
			),
		})
	}

	root, canonical, err := WorkEpochRootV1(
		chainID,
		7,
		common.HexToHash("0x7001"),
		big.NewInt(4096),
		seats,
	)
	if err != nil {
		t.Fatal(err)
	}

	snapshot := &WorkEpochSnapshotV1{
		Epoch:      7,
		Anchor:     common.HexToHash("0x7001"),
		Difficulty: big.NewInt(4096),
		Root:       root,
		Seats:      canonical,
	}

	if err := snapshot.Validate(chainID); err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func TestRabbitVRFCommitteeSeedV1StableAndContextBound(t *testing.T) {
	chainID := big.NewInt(9280)
	root := common.HexToHash("0x1234")
	entropy := common.HexToHash("0x5678")

	first, err := RabbitVRFCommitteeSeedV1(
		chainID,
		7,
		root,
		entropy,
	)
	if err != nil {
		t.Fatal(err)
	}

	second, err := RabbitVRFCommitteeSeedV1(
		chainID,
		7,
		root,
		entropy,
	)
	if err != nil {
		t.Fatal(err)
	}

	if first != second {
		t.Fatal("same closed context produced different VRF committee seed")
	}

	changedEpoch, err := RabbitVRFCommitteeSeedV1(
		chainID,
		8,
		root,
		entropy,
	)
	if err != nil {
		t.Fatal(err)
	}
	if changedEpoch == first {
		t.Fatal("source epoch did not bind VRF committee seed")
	}

	changedRoot, err := RabbitVRFCommitteeSeedV1(
		chainID,
		7,
		common.HexToHash("0x1235"),
		entropy,
	)
	if err != nil {
		t.Fatal(err)
	}
	if changedRoot == first {
		t.Fatal("selection root did not bind VRF committee seed")
	}

	changedEntropy, err := RabbitVRFCommitteeSeedV1(
		chainID,
		7,
		root,
		common.HexToHash("0x5679"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if changedEntropy == first {
		t.Fatal("entropy did not bind VRF committee seed")
	}

	changedChain, err := RabbitVRFCommitteeSeedV1(
		big.NewInt(928),
		7,
		root,
		entropy,
	)
	if err != nil {
		t.Fatal(err)
	}
	if changedChain == first {
		t.Fatal("chain id did not bind VRF committee seed")
	}
}

func TestRabbitVRFCommitteeSizeV1(t *testing.T) {
	tests := []struct {
		seats uint64
		want  uint64
	}{
		{0, 0},
		{1, 1},
		{5, 5},
		{31, 31},
		{32, 32},
		{100, 32},
		{320, 32},
		{321, 33},
		{1000, 100},
		{2000, 128},
	}

	for _, test := range tests {
		got := RabbitVRFCommitteeSizeV1(
			test.seats,
			32,
			128,
		)
		if got != test.want {
			t.Fatalf(
				"seats=%d size=%d want=%d",
				test.seats,
				got,
				test.want,
			)
		}
	}
}

func TestRabbitVRFCommitteeForSnapshotV1Deterministic(t *testing.T) {
	chainID := big.NewInt(9280)
	snapshot := rabbitVRFCommitteeTestSnapshotV1(
		t,
		chainID,
		100,
	)
	entropy := common.HexToHash("0xabcdef")

	first, seedA, err := RabbitVRFCommitteeForSnapshotV1(
		chainID,
		snapshot,
		entropy,
		32,
		128,
	)
	if err != nil {
		t.Fatal(err)
	}

	second, seedB, err := RabbitVRFCommitteeForSnapshotV1(
		chainID,
		snapshot,
		entropy,
		32,
		128,
	)
	if err != nil {
		t.Fatal(err)
	}

	if seedA != seedB {
		t.Fatal("committee seed changed")
	}

	if len(first) != 32 || len(second) != 32 {
		t.Fatalf(
			"committee sizes=%d,%d want=32",
			len(first),
			len(second),
		)
	}

	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("committee differs at index %d", i)
		}
	}
}

func TestRabbitVRFCommitteeForSnapshotV1UsesAllSmallSet(t *testing.T) {
	chainID := big.NewInt(9280)
	snapshot := rabbitVRFCommitteeTestSnapshotV1(
		t,
		chainID,
		5,
	)

	committee, _, err := RabbitVRFCommitteeForSnapshotV1(
		chainID,
		snapshot,
		common.HexToHash("0x123456"),
		32,
		128,
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(committee) != 5 {
		t.Fatalf(
			"committee size=%d want=5",
			len(committee),
		)
	}
}

func TestRabbitVRFCommitteeForSnapshotV1RejectsInvalidSnapshot(
	t *testing.T,
) {
	chainID := big.NewInt(9280)
	snapshot := rabbitVRFCommitteeTestSnapshotV1(
		t,
		chainID,
		5,
	)

	broken := *snapshot
	broken.Root = common.HexToHash("0xdead")

	if _, _, err := RabbitVRFCommitteeForSnapshotV1(
		chainID,
		&broken,
		common.HexToHash("0x123456"),
		32,
		128,
	); err == nil {
		t.Fatal("invalid closed snapshot was accepted")
	}
}
