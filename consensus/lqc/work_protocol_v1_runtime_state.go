package lqc

import (
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
)

var (
	ErrInvalidWorkDifficultyStateV1 = errors.New(
		"invalid lqc work difficulty state v1",
	)
	ErrWorkDifficultyEpochUnavailableV1 = errors.New(
		"lqc work difficulty epoch unavailable v1",
	)
	ErrWorkDifficultyClosedEpochMismatchV1 = errors.New(
		"lqc closed work epoch difficulty mismatch v1",
	)
	ErrInvalidCanonicalWorkRuntimeStateV1 = errors.New(
		"invalid canonical lqc work runtime state v1",
	)
)

var (
	workDifficultyStateRootDomainV1 = []byte(
		"RABBIT-LQC-WORK-DIFFICULTY-STATE-V1",
	)
	canonicalWorkRuntimeRootDomainV1 = []byte(
		"RABBIT-LQC-WORK-RUNTIME-STATE-V1",
	)
)

// WorkDifficultyStateV1 keeps exactly the two parity chains required by the
// delayed protocol.
//
// Work epoch N is committed/closed one chain epoch later, so its observed count
// can first affect work epoch N+2. Odd and even difficulties therefore evolve
// independently:
//
//	D3 <- close(E1), D5 <- close(E3), ...
//	D4 <- close(E2), D6 <- close(E4), ...
//
// Keeping both parity heads prevents the canonical difficulty needed by a
// future commit window from being discarded when the latest closed work epoch
// advances.
type WorkDifficultyStateV1 struct {
	OddEpoch       uint64
	OddDifficulty  *big.Int
	EvenEpoch      uint64
	EvenDifficulty *big.Int
	StateRoot      common.Hash
}

type workDifficultyStateRootPayloadV1 struct {
	Domain         []byte
	Version        uint8
	ChainID        *big.Int
	OddEpoch       uint64
	OddDifficulty  *big.Int
	EvenEpoch      uint64
	EvenDifficulty *big.Int
}

func NewWorkDifficultyStateV1(
	chainID *big.Int,
	initialDifficulty *big.Int,
) (*WorkDifficultyStateV1, error) {
	if chainID == nil ||
		chainID.Sign() <= 0 ||
		initialDifficulty == nil ||
		initialDifficulty.Sign() <= 0 {
		return nil, ErrInvalidWorkDifficultyStateV1
	}

	state := &WorkDifficultyStateV1{
		OddEpoch:       1,
		OddDifficulty:  new(big.Int).Set(initialDifficulty),
		EvenEpoch:      2,
		EvenDifficulty: new(big.Int).Set(initialDifficulty),
	}
	root, err := WorkDifficultyStateRootV1(chainID, state)
	if err != nil {
		return nil, err
	}
	state.StateRoot = root
	return state, nil
}

func (s *WorkDifficultyStateV1) clone() *WorkDifficultyStateV1 {
	if s == nil {
		return nil
	}
	return &WorkDifficultyStateV1{
		OddEpoch:       s.OddEpoch,
		OddDifficulty:  cloneBigIntV1(s.OddDifficulty),
		EvenEpoch:      s.EvenEpoch,
		EvenDifficulty: cloneBigIntV1(s.EvenDifficulty),
		StateRoot:      s.StateRoot,
	}
}

func WorkDifficultyStateRootV1(
	chainID *big.Int,
	state *WorkDifficultyStateV1,
) (common.Hash, error) {
	if chainID == nil ||
		chainID.Sign() <= 0 ||
		state == nil ||
		state.OddEpoch == 0 ||
		state.OddEpoch%2 != 1 ||
		state.OddDifficulty == nil ||
		state.OddDifficulty.Sign() <= 0 ||
		state.EvenEpoch == 0 ||
		state.EvenEpoch%2 != 0 ||
		state.EvenDifficulty == nil ||
		state.EvenDifficulty.Sign() <= 0 {
		return common.Hash{}, ErrInvalidWorkDifficultyStateV1
	}

	blob, err := rlp.EncodeToBytes(workDifficultyStateRootPayloadV1{
		Domain:         workDifficultyStateRootDomainV1,
		Version:        RandomXWorkProtocolVersion,
		ChainID:        new(big.Int).Set(chainID),
		OddEpoch:       state.OddEpoch,
		OddDifficulty:  cloneBigIntV1(state.OddDifficulty),
		EvenEpoch:      state.EvenEpoch,
		EvenDifficulty: cloneBigIntV1(state.EvenDifficulty),
	})
	if err != nil {
		return common.Hash{}, err
	}
	return crypto.Keccak256Hash(blob), nil
}

