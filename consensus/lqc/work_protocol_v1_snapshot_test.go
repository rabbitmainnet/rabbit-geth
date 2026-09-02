package lqc

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

func workSnapshotHashV1(label string, number uint64) common.Hash {
	return crypto.Keccak256Hash(
		[]byte(label),
		new(big.Int).SetUint64(number).Bytes(),
	)
}

func verifiedWorkV1(
	epoch uint64,
	participant common.Address,
	hashByte byte,
	nonce uint64,
) VerifiedRandomXWorkTicketV1 {
	return VerifiedRandomXWorkTicketV1{
		Ticket: RandomXWorkTicketV1{
			Version:     RandomXWorkProtocolVersion,
			Epoch:       epoch,
			Participant: participant,
			Nonce:       nonce,
		},
		Hash: workV1Hash(hashByte),
	}
}

func advanceEmptyWorkBlocksV1(
	t *testing.T,
	chainID *big.Int,
	snapshot *WorkChainSnapshotV1,
	to uint64,
) *WorkChainSnapshotV1 {
	t.Helper()
	current := snapshot
	for current.Number < to {
		number := current.Number + 1
		next, err := current.ApplyVerifiedBlockV1(
			chainID,
			number,
			workSnapshotHashV1("main", number),
			current.Hash,
			common.Hash{},
			nil,
			nil,
		)
		if err != nil {
			t.Fatalf("block %d: %v", number, err)
		}
		current = next
	}
	return current
}

func TestWorkChainSnapshotV1ClosesEpochForNPlus2Selection(t *testing.T) {
	chainID := big.NewInt(928)
	genesisHash := workSnapshotHashV1("genesis", 0)

	snapshot, err := NewWorkChainSnapshotV1(
		chainID,
		0,
		genesisHash,
		128,
	)
	if err != nil {
		t.Fatal(err)
	}

	snapshot = advanceEmptyWorkBlocksV1(
		t,
		chainID,
		snapshot,
		128,
	)

	participant := common.HexToAddress(
		"0x00000000000000000000000000000000000000a1",
	)
	participantB := common.HexToAddress(
		"0x00000000000000000000000000000000000000b1",
	)
	difficulty := big.NewInt(8)

	for number := uint64(129); number <= 256; number++ {
		var verified []VerifiedRandomXWorkTicketV1
		if number == 129 {
			verified = []VerifiedRandomXWorkTicketV1{
				verifiedWorkV1(1, participant, 1, 1),
				verifiedWorkV1(1, participantB, 2, 2),
			}
		}

		next, err := snapshot.ApplyVerifiedBlockV1(
			chainID,
			number,
			workSnapshotHashV1("main", number),
			snapshot.Hash,
			genesisHash,
			difficulty,
			verified,
		)
		if err != nil {
			t.Fatalf("block %d: %v", number, err)
		}
		snapshot = next
	}

	if snapshot.CommitEpoch != 0 {
		t.Fatalf("commit epoch still open: %d", snapshot.CommitEpoch)
	}
	if snapshot.SelectionEpoch != 1 {
		t.Fatalf("selection epoch = %d, want 1", snapshot.SelectionEpoch)
	}
	if len(snapshot.SelectionSeats) != 2 {
		t.Fatalf("selection seats = %d, want 2", len(snapshot.SelectionSeats))
	}

	selected, ok, err := snapshot.SelectionSnapshotV1(chainID, 257)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || selected.Epoch != 1 || len(selected.Seats) != 2 {
		t.Fatalf("unexpected selection snapshot: %+v ok=%v", selected, ok)
	}
}

func TestWorkChainSnapshotV1KeepsPersistentSeatsAcrossEpochs(t *testing.T) {
	chainID := big.NewInt(928)
	genesisHash := workSnapshotHashV1("persistent-genesis", 0)
	snapshot, err := NewWorkChainSnapshotV1(chainID, 0, genesisHash, 128)
	if err != nil {
		t.Fatal(err)
	}
	snapshot = advanceEmptyWorkBlocksV1(t, chainID, snapshot, 128)

	participantA := common.HexToAddress(
		"0x00000000000000000000000000000000000000a1",
	)
	participantB := common.HexToAddress(
		"0x00000000000000000000000000000000000000b1",
	)
	difficulty := big.NewInt(8)

	for number := uint64(129); number <= 256; number++ {
		var verified []VerifiedRandomXWorkTicketV1
		if number == 129 {
			verified = []VerifiedRandomXWorkTicketV1{
				verifiedWorkV1(1, participantA, 1, 1),
			}
		}
		next, applyErr := snapshot.ApplyVerifiedBlockV1(
			chainID, number,
			workSnapshotHashV1("persistent", number),
			snapshot.Hash, genesisHash, difficulty, verified,
		)
		if applyErr != nil {
			t.Fatalf("block %d: %v", number, applyErr)
		}
		snapshot = next
	}

	anchorEpoch2 := workSnapshotHashV1("persistent-anchor", 2)
	for number := uint64(257); number <= 384; number++ {
		var verified []VerifiedRandomXWorkTicketV1
		if number == 257 {
			verified = []VerifiedRandomXWorkTicketV1{
				verifiedWorkV1(2, participantB, 2, 2),
			}
		}
		next, applyErr := snapshot.ApplyVerifiedBlockV1(
			chainID, number,
			workSnapshotHashV1("persistent", number),
			snapshot.Hash, anchorEpoch2, difficulty, verified,
		)
		if applyErr != nil {
			t.Fatalf("block %d: %v", number, applyErr)
		}
		snapshot = next
	}

	if snapshot.SelectionEpoch != 2 {
		t.Fatalf("selection epoch = %d, want 2", snapshot.SelectionEpoch)
	}
	if len(snapshot.SelectionSeats) != 2 {
		t.Fatalf("persistent seats = %d, want 2", len(snapshot.SelectionSeats))
	}
	seen := make(map[common.Address]bool)
	for _, seat := range snapshot.SelectionSeats {
		seen[seat.Participant] = true
	}
	if !seen[participantA] || !seen[participantB] {
		t.Fatalf("persistent seats lost: %+v", snapshot.SelectionSeats)
	}
}

