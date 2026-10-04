package lqc

import (
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
)

// Canonical availability-based committee construction for the V2 fork.
func rabbitVRFAvailableCommitteeWithSeedV2(
	chainID *big.Int,
	snapshot *WorkEpochSnapshotV1,
	registry *RegistrySnapshot,
	referenceBlock, heartbeatWindow, maxSize uint64,
	entropy common.Hash,
) (common.Hash, common.Hash, []RabbitVRFCommitteeMemberV1, error) {
	fail := errors.New("invalid VRF V2 committee context")
	if chainID == nil || chainID.Sign() <= 0 || snapshot == nil ||
		registry == nil || registry.Hash == (common.Hash{}) ||
		registry.RegistryRoot == (common.Hash{}) ||
		entropy == (common.Hash{}) || maxSize < 1 || maxSize > 128 {
		return common.Hash{}, common.Hash{}, nil, fail
	}
	if err := snapshot.Validate(chainID); err != nil {
		return common.Hash{}, common.Hash{}, nil, err
	}
	candidates, err := rabbitVRFAvailableCandidatesV2(
		snapshot.Seats, registry, referenceBlock, heartbeatWindow,
	)
	if err != nil {
		return common.Hash{}, common.Hash{}, nil, err
	}
	if len(candidates) == 0 {
		return common.Hash{}, common.Hash{}, nil, errors.New("insufficient VRF V2 candidates")
	}

	// Bind availability to one immutable canonical reference.
	encoded, err := rlp.EncodeToBytes(struct {
		Domain          []byte
		ChainID         *big.Int
		SourceEpoch     uint64
		SelectionRoot   common.Hash
		ReferenceBlock  uint64
		ReferenceHash   common.Hash
		RegistryRoot    common.Hash
		HeartbeatWindow uint64
		MaxSize         uint64
		Entropy         common.Hash
		Candidates      []WorkSeatV1
	}{
		[]byte("RABBIT-VRF-AVAILABLE-COMMITTEE-V2"),
		new(big.Int).Set(chainID), snapshot.Epoch, snapshot.Root,
		referenceBlock, registry.Hash, registry.RegistryRoot,
		heartbeatWindow, maxSize, entropy, candidates,
	})
	if err != nil {
		return common.Hash{}, common.Hash{}, nil, err
	}
	seed := crypto.Keccak256Hash(encoded)
	size := RabbitVRFCommitteeSizeV1(uint64(len(candidates)), 32, maxSize)
	committee, err := DeterministicallySelectWorkSeatsV1(candidates, seed, size)
	if err != nil {
		return common.Hash{}, common.Hash{}, nil, err
	}
	root, members, err := RabbitVRFCommitteeRootV1(chainID, snapshot.Epoch, snapshot.Root, seed, committee)
	return root, seed, members, err
}

func rabbitVRFAvailableCommitteeV2(chainID *big.Int, snapshot *WorkEpochSnapshotV1,
	registry *RegistrySnapshot, referenceBlock, heartbeatWindow, maxSize uint64,
	entropy common.Hash) (common.Hash, []RabbitVRFCommitteeMemberV1, error) {
	root, _, members, err := rabbitVRFAvailableCommitteeWithSeedV2(chainID, snapshot,
		registry, referenceBlock, heartbeatWindow, maxSize, entropy)
	return root, members, err
}
