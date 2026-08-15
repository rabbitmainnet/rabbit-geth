package lqc

import (
	"bytes"
	"errors"
	"math/big"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
)

const (
	// Work generated in epoch N is committed during N+1 and first becomes
	// eligible for role selection in N+2.
	WorkCommitDelayEpochsV1    uint64 = 1
	WorkSelectionDelayEpochsV1 uint64 = 2
)

var (
	ErrInvalidWorkEpochV1       = errors.New("invalid lqc work epoch v1")
	ErrInvalidWorkEpochLengthV1 = errors.New("invalid lqc work epoch length v1")
	ErrInvalidWorkEpochRootV1   = errors.New("invalid lqc work epoch root v1")
)

var workEpochRootDomainV1 = []byte("RABBIT-LQC-WORK-EPOCH-ROOT-V1")

// WorkEpochSnapshotV1 is an inactive, deterministic model of one CLOSED work
// epoch. It contains work seats only after RandomX target and participant
// authorization have already been verified.
//
// It is not wired into header validation or block production yet.
type WorkEpochSnapshotV1 struct {
	Epoch      uint64
	Anchor     common.Hash
	Difficulty *big.Int
	Root       common.Hash
	Seats      []WorkSeatV1
}

type workEpochRootPayloadV1 struct {
	Domain     []byte
	Version    uint8
	ChainID    *big.Int
	Epoch      uint64
	Anchor     common.Hash
	Difficulty *big.Int
	Seats      []WorkSeatV1
}

// WorkEpochForBlockV1 maps blocks 1..epochLength to epoch 1,
// epochLength+1..2*epochLength to epoch 2, and so on.
func WorkEpochForBlockV1(blockNumber, epochLength uint64) (uint64, error) {
	if blockNumber == 0 || epochLength == 0 {
		return 0, ErrInvalidWorkEpochLengthV1
	}
	return ((blockNumber - 1) / epochLength) + 1, nil
}

// WorkEpochStartBlockV1 returns the first block inside an epoch.
func WorkEpochStartBlockV1(epoch, epochLength uint64) (uint64, error) {
	if epoch == 0 || epochLength == 0 {
		return 0, ErrInvalidWorkEpochLengthV1
	}
	if epoch-1 > (^uint64(0)-1)/epochLength {
		return 0, ErrInvalidWorkEpochLengthV1
	}
	return (epoch-1)*epochLength + 1, nil
}

// WorkEpochAnchorBlockV1 returns the canonical block whose hash anchors the
// RandomX epoch key. Epoch 1 is therefore anchored by genesis block 0.
func WorkEpochAnchorBlockV1(epoch, epochLength uint64) (uint64, error) {
	start, err := WorkEpochStartBlockV1(epoch, epochLength)
	if err != nil {
		return 0, err
	}
	return start - 1, nil
}

// WorkCommitTargetEpochV1 returns the epoch whose tickets are in their commit
// window at blockNumber. During epoch 1 there is no prior epoch to commit.
func WorkCommitTargetEpochV1(
	blockNumber,
	epochLength uint64,
) (uint64, bool, error) {
	current, err := WorkEpochForBlockV1(blockNumber, epochLength)
	if err != nil {
		return 0, false, err
	}
	if current <= WorkCommitDelayEpochsV1 {
		return 0, false, nil
	}
	return current - WorkCommitDelayEpochsV1, true, nil
}

// WorkSelectionSourceEpochV1 returns the CLOSED work epoch used for role
// selection at blockNumber. The first two epochs deliberately have no work
// source yet; bootstrap/liveness behavior remains an engine-wiring decision.
func WorkSelectionSourceEpochV1(
	blockNumber,
	epochLength uint64,
) (uint64, bool, error) {
	current, err := WorkEpochForBlockV1(blockNumber, epochLength)
	if err != nil {
		return 0, false, err
	}
	if current <= WorkSelectionDelayEpochsV1 {
		return 0, false, nil
	}
	return current - WorkSelectionDelayEpochsV1, true, nil
}

