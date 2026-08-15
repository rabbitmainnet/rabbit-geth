package lqc

import (
	"bytes"
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
)

var (
	ErrInvalidWorkChainSnapshotV1     = errors.New("invalid lqc work chain snapshot v1")
	ErrWorkChainSnapshotMismatchV1    = errors.New("lqc work chain snapshot mismatch v1")
	ErrUnexpectedWorkCommitV1         = errors.New("unexpected lqc work commit v1")
	ErrWorkCommitEpochMismatchV1      = errors.New("lqc work commit epoch mismatch v1")
	ErrWorkCommitContextMismatchV1    = errors.New("lqc work commit context mismatch v1")
	ErrWorkSelectionSnapshotMissingV1 = errors.New("lqc work selection snapshot missing v1")
)

var workChainStateRootDomainV1 = []byte("RABBIT-LQC-WORK-CHAIN-STATE-V1")

// WorkChainSnapshotV1 is a bounded, inactive model of Rabbit's canonical work
// state at one block hash.
//
// It keeps only:
//  1. the work epoch currently being committed, and
//  2. the most recently closed epoch used by role selection.
//
// Older work epochs are intentionally discarded from the live snapshot. A
// future implementation may persist historical checkpoints separately.
type WorkChainSnapshotV1 struct {
	Number      uint64
	Hash        common.Hash
	EpochLength uint64
	StateRoot   common.Hash

	CommitEpoch      uint64
	CommitAnchor     common.Hash
	CommitDifficulty *big.Int
	CommitSeats      []WorkSeatV1

	SelectionEpoch      uint64
	SelectionAnchor     common.Hash
	SelectionDifficulty *big.Int
	SelectionRoot       common.Hash
	SelectionSeats      []WorkSeatV1
}

type workChainStateRootPayloadV1 struct {
	Domain      []byte
	Version     uint8
	ChainID     *big.Int
	Number      uint64
	EpochLength uint64

	CommitEpoch      uint64
	CommitAnchor     common.Hash
	CommitDifficulty *big.Int
	CommitSeats      []WorkSeatV1

	SelectionEpoch      uint64
	SelectionAnchor     common.Hash
	SelectionDifficulty *big.Int
	SelectionRoot       common.Hash
	SelectionSeats      []WorkSeatV1
}

func cloneBigIntV1(input *big.Int) *big.Int {
	if input == nil {
		return nil
	}
	return new(big.Int).Set(input)
}

func cloneWorkSeatsV1(input []WorkSeatV1) []WorkSeatV1 {
	return append([]WorkSeatV1(nil), input...)
}

func (s *WorkChainSnapshotV1) clone() *WorkChainSnapshotV1 {
	if s == nil {
		return nil
	}
	return &WorkChainSnapshotV1{
		Number:              s.Number,
		Hash:                s.Hash,
		EpochLength:         s.EpochLength,
		StateRoot:           s.StateRoot,
		CommitEpoch:         s.CommitEpoch,
		CommitAnchor:        s.CommitAnchor,
		CommitDifficulty:    cloneBigIntV1(s.CommitDifficulty),
		CommitSeats:         cloneWorkSeatsV1(s.CommitSeats),
		SelectionEpoch:      s.SelectionEpoch,
		SelectionAnchor:     s.SelectionAnchor,
		SelectionDifficulty: cloneBigIntV1(s.SelectionDifficulty),
		SelectionRoot:       s.SelectionRoot,
		SelectionSeats:      cloneWorkSeatsV1(s.SelectionSeats),
	}
}