func TestWorkChainSnapshotV1RejectsAlreadySeatedParticipant(t *testing.T) {
	chainID := big.NewInt(928)
	genesisHash := workSnapshotHashV1("already-seated-genesis", 0)
	snapshot, err := NewWorkChainSnapshotV1(chainID, 0, genesisHash, 128)
	if err != nil {
		t.Fatal(err)
	}
	snapshot = advanceEmptyWorkBlocksV1(t, chainID, snapshot, 128)
	participant := common.HexToAddress(
		"0x00000000000000000000000000000000000000a1",
	)
	difficulty := big.NewInt(8)
	for number := uint64(129); number <= 256; number++ {
		var verified []VerifiedRandomXWorkTicketV1
		if number == 129 {
			verified = []VerifiedRandomXWorkTicketV1{
				verifiedWorkV1(1, participant, 1, 1),
			}
		}
		next, applyErr := snapshot.ApplyVerifiedBlockV1(
			chainID, number,
			workSnapshotHashV1("already-seated", number),
			snapshot.Hash, genesisHash, difficulty, verified,
		)
		if applyErr != nil {
			t.Fatalf("block %d: %v", number, applyErr)
		}
		snapshot = next
	}

	_, err = snapshot.ApplyVerifiedBlockV1(
		chainID, 257,
		workSnapshotHashV1("already-seated", 257),
		snapshot.Hash,
		workSnapshotHashV1("already-seated-anchor", 2),
		difficulty,
		[]VerifiedRandomXWorkTicketV1{
			verifiedWorkV1(2, participant, 2, 2),
		},
	)
	if err != ErrWorkParticipantAlreadySeatedV1 {
		t.Fatalf(
			"error = %v, want %v",
			err,
			ErrWorkParticipantAlreadySeatedV1,
		)
	}
}