func (s *WorkDifficultyStateV1) Validate(chainID *big.Int) error {
	if s == nil || s.StateRoot == (common.Hash{}) {
		return ErrInvalidWorkDifficultyStateV1
	}
	root, err := WorkDifficultyStateRootV1(chainID, s)
	if err != nil {
		return err
	}
	if root != s.StateRoot {
		return ErrInvalidWorkDifficultyStateV1
	}
	return nil
}

// DifficultyForEpochV1 returns only an epoch whose difficulty has already been
// canonically scheduled. There is no caller override.
func (s *WorkDifficultyStateV1) DifficultyForEpochV1(
	chainID *big.Int,
	epoch uint64,
) (*big.Int, error) {
	if err := s.Validate(chainID); err != nil {
		return nil, err
	}

	switch {
	case epoch == s.OddEpoch:
		return cloneBigIntV1(s.OddDifficulty), nil
	case epoch == s.EvenEpoch:
		return cloneBigIntV1(s.EvenDifficulty), nil
	default:
		return nil, ErrWorkDifficultyEpochUnavailableV1
	}
}

// AdvanceClosedEpochV1 consumes one CLOSED canonical work epoch and schedules
// exactly N+2. The closed snapshot difficulty itself must equal the canonical
// difficulty previously scheduled for N.
func (s *WorkDifficultyStateV1) AdvanceClosedEpochV1(
	chainID *big.Int,
	closed *WorkEpochSnapshotV1,
) (*WorkDifficultyStateV1, error) {
	if err := s.Validate(chainID); err != nil {
		return nil, err
	}
	if closed == nil {
		return nil, ErrInvalidWorkDifficultyStateV1
	}
	if err := closed.Validate(chainID); err != nil {
		return nil, err
	}

	current, err := s.DifficultyForEpochV1(chainID, closed.Epoch)
	if err != nil {
		return nil, err
	}
	if closed.Difficulty == nil ||
		current.Cmp(closed.Difficulty) != 0 {
		return nil, ErrWorkDifficultyClosedEpochMismatchV1
	}

	nextDifficulty, err := NextWorkDifficultyV1(
		current,
		uint64(len(closed.Seats)),
	)
	if err != nil {
		return nil, err
	}
	if closed.Epoch > ^uint64(0)-2 {
		return nil, ErrInvalidWorkDifficultyStateV1
	}

	next := s.clone()
	if closed.Epoch%2 == 1 {
		next.OddEpoch = closed.Epoch + 2
		next.OddDifficulty = nextDifficulty
	} else {
		next.EvenEpoch = closed.Epoch + 2
		next.EvenDifficulty = nextDifficulty
	}

	root, err := WorkDifficultyStateRootV1(chainID, next)
	if err != nil {
		return nil, err
	}
	next.StateRoot = root
	return next, nil
}

// CanonicalWorkRuntimeStateV1 binds the existing bounded work snapshot to the
// delayed canonical difficulty schedule. Header V3 can later commit this single
// StateRoot without accepting a process-local/caller-selected difficulty.
type CanonicalWorkRuntimeStateV1 struct {
	Work       *WorkChainSnapshotV1
	Difficulty *WorkDifficultyStateV1
	StateRoot  common.Hash
}

type canonicalWorkRuntimeRootPayloadV1 struct {
	Domain         []byte
	Version        uint8
	ChainID        *big.Int
	WorkStateRoot  common.Hash
	DifficultyRoot common.Hash
}

func NewCanonicalWorkRuntimeStateV1(
	chainID *big.Int,
	number uint64,
	hash common.Hash,
	epochLength uint64,
	initialDifficulty *big.Int,
) (*CanonicalWorkRuntimeStateV1, error) {
	work, err := NewWorkChainSnapshotV1(
		chainID,
		number,
		hash,
		epochLength,
	)
	if err != nil {
		return nil, err
	}
	difficulty, err := NewWorkDifficultyStateV1(
		chainID,
		initialDifficulty,
	)
	if err != nil {
		return nil, err
	}

	state := &CanonicalWorkRuntimeStateV1{
		Work:       work,
		Difficulty: difficulty,
	}
	root, err := CanonicalWorkRuntimeStateRootV1(chainID, state)
	if err != nil {
		return nil, err
	}
	state.StateRoot = root
	return state, nil
}

