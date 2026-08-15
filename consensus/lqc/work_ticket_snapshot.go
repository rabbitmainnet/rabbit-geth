package lqc

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/rlp"
)

var (
	workTicketSnapshotPrefix  = []byte("lqc-work-ticket-snapshot-v1-")
	workTicketStateRootDomain = []byte("RABBIT-LQC-WORK-TICKET-STATE-V1")

	ErrInvalidWorkTicketSnapshot       = errors.New("invalid lqc work ticket snapshot")
	ErrWorkTicketSnapshotChainMismatch = errors.New("lqc work ticket snapshot chain mismatch")
	ErrInactiveWorkTicketParticipant   = errors.New("inactive lqc work ticket participant")
)

type WorkTicketLaneEntry struct {
	Participant  common.Address
	Epoch        uint64
	NextSequence uint64
	Previous     common.Hash
}

type workTicketStateRootPayload struct {
	Domain  []byte
	ChainID *big.Int
	Epoch   uint64
	Anchor  common.Hash
	Lanes   []WorkTicketLaneEntry
}

// WorkTicketSnapshot is indexed by block hash. Stored snapshots are caches;
// a future activated header envelope remains the source of consensus truth.
type WorkTicketSnapshot struct {
	Number    uint64
	Hash      common.Hash
	Epoch     uint64
	Anchor    common.Hash
	StateRoot common.Hash
	Lanes     []WorkTicketLaneEntry
}

func canonicalWorkTicketLaneEntries(states map[common.Address]WorkTicketLaneState) ([]WorkTicketLaneEntry, error) {
	entries := make([]WorkTicketLaneEntry, 0, len(states))
	for participant, state := range states {
		if participant == (common.Address{}) || state.Epoch == 0 || state.NextSequence == 0 || state.Previous == (common.Hash{}) {
			return nil, ErrInvalidWorkTicketSnapshot
		}
		entries = append(entries, WorkTicketLaneEntry{
			Participant:  participant,
			Epoch:        state.Epoch,
			NextSequence: state.NextSequence,
			Previous:     state.Previous,
		})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Participant.Cmp(entries[j].Participant) < 0 })
	return entries, nil
}

func workTicketLaneEntriesEqual(left, right []WorkTicketLaneEntry) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func statesFromWorkTicketLaneEntries(entries []WorkTicketLaneEntry) (map[common.Address]WorkTicketLaneState, error) {
	states := make(map[common.Address]WorkTicketLaneState, len(entries))
	for _, entry := range entries {
		if entry.Participant == (common.Address{}) || entry.Epoch == 0 || entry.NextSequence == 0 || entry.Previous == (common.Hash{}) {
			return nil, ErrInvalidWorkTicketSnapshot
		}
		if _, exists := states[entry.Participant]; exists {
			return nil, ErrInvalidWorkTicketSnapshot
		}
		states[entry.Participant] = WorkTicketLaneState{
			Epoch:        entry.Epoch,
			NextSequence: entry.NextSequence,
			Previous:     entry.Previous,
		}
	}
	canonical, err := canonicalWorkTicketLaneEntries(states)
	if err != nil || !workTicketLaneEntriesEqual(canonical, entries) {
		return nil, ErrInvalidWorkTicketSnapshot
	}
	return states, nil
}

func WorkTicketStateRoot(chainID *big.Int, anchor common.Hash, epoch uint64, states map[common.Address]WorkTicketLaneState) (common.Hash, error) {
	if !validWorkTicketChainID(chainID) || anchor == (common.Hash{}) || epoch == 0 {
		return common.Hash{}, ErrInvalidWorkTicketSnapshot
	}
	entries, err := canonicalWorkTicketLaneEntries(states)
	if err != nil {
		return common.Hash{}, err
	}
	for _, entry := range entries {
		if entry.Epoch != epoch {
			return common.Hash{}, ErrWorkTicketEpochMismatch
		}
	}
	payload, err := rlp.EncodeToBytes(workTicketStateRootPayload{
		Domain:  workTicketStateRootDomain,
		ChainID: workTicketChainID(chainID),
		Epoch:   epoch,
		Anchor:  anchor,
		Lanes:   entries,
	})
	if err != nil {
		return common.Hash{}, fmt.Errorf("%w: %v", ErrInvalidWorkTicketSnapshot, err)
	}
	return crypto.Keccak256Hash(payload), nil
}

func NewWorkTicketSnapshot(number uint64, hash common.Hash, chainID *big.Int, anchor common.Hash, epoch uint64, participants []common.Address) (*WorkTicketSnapshot, error) {
	if hash == (common.Hash{}) || len(participants) == 0 {
		return nil, ErrInvalidWorkTicketSnapshot
	}
	states := make(map[common.Address]WorkTicketLaneState, len(participants))
	for _, participant := range participants {
		if participant == (common.Address{}) {
			return nil, ErrInvalidWorkTicketSnapshot
		}
		if _, exists := states[participant]; exists {
			return nil, ErrInvalidWorkTicketSnapshot
		}
		states[participant] = NewWorkTicketLaneState(chainID, anchor, epoch, participant)
	}
	return newWorkTicketSnapshot(number, hash, chainID, anchor, epoch, states)
}