func TestWorkChainSnapshotV1RejectsDuplicateAcrossBlocks(t *testing.T) {
	chainID := big.NewInt(928)
	genesisHash := workSnapshotHashV1("genesis", 0)

	snapshot, err := NewWorkChainSnapshotV1(
		chainID, 0, genesisHash, 128,
	)
	if err != nil {
		t.Fatal(err)
	}
	snapshot = advanceEmptyWorkBlocksV1(
		t, chainID, snapshot, 128,
	)

	participant := common.HexToAddress(
		"0x00000000000000000000000000000000000000a1",
	)
	ticket := verifiedWorkV1(1, participant, 9, 1)

	block129, err := snapshot.ApplyVerifiedBlockV1(
		chainID,
		129,
		workSnapshotHashV1("main", 129),
		snapshot.Hash,
		genesisHash,
		big.NewInt(8),
		[]VerifiedRandomXWorkTicketV1{ticket},
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = block129.ApplyVerifiedBlockV1(
		chainID,
		130,
		workSnapshotHashV1("main", 130),
		block129.Hash,
		genesisHash,
		big.NewInt(8),
		[]VerifiedRandomXWorkTicketV1{ticket},
	)
	if err != ErrDuplicateRandomXWorkHash {
		t.Fatalf(
			"duplicate error = %v, want %v",
			err,
			ErrDuplicateRandomXWorkHash,
		)
	}
}

func TestWorkChainSnapshotV1RejectsParticipantAcrossBlocks(t *testing.T) {
	chainID := big.NewInt(928)
	genesisHash := workSnapshotHashV1("genesis-participant", 0)
	snapshot, err := NewWorkChainSnapshotV1(chainID, 0, genesisHash, 128)
	if err != nil {
		t.Fatal(err)
	}
	snapshot = advanceEmptyWorkBlocksV1(t, chainID, snapshot, 128)
	participant := common.HexToAddress("0x00000000000000000000000000000000000000a1")
	block129, err := snapshot.ApplyVerifiedBlockV1(
		chainID, 129, workSnapshotHashV1("participant", 129),
		snapshot.Hash, genesisHash, big.NewInt(8),
		[]VerifiedRandomXWorkTicketV1{
			verifiedWorkV1(1, participant, 1, 1),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = block129.ApplyVerifiedBlockV1(
		chainID, 130, workSnapshotHashV1("participant", 130),
		block129.Hash, genesisHash, big.NewInt(8),
		[]VerifiedRandomXWorkTicketV1{
			verifiedWorkV1(1, participant, 2, 2),
		},
	)
	if err != ErrDuplicateWorkParticipantV1 {
		t.Fatalf("error = %v, want %v", err, ErrDuplicateWorkParticipantV1)
	}
}

func TestWorkChainSnapshotV1ReorgBranchesStayIndependent(t *testing.T) {
	chainID := big.NewInt(928)
	genesisHash := workSnapshotHashV1("genesis", 0)

	base, err := NewWorkChainSnapshotV1(
		chainID, 0, genesisHash, 128,
	)
	if err != nil {
		t.Fatal(err)
	}
	base = advanceEmptyWorkBlocksV1(
		t, chainID, base, 128,
	)

	a := common.HexToAddress(
		"0x00000000000000000000000000000000000000a1",
	)
	b := common.HexToAddress(
		"0x00000000000000000000000000000000000000b1",
	)

	branchA, err := base.ApplyVerifiedBlockV1(
		chainID,
		129,
		workSnapshotHashV1("branch-a", 129),
		base.Hash,
		genesisHash,
		big.NewInt(8),
		[]VerifiedRandomXWorkTicketV1{
			verifiedWorkV1(1, a, 1, 1),
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	branchB, err := base.ApplyVerifiedBlockV1(
		chainID,
		129,
		workSnapshotHashV1("branch-b", 129),
		base.Hash,
		genesisHash,
		big.NewInt(8),
		[]VerifiedRandomXWorkTicketV1{
			verifiedWorkV1(1, b, 2, 1),
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	if branchA.Hash == branchB.Hash {
		t.Fatal("reorg branches unexpectedly share block hash")
	}
	if branchA.StateRoot == branchB.StateRoot {
		t.Fatal("different branch work state produced same root")
	}
	if len(base.CommitSeats) != 0 ||
		base.CommitEpoch != 0 {
		t.Fatal("parent snapshot was mutated by child application")
	}
	if branchA.CommitSeats[0].Participant != a {
		t.Fatal("branch A contaminated")
	}
	if branchB.CommitSeats[0].Participant != b {
		t.Fatal("branch B contaminated")
	}
}

func TestWorkChainSnapshotV1ArrivalOrderDoesNotChangeStateRoot(t *testing.T) {
	chainID := big.NewInt(928)
	genesisHash := workSnapshotHashV1("genesis", 0)

	base, err := NewWorkChainSnapshotV1(
		chainID, 0, genesisHash, 128,
	)
	if err != nil {
		t.Fatal(err)
	}
	base = advanceEmptyWorkBlocksV1(
		t, chainID, base, 128,
	)

	a := common.HexToAddress(
		"0x00000000000000000000000000000000000000a1",
	)
	b := common.HexToAddress(
		"0x00000000000000000000000000000000000000b1",
	)
	x := verifiedWorkV1(1, a, 1, 1)
	y := verifiedWorkV1(1, b, 2, 1)

	hash := workSnapshotHashV1("same-block", 129)

	left, err := base.ApplyVerifiedBlockV1(
		chainID,
		129,
		hash,
		base.Hash,
		genesisHash,
		big.NewInt(8),
		[]VerifiedRandomXWorkTicketV1{x, y},
	)
	if err != nil {
		t.Fatal(err)
	}

	right, err := base.ApplyVerifiedBlockV1(
		chainID,
		129,
		hash,
		base.Hash,
		genesisHash,
		big.NewInt(8),
		[]VerifiedRandomXWorkTicketV1{y, x},
	)
	if err != nil {
		t.Fatal(err)
	}

	if left.StateRoot != right.StateRoot {
		t.Fatalf(
			"arrival order changed root: %s != %s",
			left.StateRoot,
			right.StateRoot,
		)
	}
	if !left.EqualConsensusStateV1(right) {
		t.Fatal("arrival order changed consensus state")
	}
}

func TestWorkChainSnapshotV1RejectsTamperedStateRoot(t *testing.T) {
	chainID := big.NewInt(928)
	snapshot, err := NewWorkChainSnapshotV1(
		chainID,
		0,
		workSnapshotHashV1("genesis", 0),
		128,
	)
	if err != nil {
		t.Fatal(err)
	}

	snapshot.StateRoot = common.HexToHash("0xdead")
	if err := snapshot.Validate(chainID); err != ErrInvalidWorkChainSnapshotV1 {
		t.Fatalf("tampered root error = %v", err)
	}
}