func canonicalWorkEpochSeatsV1(input []WorkSeatV1) ([]WorkSeatV1, error) {
	out := append([]WorkSeatV1(nil), input...)
	seen := make(map[common.Hash]struct{}, len(out))

	for _, seat := range out {
		if seat.TicketHash == (common.Hash{}) ||
			seat.Participant == (common.Address{}) {
			return nil, ErrInvalidWorkSeat
		}
		if _, exists := seen[seat.TicketHash]; exists {
			return nil, ErrDuplicateRandomXWorkHash
		}
		seen[seat.TicketHash] = struct{}{}
	}

	sort.Slice(out, func(i, j int) bool {
		if order := bytes.Compare(
			out[i].TicketHash.Bytes(),
			out[j].TicketHash.Bytes(),
		); order != 0 {
			return order < 0
		}
		return out[i].Participant.Cmp(out[j].Participant) < 0
	})
	return out, nil
}

// WorkEpochRootV1 commits to chain, epoch, anchor, target difficulty and every
// work seat. Arrival order is irrelevant. Participant identity remains in the
// commitment but never creates an additional seat by itself.
func WorkEpochRootV1(
	chainID *big.Int,
	epoch uint64,
	anchor common.Hash,
	difficulty *big.Int,
	seats []WorkSeatV1,
) (common.Hash, []WorkSeatV1, error) {
	if chainID == nil || chainID.Sign() <= 0 ||
		epoch == 0 ||
		anchor == (common.Hash{}) ||
		difficulty == nil ||
		difficulty.Sign() <= 0 {
		return common.Hash{}, nil, ErrInvalidWorkEpochV1
	}

	canonical, err := canonicalWorkEpochSeatsV1(seats)
	if err != nil {
		return common.Hash{}, nil, err
	}

	encoded, err := rlp.EncodeToBytes(workEpochRootPayloadV1{
		Domain:     workEpochRootDomainV1,
		Version:    RandomXWorkProtocolVersion,
		ChainID:    new(big.Int).Set(chainID),
		Epoch:      epoch,
		Anchor:     anchor,
		Difficulty: new(big.Int).Set(difficulty),
		Seats:      canonical,
	})
	if err != nil {
		return common.Hash{}, nil, err
	}

	return crypto.Keccak256Hash(encoded), canonical, nil
}

func NewWorkEpochSnapshotV1(
	chainID *big.Int,
	epoch uint64,
	anchor common.Hash,
	difficulty *big.Int,
	seats []WorkSeatV1,
) (*WorkEpochSnapshotV1, error) {
	root, canonical, err := WorkEpochRootV1(
		chainID,
		epoch,
		anchor,
		difficulty,
		seats,
	)
	if err != nil {
		return nil, err
	}
	if root == (common.Hash{}) {
		return nil, ErrInvalidWorkEpochRootV1
	}

	return &WorkEpochSnapshotV1{
		Epoch:      epoch,
		Anchor:     anchor,
		Difficulty: new(big.Int).Set(difficulty),
		Root:       root,
		Seats:      canonical,
	}, nil
}

func (s *WorkEpochSnapshotV1) Validate(
	chainID *big.Int,
) error {
	if s == nil ||
		s.Epoch == 0 ||
		s.Anchor == (common.Hash{}) ||
		s.Difficulty == nil ||
		s.Difficulty.Sign() <= 0 ||
		s.Root == (common.Hash{}) {
		return ErrInvalidWorkEpochV1
	}

	root, canonical, err := WorkEpochRootV1(
		chainID,
		s.Epoch,
		s.Anchor,
		s.Difficulty,
		s.Seats,
	)
	if err != nil {
		return err
	}
	if root != s.Root ||
		len(canonical) != len(s.Seats) {
		return ErrInvalidWorkEpochRootV1
	}
	for i := range canonical {
		if canonical[i] != s.Seats[i] {
			return ErrInvalidWorkEpochRootV1
		}
	}
	return nil
}
