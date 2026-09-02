package lqc

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

func runtimeHashV1(label string, number uint64) common.Hash {
	return crypto.Keccak256Hash(
		[]byte(label),
		new(big.Int).SetUint64(number).Bytes(),
	)
}

func runtimeVerifiedV1(
	epoch uint64,
	index uint64,
) VerifiedRandomXWorkTicketV1 {
	participant := common.BigToAddress(
		new(big.Int).SetUint64(index + 1),
	)
	return VerifiedRandomXWorkTicketV1{
		Ticket: RandomXWorkTicketV1{
			Version:     RandomXWorkProtocolVersion,
			Epoch:       epoch,
			Participant: participant,
			Nonce:       index + 1,
		},
		Hash: crypto.Keccak256Hash(
			[]byte("runtime-ticket"),
			new(big.Int).SetUint64(epoch).Bytes(),
			new(big.Int).SetUint64(index).Bytes(),
		),
	}
}

func runtimeAdvanceBlockV1(
	t *testing.T,
	chainID *big.Int,
	state *CanonicalWorkRuntimeStateV1,
	number uint64,
	challenge common.Hash,
	verified []VerifiedRandomXWorkTicketV1,
) *CanonicalWorkRuntimeStateV1 {
	t.Helper()
	next, err := state.ApplyVerifiedBlockV1(
		chainID,
		number,
		runtimeHashV1("block", number),
		state.Work.Hash,
		challenge,
		verified,
	)
	if err != nil {
		t.Fatalf("block %d: %v", number, err)
	}
	return next
}

func TestCanonicalWorkRuntimeV1InitialEpochsShareExplicitDifficulty(
	t *testing.T,
) {
	chainID := big.NewInt(928)
	state, err := NewCanonicalWorkRuntimeStateV1(
		chainID,
		0,
		runtimeHashV1("genesis", 0),
		WorkProtocolEpochLengthV1,
		big.NewInt(4096),
	)
	if err != nil {
		t.Fatal(err)
	}

	for _, epoch := range []uint64{1, 2} {
		got, err := state.Difficulty.DifficultyForEpochV1(
			chainID,
			epoch,
		)
		if err != nil {
			t.Fatal(err)
		}
		if got.Cmp(big.NewInt(4096)) != 0 {
			t.Fatalf("epoch %d difficulty=%s want=4096", epoch, got)
		}
	}
}

func TestCanonicalWorkRuntimeV1NPlus2DifficultySchedule(t *testing.T) {
	chainID := big.NewInt(928)
	genesis := runtimeHashV1("genesis", 0)
	state, err := NewCanonicalWorkRuntimeStateV1(
		chainID,
		0,
		genesis,
		WorkProtocolEpochLengthV1,
		big.NewInt(4096),
	)
	if err != nil {
		t.Fatal(err)
	}

	// Chain epoch 1: no commits.
	for number := uint64(1); number <= 128; number++ {
		state = runtimeAdvanceBlockV1(
			t, chainID, state, number, common.Hash{}, nil,
		)
	}

	// Chain epoch 2 commits work epoch 1 with the full 1024-seat capacity.
	// Closing E1 must schedule E3 at 4x difficulty.
	var ticket uint64
	for number := uint64(129); number <= 256; number++ {
		batch := make([]VerifiedRandomXWorkTicketV1, 0, 8)
		for range 8 {
			batch = append(batch, runtimeVerifiedV1(1, ticket))
			ticket++
		}
		state = runtimeAdvanceBlockV1(
			t, chainID, state, number, genesis, batch,
		)
	}
	if ticket != WorkTicketCommitCapacityPerEpochV1 {
		t.Fatalf("epoch1 tickets=%d", ticket)
	}

	d3, err := state.Difficulty.DifficultyForEpochV1(chainID, 3)
	if err != nil {
		t.Fatal(err)
	}
	if d3.Cmp(big.NewInt(16384)) != 0 {
		t.Fatalf("D3=%s want=16384", d3)
	}

	// E2 still uses its original parity head; closing 128 seats halves D4.
	challenge2 := runtimeHashV1("challenge-e2", 128)
	for number := uint64(257); number <= 384; number++ {
		index := number - 257
		state = runtimeAdvanceBlockV1(
			t,
			chainID,
			state,
			number,
			challenge2,
			[]VerifiedRandomXWorkTicketV1{
				runtimeVerifiedV1(2, WorkTicketCommitCapacityPerEpochV1+index),
			},
		)
	}

	d4, err := state.Difficulty.DifficultyForEpochV1(chainID, 4)
	if err != nil {
		t.Fatal(err)
	}
	if d4.Cmp(big.NewInt(2048)) != 0 {
		t.Fatalf("D4=%s want=2048", d4)
	}

	// At the first block committing E3, the runtime itself returns 16384.
	difficulty, ok, err := state.CommitDifficultyV1(chainID, 385)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || difficulty.Cmp(big.NewInt(16384)) != 0 {
		t.Fatalf("commit E3 difficulty=%v ok=%v", difficulty, ok)
	}

	if err := state.Validate(chainID); err != nil {
		t.Fatal(err)
	}
}