func validateOptionalWorkEpochStateV1(
	epoch uint64,
	anchor common.Hash,
	difficulty *big.Int,
	seats []WorkSeatV1,
) error {
	if epoch == 0 {
		if anchor != (common.Hash{}) ||
			difficulty != nil ||
			len(seats) != 0 {
			return ErrInvalidWorkChainSnapshotV1
		}
		return nil
	}

	if anchor == (common.Hash{}) ||
		difficulty == nil ||
		difficulty.Sign() <= 0 {
		return ErrInvalidWorkChainSnapshotV1
	}

	canonical, err := canonicalWorkEpochSeatsV1(seats)
	if err != nil {
		return err
	}
	if len(canonical) != len(seats) {
		return ErrInvalidWorkChainSnapshotV1
	}
	for index := range canonical {
		if canonical[index] != seats[index] {
			return ErrInvalidWorkChainSnapshotV1
		}
	}
	return nil
}

func WorkChainStateRootV1(
	chainID *big.Int,
	snapshot *WorkChainSnapshotV1,
) (common.Hash, error) {
	if chainID == nil || chainID.Sign() <= 0 ||
		snapshot == nil ||
		snapshot.Hash == (common.Hash{}) ||
		snapshot.EpochLength == 0 {
		return common.Hash{}, ErrInvalidWorkChainSnapshotV1
	}

	if err := validateOptionalWorkEpochStateV1(
		snapshot.CommitEpoch,
		snapshot.CommitAnchor,
		snapshot.CommitDifficulty,
		snapshot.CommitSeats,
	); err != nil {
		return common.Hash{}, err
	}

	if err := validateOptionalWorkEpochStateV1(
		snapshot.SelectionEpoch,
		snapshot.SelectionAnchor,
		snapshot.SelectionDifficulty,
		snapshot.SelectionSeats,
	); err != nil {
		return common.Hash{}, err
	}

	if snapshot.SelectionEpoch == 0 {
		if snapshot.SelectionRoot != (common.Hash{}) {
			return common.Hash{}, ErrInvalidWorkChainSnapshotV1
		}
	} else {
		root, canonical, err := WorkEpochRootV1(
			chainID,
			snapshot.SelectionEpoch,
			snapshot.SelectionAnchor,
			snapshot.SelectionDifficulty,
			snapshot.SelectionSeats,
		)
		if err != nil {
			return common.Hash{}, err
		}
		if root != snapshot.SelectionRoot ||
			len(canonical) != len(snapshot.SelectionSeats) {
			return common.Hash{}, ErrInvalidWorkChainSnapshotV1
		}
	}

	encoded, err := rlp.EncodeToBytes(workChainStateRootPayloadV1{
		Domain:              workChainStateRootDomainV1,
		Version:             RandomXWorkProtocolVersion,
		ChainID:             new(big.Int).Set(chainID),
		Number:              snapshot.Number,
		EpochLength:         snapshot.EpochLength,
		CommitEpoch:         snapshot.CommitEpoch,
		CommitAnchor:        snapshot.CommitAnchor,
		CommitDifficulty:    cloneBigIntV1(snapshot.CommitDifficulty),
		CommitSeats:         cloneWorkSeatsV1(snapshot.CommitSeats),
		SelectionEpoch:      snapshot.SelectionEpoch,
		SelectionAnchor:     snapshot.SelectionAnchor,
		SelectionDifficulty: cloneBigIntV1(snapshot.SelectionDifficulty),
		SelectionRoot:       snapshot.SelectionRoot,
		SelectionSeats:      cloneWorkSeatsV1(snapshot.SelectionSeats),
	})
	if err != nil {
		return common.Hash{}, err
	}
	return crypto.Keccak256Hash(encoded), nil
}

// NewWorkChainSnapshotV1 creates an empty canonical work state at a known block
// hash. For Rabbit's future mainnet wiring, the initial snapshot would normally
// be created at genesis.
func NewWorkChainSnapshotV1(
	chainID *big.Int,
	number uint64,
	hash common.Hash,
	epochLength uint64,
) (*WorkChainSnapshotV1, error) {
	if chainID == nil || chainID.Sign() <= 0 ||
		hash == (common.Hash{}) ||
		epochLength == 0 {
		return nil, ErrInvalidWorkChainSnapshotV1
	}

	snapshot := &WorkChainSnapshotV1{
		Number:      number,
		Hash:        hash,
		EpochLength: epochLength,
	}
	root, err := WorkChainStateRootV1(chainID, snapshot)
	if err != nil {
		return nil, err
	}
	snapshot.StateRoot = root
	return snapshot, nil
}

