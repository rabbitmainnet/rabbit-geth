package lqc

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/rlp"
)

var (
	registrySnapshotPrefix = []byte("lqc-registry-snapshot-v1-")

	ErrInvalidRegistrySnapshot       = errors.New("invalid lqc registry snapshot")
	ErrRegistrySnapshotChainMismatch = errors.New("lqc registry snapshot chain mismatch")
	ErrRegistryRootMismatch          = errors.New("lqc registry root mismatch")
	ErrUnauthorizedRegistryProducer  = errors.New("unauthorized lqc registry producer")
)

// RegistrySnapshotRules contains only rules needed to derive registry state.
// Queue size and reward parameters deliberately do not belong here.
type RegistrySnapshotRules struct {
	ProofDifficulty uint64
	ActivationDelay uint64
	HeartbeatWindow uint64
	HeartbeatGrace  uint64
	JailBlocks      uint64
	MaxMissedTurns  uint64
}

// RegistrySnapshot is derived from canonical headers and indexed by block
// hash. The database representation is only a cache; headers remain the source
// of consensus truth.
type RegistrySnapshot struct {
	Number       uint64
	Hash         common.Hash
	RegistryRoot common.Hash
	Participants []CanonicalParticipant
}

func NewGenesisRegistrySnapshot(hash common.Hash, participants []common.Address) (*RegistrySnapshot, error) {
	return NewBootstrapRegistrySnapshot(0, hash, participants)
}

func NewBootstrapRegistrySnapshot(number uint64, hash common.Hash, participants []common.Address) (*RegistrySnapshot, error) {
	if hash == (common.Hash{}) || len(participants) == 0 {
		return nil, ErrInvalidRegistrySnapshot
	}
	registry := NewCanonicalRegistry()
	for _, address := range participants {
		if address == (common.Address{}) {
			return nil, ErrInvalidRegistryAddress
		}
		if _, exists := registry.entries[address]; exists {
			return nil, ErrDuplicateRegistryOperation
		}
		registry.entries[address] = CanonicalParticipant{
			Address:       address,
			RegisteredAt:  0,
			LastHeartbeat: number,
			Sequence:      0,
			Active:        true,
		}
	}
	return newRegistrySnapshot(number, hash, registry), nil
}

func newRegistrySnapshot(number uint64, hash common.Hash, registry *CanonicalRegistry) *RegistrySnapshot {
	if registry == nil {
		registry = NewCanonicalRegistry()
	}
	return &RegistrySnapshot{
		Number:       number,
		Hash:         hash,
		RegistryRoot: registry.Root(),
		Participants: registry.Participants(),
	}
}

func (s *RegistrySnapshot) Registry() (*CanonicalRegistry, error) {
	if s == nil || s.Hash == (common.Hash{}) || s.RegistryRoot == (common.Hash{}) {
		return nil, ErrInvalidRegistrySnapshot
	}
	registry := NewCanonicalRegistry()
	for _, participant := range s.Participants {
		if participant.Address == (common.Address{}) {
			return nil, ErrInvalidRegistryAddress
		}
		if _, exists := registry.entries[participant.Address]; exists {
			return nil, ErrDuplicateRegistryOperation
		}
		registry.entries[participant.Address] = participant
	}
	canonical := registry.Participants()
	if !canonicalParticipantsEqual(canonical, s.Participants) {
		return nil, ErrInvalidRegistrySnapshot
	}
	if registry.Root() != s.RegistryRoot {
		return nil, ErrRegistryRootMismatch
	}
	return registry, nil
}