func TestCanonicalWorkRuntimeV1RejectsClosedDifficultyMismatch(
	t *testing.T,
) {
	chainID := big.NewInt(928)
	state, err := NewWorkDifficultyStateV1(
		chainID,
		big.NewInt(4096),
	)
	if err != nil {
		t.Fatal(err)
	}

	closed, err := NewWorkEpochSnapshotV1(
		chainID,
		1,
		runtimeHashV1("challenge", 1),
		big.NewInt(2048),
		[]WorkSeatV1{
			{
				TicketHash: runtimeHashV1("ticket", 1),
				Participant: common.HexToAddress(
					"0x0000000000000000000000000000000000000001",
				),
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = state.AdvanceClosedEpochV1(chainID, closed)
	if err != ErrWorkDifficultyClosedEpochMismatchV1 {
		t.Fatalf("error=%v", err)
	}
}

func TestCanonicalWorkRuntimeV1StateRootBindsDifficulty(t *testing.T) {
	chainID := big.NewInt(928)
	genesis := runtimeHashV1("genesis", 0)

	left, err := NewCanonicalWorkRuntimeStateV1(
		chainID, 0, genesis, 128, big.NewInt(4096),
	)
	if err != nil {
		t.Fatal(err)
	}
	right, err := NewCanonicalWorkRuntimeStateV1(
		chainID, 0, genesis, 128, big.NewInt(8192),
	)
	if err != nil {
		t.Fatal(err)
	}

	if left.Work.StateRoot != right.Work.StateRoot {
		t.Fatal("base work root unexpectedly depends on runtime difficulty")
	}
	if left.StateRoot == right.StateRoot {
		t.Fatal("runtime root failed to bind difficulty state")
	}
}

func TestCanonicalWorkRuntimeV1ReorgBranchesStayIndependent(t *testing.T) {
	chainID := big.NewInt(928)
	genesis := runtimeHashV1("genesis", 0)
	base, err := NewCanonicalWorkRuntimeStateV1(
		chainID, 0, genesis, 128, big.NewInt(4096),
	)
	if err != nil {
		t.Fatal(err)
	}

	for number := uint64(1); number <= 128; number++ {
		base = runtimeAdvanceBlockV1(
			t, chainID, base, number, common.Hash{}, nil,
		)
	}

	left, err := base.ApplyVerifiedBlockV1(
		chainID,
		129,
		runtimeHashV1("left", 129),
		base.Work.Hash,
		genesis,
		[]VerifiedRandomXWorkTicketV1{
			runtimeVerifiedV1(1, 1),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	right, err := base.ApplyVerifiedBlockV1(
		chainID,
		129,
		runtimeHashV1("right", 129),
		base.Work.Hash,
		genesis,
		[]VerifiedRandomXWorkTicketV1{
			runtimeVerifiedV1(1, 2),
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	if left.StateRoot == right.StateRoot {
		t.Fatal("reorg branches share runtime root")
	}
	if base.Work.CommitEpoch != 0 || len(base.Work.CommitSeats) != 0 {
		t.Fatal("parent runtime state mutated")
	}
}