func (s *WorkChainSnapshotV1) Validate(chainID *big.Int) error {
	if s == nil || s.StateRoot == (common.Hash{}) {
		return ErrInvalidWorkChainSnapshotV1
	}
	root, err := WorkChainStateRootV1(chainID, s)
	if err != nil {
		return err
	}
	if root != s.StateRoot {
		return ErrInvalidWorkChainSnapshotV1
	}
	return nil
}

// SelectionSnapshotV1 returns the closed work epoch required by blockNumber.
// If the chain is still in the first two epochs there is intentionally no work
// selection source yet.
func (s *WorkChainSnapshotV1) SelectionSnapshotV1(
	chainID *big.Int,
	blockNumber uint64,
) (*WorkEpochSnapshotV1, bool, error) {
	if s == nil || s.EpochLength == 0 {
		return nil, false, ErrInvalidWorkChainSnapshotV1
	}
	want, ok, err := WorkSelectionSourceEpochV1(
		blockNumber,
		s.EpochLength,
	)
	if err != nil || !ok {
		return nil, ok, err
	}
	if s.SelectionEpoch != want ||
		s.SelectionAnchor == (common.Hash{}) ||
		s.SelectionDifficulty == nil ||
		s.SelectionRoot == (common.Hash{}) {
		return nil, false, ErrWorkSelectionSnapshotMissingV1
	}

	out := &WorkEpochSnapshotV1{
		Epoch:      s.SelectionEpoch,
		Anchor:     s.SelectionAnchor,
		Difficulty: cloneBigIntV1(s.SelectionDifficulty),
		Root:       s.SelectionRoot,
		Seats:      cloneWorkSeatsV1(s.SelectionSeats),
	}
	return out, true, out.Validate(chainID)
}