func (s *RegistrySnapshot) ApplyHeader(chainID *big.Int, rules RegistrySnapshotRules, header *types.Header) (*RegistrySnapshot, error) {
	registry, err := s.Registry()
	if err != nil {
		return nil, err
	}
	if header == nil || header.Number == nil || s.Number == ^uint64(0) || header.Number.Uint64() != s.Number+1 || header.ParentHash != s.Hash {
		return nil, ErrRegistrySnapshotChainMismatch
	}
	if rules.ProofDifficulty == 0 {
		return nil, ErrInvalidLightHashProof
	}
	blockNumber := header.Number.Uint64()
	envelope, err := ValidateRegistryHeaderExtra(chainID, blockNumber, rules.ProofDifficulty, header.Extra)
	if err != nil {
		return nil, err
	}
	ordered := registry.OrderedParticipantsForBlock(
		header.ParentHash,
		blockNumber,
		rules.ActivationDelay,
		rules.HeartbeatWindow,
		rules.HeartbeatGrace,
	)
	selection := HybridSelection{Ordered: ordered}
	allowed, queuePos := IsAuthorAllowed(selection, header.Coinbase)
	if header.Coinbase == (common.Address{}) || !allowed {
		return nil, ErrUnauthorizedRegistryProducer
	}

	for index := 0; index < queuePos; index++ {
		if err := registry.ApplyMissedTurn(
			selection.Ordered[index].Address,
			blockNumber,
			rules.MaxMissedTurns,
			rules.JailBlocks,
		); err != nil {
			return nil, err
		}
	}

	if err := registry.MarkProducerHeartbeat(header.Coinbase, blockNumber); err != nil {
		return nil, err
	}
	for _, operation := range envelope.Operations {
		if err := registry.ApplyOperation(chainID, blockNumber, rules.ProofDifficulty, operation); err != nil {
			return nil, err
		}
	}
	if registry.Root() != envelope.RegistryRoot {
		return nil, ErrRegistryRootMismatch
	}
	return newRegistrySnapshot(blockNumber, header.Hash(), registry), nil
}

func ReconstructRegistrySnapshot(genesis *RegistrySnapshot, chainID *big.Int, rules RegistrySnapshotRules, headers []*types.Header) (*RegistrySnapshot, error) {
	if genesis == nil {
		return nil, ErrInvalidRegistrySnapshot
	}
	snapshot := genesis
	var err error
	for _, header := range headers {
		snapshot, err = snapshot.ApplyHeader(chainID, rules, header)
		if err != nil {
			return nil, err
		}
	}
	return snapshot, nil
}

func StoreRegistrySnapshot(db ethdb.KeyValueWriter, snapshot *RegistrySnapshot) error {
	if db == nil || snapshot == nil {
		return ErrInvalidRegistrySnapshot
	}
	if _, err := snapshot.Registry(); err != nil {
		return err
	}
	blob, err := rlp.EncodeToBytes(snapshot)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidRegistrySnapshot, err)
	}
	return db.Put(registrySnapshotKey(snapshot.Hash), blob)
}

func LoadRegistrySnapshot(db ethdb.KeyValueReader, hash common.Hash) (*RegistrySnapshot, error) {
	if db == nil || hash == (common.Hash{}) {
		return nil, ErrInvalidRegistrySnapshot
	}
	blob, err := db.Get(registrySnapshotKey(hash))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidRegistrySnapshot, err)
	}
	var snapshot RegistrySnapshot
	if err := rlp.DecodeBytes(blob, &snapshot); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidRegistrySnapshot, err)
	}
	if snapshot.Hash != hash {
		return nil, ErrRegistrySnapshotChainMismatch
	}
	if _, err := snapshot.Registry(); err != nil {
		return nil, err
	}
	canonical, err := rlp.EncodeToBytes(&snapshot)
	if err != nil || !bytes.Equal(canonical, blob) {
		return nil, ErrInvalidRegistrySnapshot
	}
	return &snapshot, nil
}

func registrySnapshotKey(hash common.Hash) []byte {
	key := make([]byte, 0, len(registrySnapshotPrefix)+len(hash))
	key = append(key, registrySnapshotPrefix...)
	key = append(key, hash[:]...)
	return key
}

func canonicalParticipantsEqual(left, right []CanonicalParticipant) bool {
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