func CanonicalWorkRuntimeStateRootV1(
	chainID *big.Int,
	state *CanonicalWorkRuntimeStateV1,
) (common.Hash, error) {
	if chainID == nil ||
		chainID.Sign() <= 0 ||
		state == nil ||
		state.Work == nil ||
		state.Difficulty == nil {
		return common.Hash{}, ErrInvalidCanonicalWorkRuntimeStateV1
	}
	if err := state.Work.Validate(chainID); err != nil {
		return common.Hash{}, err
	}
	if err := state.Difficulty.Validate(chainID); err != nil {
		return common.Hash{}, err
	}

	blob, err := rlp.EncodeToBytes(canonicalWorkRuntimeRootPayloadV1{
		Domain:         canonicalWorkRuntimeRootDomainV1,
		Version:        RandomXWorkProtocolVersion,
		ChainID:        new(big.Int).Set(chainID),
		WorkStateRoot:  state.Work.StateRoot,
		DifficultyRoot: state.Difficulty.StateRoot,
	})
	if err != nil {
		return common.Hash{}, err
	}
	return crypto.Keccak256Hash(blob), nil
}

func (s *CanonicalWorkRuntimeStateV1) Validate(
	chainID *big.Int,
) error {
	if s == nil || s.StateRoot == (common.Hash{}) {
		return ErrInvalidCanonicalWorkRuntimeStateV1
	}
	root, err := CanonicalWorkRuntimeStateRootV1(chainID, s)
	if err != nil {
		return err
	}
	if root != s.StateRoot {
		return ErrInvalidCanonicalWorkRuntimeStateV1
	}
	return nil
}

// CommitDifficultyV1 exposes the only difficulty that may be used to validate
// tickets being committed in blockNumber.
func (s *CanonicalWorkRuntimeStateV1) CommitDifficultyV1(
	chainID *big.Int,
	blockNumber uint64,
) (*big.Int, bool, error) {
	if err := s.Validate(chainID); err != nil {
		return nil, false, err
	}
	epoch, ok, err := WorkCommitTargetEpochV1(
		blockNumber,
		s.Work.EpochLength,
	)
	if err != nil || !ok {
		return nil, ok, err
	}
	difficulty, err := s.Difficulty.DifficultyForEpochV1(
		chainID,
		epoch,
	)
	if err != nil {
		return nil, false, err
	}
	return difficulty, true, nil
}

// ApplyVerifiedBlockV1 deliberately has NO difficulty argument.
//
// commitChallengeAnchor is the canonical challenge anchor for the work epoch.
// DatasetAnchor remains independently derivable from the epoch schedule and
// canonical chain when RandomX verification happens before this state update.
func (s *CanonicalWorkRuntimeStateV1) ApplyVerifiedBlockV1(
	chainID *big.Int,
	blockNumber uint64,
	blockHash common.Hash,
	parentHash common.Hash,
	commitChallengeAnchor common.Hash,
	verified []VerifiedRandomXWorkTicketV1,
) (*CanonicalWorkRuntimeStateV1, error) {
	if err := s.Validate(chainID); err != nil {
		return nil, err
	}

	commitDifficulty, hasCommit, err := s.CommitDifficultyV1(
		chainID,
		blockNumber,
	)
	if err != nil {
		return nil, err
	}
	if !hasCommit {
		commitDifficulty = nil
	}

	nextWork, err := s.Work.ApplyVerifiedBlockV1(
		chainID,
		blockNumber,
		blockHash,
		parentHash,
		commitChallengeAnchor,
		commitDifficulty,
		verified,
	)
	if err != nil {
		return nil, err
	}

	nextDifficulty := s.Difficulty.clone()

	// A changed SelectionEpoch means this block just closed one canonical work
	// epoch. Retarget the same parity immediately for N+2.
	if nextWork.SelectionEpoch != 0 &&
		nextWork.SelectionEpoch != s.Work.SelectionEpoch {
		previousParticipants := make(
			map[common.Address]struct{},
			len(s.Work.SelectionSeats),
		)
		for _, seat := range s.Work.SelectionSeats {
			previousParticipants[seat.Participant] = struct{}{}
		}
		admissions := make([]WorkSeatV1, 0, len(nextWork.SelectionSeats))
		for _, seat := range nextWork.SelectionSeats {
			if _, exists := previousParticipants[seat.Participant]; !exists {
				admissions = append(admissions, seat)
			}
		}
		closed, err := NewWorkEpochSnapshotV1(
			chainID,
			nextWork.SelectionEpoch,
			nextWork.SelectionAnchor,
			nextWork.SelectionDifficulty,
			admissions,
		)
		if err != nil {
			return nil, err
		}
		nextDifficulty, err = nextDifficulty.AdvanceClosedEpochV1(
			chainID,
			closed,
		)
		if err != nil {
			return nil, err
		}
	}

	next := &CanonicalWorkRuntimeStateV1{
		Work:       nextWork,
		Difficulty: nextDifficulty,
	}
	root, err := CanonicalWorkRuntimeStateRootV1(chainID, next)
	if err != nil {
		return nil, err
	}
	next.StateRoot = root
	return next, nil
}