func newWorkTicketSnapshot(number uint64, hash common.Hash, chainID *big.Int, anchor common.Hash, epoch uint64, states map[common.Address]WorkTicketLaneState) (*WorkTicketSnapshot, error) {
	entries, err := canonicalWorkTicketLaneEntries(states)
	if err != nil {
		return nil, err
	}
	root, err := WorkTicketStateRoot(chainID, anchor, epoch, states)
	if err != nil {
		return nil, err
	}
	return &WorkTicketSnapshot{Number: number, Hash: hash, Epoch: epoch, Anchor: anchor, StateRoot: root, Lanes: entries}, nil
}

func (s *WorkTicketSnapshot) States(chainID *big.Int) (map[common.Address]WorkTicketLaneState, error) {
	if s == nil || s.Hash == (common.Hash{}) || s.Epoch == 0 || s.Anchor == (common.Hash{}) || s.StateRoot == (common.Hash{}) {
		return nil, ErrInvalidWorkTicketSnapshot
	}
	states, err := statesFromWorkTicketLaneEntries(s.Lanes)
	if err != nil {
		return nil, err
	}
	root, err := WorkTicketStateRoot(chainID, s.Anchor, s.Epoch, states)
	if err != nil || root != s.StateRoot {
		return nil, ErrInvalidWorkTicketStateRoot
	}
	return states, nil
}

func reconcileWorkTicketParticipants(chainID *big.Int, anchor common.Hash, epoch uint64, states map[common.Address]WorkTicketLaneState, participants []common.Address) (map[common.Address]WorkTicketLaneState, map[common.Address]struct{}, error) {
	next := cloneWorkTicketLaneStates(states)
	active := make(map[common.Address]struct{}, len(participants))
	for _, participant := range participants {
		if participant == (common.Address{}) {
			return nil, nil, ErrInvalidWorkTicketSnapshot
		}
		if _, duplicate := active[participant]; duplicate {
			return nil, nil, ErrInvalidWorkTicketSnapshot
		}
		active[participant] = struct{}{}
		if _, exists := next[participant]; !exists {
			next[participant] = NewWorkTicketLaneState(chainID, anchor, epoch, participant)
		}
	}
	return next, active, nil
}

// ApplyEnvelope advances a snapshot only when block linkage, active registry
// membership, every proof and the committed post-state root agree.
func (s *WorkTicketSnapshot) ApplyEnvelope(chainID *big.Int, blockHash, parentHash common.Hash, activeParticipants []common.Address, blob []byte) (*WorkTicketSnapshot, error) {
	states, err := s.States(chainID)
	if err != nil {
		return nil, err
	}
	if s.Number == ^uint64(0) || blockHash == (common.Hash{}) || parentHash != s.Hash {
		return nil, ErrWorkTicketSnapshotChainMismatch
	}
	states, active, err := reconcileWorkTicketParticipants(chainID, s.Anchor, s.Epoch, states, activeParticipants)
	if err != nil {
		return nil, err
	}
	envelope, next, err := ValidateWorkTicketEnvelope(chainID, s.Number+1, s.Epoch, s.Anchor, states, blob)
	if err != nil {
		return nil, err
	}
	for _, ticket := range envelope.Tickets {
		if _, ok := active[ticket.Participant]; !ok {
			return nil, ErrInactiveWorkTicketParticipant
		}
	}
	return newWorkTicketSnapshot(s.Number+1, blockHash, chainID, s.Anchor, s.Epoch, next)
}

func StoreWorkTicketSnapshot(db ethdb.KeyValueWriter, chainID *big.Int, snapshot *WorkTicketSnapshot) error {
	if db == nil || snapshot == nil {
		return ErrInvalidWorkTicketSnapshot
	}
	if _, err := snapshot.States(chainID); err != nil {
		return err
	}
	blob, err := rlp.EncodeToBytes(snapshot)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidWorkTicketSnapshot, err)
	}
	return db.Put(workTicketSnapshotKey(snapshot.Hash), blob)
}

func LoadWorkTicketSnapshot(db ethdb.KeyValueReader, chainID *big.Int, hash common.Hash) (*WorkTicketSnapshot, error) {
	if db == nil || hash == (common.Hash{}) {
		return nil, ErrInvalidWorkTicketSnapshot
	}
	blob, err := db.Get(workTicketSnapshotKey(hash))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidWorkTicketSnapshot, err)
	}
	var snapshot WorkTicketSnapshot
	if err := rlp.DecodeBytes(blob, &snapshot); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidWorkTicketSnapshot, err)
	}
	if snapshot.Hash != hash {
		return nil, ErrWorkTicketSnapshotChainMismatch
	}
	if _, err := snapshot.States(chainID); err != nil {
		return nil, err
	}
	canonical, err := rlp.EncodeToBytes(&snapshot)
	if err != nil || !bytes.Equal(canonical, blob) {
		return nil, ErrInvalidWorkTicketSnapshot
	}
	return &snapshot, nil
}

func workTicketSnapshotKey(hash common.Hash) []byte {
	key := make([]byte, 0, len(workTicketSnapshotPrefix)+len(hash))
	key = append(key, workTicketSnapshotPrefix...)
	key = append(key, hash[:]...)
	return key
}
