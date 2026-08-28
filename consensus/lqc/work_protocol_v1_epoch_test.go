package lqc

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestWorkEpochV1Boundaries128(t *testing.T) {
	tests := []struct {
		block uint64
		epoch uint64
	}{
		{1, 1},
		{128, 1},
		{129, 2},
		{256, 2},
		{257, 3},
		{384, 3},
		{385, 4},
	}

	for _, test := range tests {
		got, err := WorkEpochForBlockV1(test.block, 128)
		if err != nil {
			t.Fatal(err)
		}
		if got != test.epoch {
			t.Fatalf(
				"block %d epoch = %d, want %d",
				test.block,
				got,
				test.epoch,
			)
		}
	}
}

func TestWorkEpochV1AnchorBlocks(t *testing.T) {
	tests := []struct {
		epoch  uint64
		anchor uint64
	}{
		{1, 0},
		{2, 128},
		{3, 256},
		{4, 384},
	}

	for _, test := range tests {
		got, err := WorkEpochAnchorBlockV1(test.epoch, 128)
		if err != nil {
			t.Fatal(err)
		}
		if got != test.anchor {
			t.Fatalf(
				"epoch %d anchor block = %d, want %d",
				test.epoch,
				got,
				test.anchor,
			)
		}
	}
}

func TestWorkEpochV1CommitAndSelectionDelays(t *testing.T) {
	if epoch, ok, err := WorkCommitTargetEpochV1(1, 128); err != nil || ok || epoch != 0 {
		t.Fatalf("epoch1 commit = %d %v %v", epoch, ok, err)
	}

	if epoch, ok, err := WorkCommitTargetEpochV1(129, 128); err != nil || !ok || epoch != 1 {
		t.Fatalf("epoch2 commit = %d %v %v", epoch, ok, err)
	}

	if epoch, ok, err := WorkSelectionSourceEpochV1(256, 128); err != nil || ok || epoch != 0 {
		t.Fatalf("epoch2 selection = %d %v %v", epoch, ok, err)
	}

	if epoch, ok, err := WorkSelectionSourceEpochV1(257, 128); err != nil || !ok || epoch != 1 {
		t.Fatalf("epoch3 selection = %d %v %v", epoch, ok, err)
	}

	if epoch, ok, err := WorkSelectionSourceEpochV1(385, 128); err != nil || !ok || epoch != 2 {
		t.Fatalf("epoch4 selection = %d %v %v", epoch, ok, err)
	}
}

func TestWorkEpochV1RootIgnoresArrivalOrder(t *testing.T) {
	chainID := big.NewInt(928)
	anchor := common.HexToHash("0x1111")
	difficulty := big.NewInt(8)

	a := common.HexToAddress("0x00000000000000000000000000000000000000a1")
	b := common.HexToAddress("0x00000000000000000000000000000000000000b1")
	c := common.HexToAddress("0x00000000000000000000000000000000000000c1")

	left := []WorkSeatV1{
		{TicketHash: workV1Hash(3), Participant: a},
		{TicketHash: workV1Hash(1), Participant: b},
		{TicketHash: workV1Hash(2), Participant: c},
	}
	right := []WorkSeatV1{
		left[1],
		left[2],
		left[0],
	}

	rootLeft, canonicalLeft, err := WorkEpochRootV1(
		chainID, 7, anchor, difficulty, left,
	)
	if err != nil {
		t.Fatal(err)
	}
	rootRight, canonicalRight, err := WorkEpochRootV1(
		chainID, 7, anchor, difficulty, right,
	)
	if err != nil {
		t.Fatal(err)
	}

	if rootLeft != rootRight {
		t.Fatalf("root depends on arrival order: %s != %s", rootLeft, rootRight)
	}
	if len(canonicalLeft) != len(canonicalRight) {
		t.Fatal("canonical seat length changed")
	}
	for i := range canonicalLeft {
		if canonicalLeft[i] != canonicalRight[i] {
			t.Fatalf("canonical order changed at %d", i)
		}
	}
}

func TestWorkEpochV1RootCommitsParticipantAndDifficulty(t *testing.T) {
	chainID := big.NewInt(928)
	anchor := common.HexToHash("0x2222")
	a := common.HexToAddress("0x00000000000000000000000000000000000000a1")
	b := common.HexToAddress("0x00000000000000000000000000000000000000b1")

	seatsA := []WorkSeatV1{
		{TicketHash: workV1Hash(1), Participant: a},
	}
	seatsB := []WorkSeatV1{
		{TicketHash: workV1Hash(1), Participant: b},
	}

	rootA, _, err := WorkEpochRootV1(
		chainID, 9, anchor, big.NewInt(8), seatsA,
	)
	if err != nil {
		t.Fatal(err)
	}
	rootB, _, err := WorkEpochRootV1(
		chainID, 9, anchor, big.NewInt(8), seatsB,
	)
	if err != nil {
		t.Fatal(err)
	}
	rootHarder, _, err := WorkEpochRootV1(
		chainID, 9, anchor, big.NewInt(9), seatsA,
	)
	if err != nil {
		t.Fatal(err)
	}

	if rootA == rootB {
		t.Fatal("participant was not committed by root")
	}
	if rootA == rootHarder {
		t.Fatal("difficulty was not committed by root")
	}
}

func TestWorkEpochV1SnapshotValidation(t *testing.T) {
	chainID := big.NewInt(928)
	anchor := common.HexToHash("0x3333")
	a := common.HexToAddress("0x00000000000000000000000000000000000000a1")
	b := common.HexToAddress("0x00000000000000000000000000000000000000b1")

	snapshot, err := NewWorkEpochSnapshotV1(
		chainID,
		12,
		anchor,
		big.NewInt(8),
		[]WorkSeatV1{
			{TicketHash: workV1Hash(2), Participant: a},
			{TicketHash: workV1Hash(1), Participant: b},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := snapshot.Validate(chainID); err != nil {
		t.Fatal(err)
	}

	snapshot.Root = common.HexToHash("0xdead")
	if err := snapshot.Validate(chainID); err != ErrInvalidWorkEpochRootV1 {
		t.Fatalf("tampered root error = %v", err)
	}
}

func TestWorkEpochV1RejectsDuplicateParticipant(t *testing.T) {
	a := common.HexToAddress("0x00000000000000000000000000000000000000a1")
	_, _, err := WorkEpochRootV1(
		big.NewInt(928),
		3,
		common.HexToHash("0x5555"),
		big.NewInt(8),
		[]WorkSeatV1{
			{TicketHash: workV1Hash(1), Participant: a},
			{TicketHash: workV1Hash(2), Participant: a},
		},
	)
	if err != ErrDuplicateWorkParticipantV1 {
		t.Fatalf("error = %v, want %v", err, ErrDuplicateWorkParticipantV1)
	}
}

func TestWorkEpochV1RejectsDuplicateTicketHash(t *testing.T) {
	chainID := big.NewInt(928)
	anchor := common.HexToHash("0x4444")
	a := common.HexToAddress("0x00000000000000000000000000000000000000a1")
	b := common.HexToAddress("0x00000000000000000000000000000000000000b1")

	_, _, err := WorkEpochRootV1(
		chainID,
		3,
		anchor,
		big.NewInt(8),
		[]WorkSeatV1{
			{TicketHash: workV1Hash(1), Participant: a},
			{TicketHash: workV1Hash(1), Participant: b},
		},
	)
	if err != ErrDuplicateRandomXWorkHash {
		t.Fatalf("error = %v, want %v", err, ErrDuplicateRandomXWorkHash)
	}
}