// ApplyVerifiedBlockV1 advances work state by exactly one canonical child.
//
// verified must already have passed:
//
//	RandomX recomputation,
//	target validation,
//	participant authorization.
//
// commitAnchor and commitDifficulty must describe the work epoch being committed
// during this block. They are ignored only while there is no commit target
// (epoch 1).
func (s *WorkChainSnapshotV1) ApplyVerifiedBlockV1(
	chainID *big.Int,
	blockNumber uint64,
	blockHash common.Hash,
	parentHash common.Hash,
	commitAnchor common.Hash,
	commitDifficulty *big.Int,
	verified []VerifiedRandomXWorkTicketV1,
) (*WorkChainSnapshotV1, error) {
	if err := s.Validate(chainID); err != nil {
		return nil, err
	}
	if s.Number == ^uint64(0) ||
		blockNumber != s.Number+1 ||
		parentHash != s.Hash ||
		blockHash == (common.Hash{}) {
		return nil, ErrWorkChainSnapshotMismatchV1
	}

	next := s.clone()
	next.Number = blockNumber
	next.Hash = blockHash

	commitEpoch, hasCommit, err := WorkCommitTargetEpochV1(
		blockNumber,
		s.EpochLength,
	)
	if err != nil {
		return nil, err
	}

	if !hasCommit {
		if len(verified) != 0 {
			return nil, ErrUnexpectedWorkCommitV1
		}
		if commitAnchor != (common.Hash{}) ||
			commitDifficulty != nil {
			return nil, ErrUnexpectedWorkCommitV1
		}
	} else {
		if commitAnchor == (common.Hash{}) ||
			commitDifficulty == nil ||
			commitDifficulty.Sign() <= 0 {
			return nil, ErrWorkCommitContextMismatchV1
		}

		if next.CommitEpoch == 0 {
			next.CommitEpoch = commitEpoch
			next.CommitAnchor = commitAnchor
			next.CommitDifficulty = new(big.Int).Set(commitDifficulty)
			next.CommitSeats = nil
		} else if next.CommitEpoch != commitEpoch {
			return nil, ErrWorkCommitEpochMismatchV1
		}

		if next.CommitAnchor != commitAnchor ||
			next.CommitDifficulty.Cmp(commitDifficulty) != 0 {
			return nil, ErrWorkCommitContextMismatchV1
		}

		newVerified := make([]VerifiedRandomXWorkTicketV1, 0, len(verified))
		for _, item := range verified {
			if item.Ticket.Epoch != commitEpoch {
				return nil, ErrWorkCommitEpochMismatchV1
			}
			newVerified = append(newVerified, item)
		}

		newSeats, err := CanonicalWorkSeatsV1(newVerified)
		if err != nil {
			return nil, err
		}

		combined := make(
			[]WorkSeatV1,
			0,
			len(next.CommitSeats)+len(newSeats),
		)
		combined = append(combined, next.CommitSeats...)
		combined = append(combined, newSeats...)

		next.CommitSeats, err = canonicalWorkEpochSeatsV1(combined)
		if err != nil {
			return nil, err
		}

		// The last block of chain epoch N+1 closes work epoch N. The resulting
		// closed snapshot is therefore already available to block 1 of N+2.
		if blockNumber%s.EpochLength == 0 {
			closed, err := NewWorkEpochSnapshotV1(
				chainID,
				next.CommitEpoch,
				next.CommitAnchor,
				next.CommitDifficulty,
				next.CommitSeats,
			)
			if err != nil {
				return nil, err
			}

			next.SelectionEpoch = closed.Epoch
			next.SelectionAnchor = closed.Anchor
			next.SelectionDifficulty = cloneBigIntV1(closed.Difficulty)
			next.SelectionRoot = closed.Root
			next.SelectionSeats = cloneWorkSeatsV1(closed.Seats)

			next.CommitEpoch = 0
			next.CommitAnchor = common.Hash{}
			next.CommitDifficulty = nil
			next.CommitSeats = nil
		}
	}

	root, err := WorkChainStateRootV1(chainID, next)
	if err != nil {
		return nil, err
	}
	next.StateRoot = root
	return next, nil
}

// EqualConsensusStateV1 compares only consensus-relevant work state. Block hash
// is intentionally included through snapshot linkage but not through StateRoot,
// avoiding a circular dependency if StateRoot later becomes a header field.
func (s *WorkChainSnapshotV1) EqualConsensusStateV1(
	other *WorkChainSnapshotV1,
) bool {
	if s == nil || other == nil {
		return s == other
	}
	if s.Number != other.Number ||
		s.Hash != other.Hash ||
		s.EpochLength != other.EpochLength ||
		s.StateRoot != other.StateRoot ||
		s.CommitEpoch != other.CommitEpoch ||
		s.CommitAnchor != other.CommitAnchor ||
		s.SelectionEpoch != other.SelectionEpoch ||
		s.SelectionAnchor != other.SelectionAnchor ||
		s.SelectionRoot != other.SelectionRoot {
		return false
	}
	if (s.CommitDifficulty == nil) != (other.CommitDifficulty == nil) ||
		(s.SelectionDifficulty == nil) != (other.SelectionDifficulty == nil) {
		return false
	}
	if s.CommitDifficulty != nil &&
		s.CommitDifficulty.Cmp(other.CommitDifficulty) != 0 {
		return false
	}
	if s.SelectionDifficulty != nil &&
		s.SelectionDifficulty.Cmp(other.SelectionDifficulty) != 0 {
		return false
	}
	if !bytes.Equal(workSeatBytesV1(s.CommitSeats), workSeatBytesV1(other.CommitSeats)) ||
		!bytes.Equal(workSeatBytesV1(s.SelectionSeats), workSeatBytesV1(other.SelectionSeats)) {
		return false
	}
	return true
}

func workSeatBytesV1(seats []WorkSeatV1) []byte {
	blob, _ := rlp.EncodeToBytes(seats)
	return blob
}
