package lqc

import (
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
)

const RabbitVRFCommitteeVersionV1 uint8 = 1

var ErrInvalidRabbitVRFCommitteeV1 = errors.New(
	"invalid rabbit vrf committee v1",
)

var rabbitVRFCommitteeSeedDomainV1 = []byte(
	"RABBIT-VRF-COMMITTEE-SEED-V1",
)

type rabbitVRFCommitteeSeedPayloadV1 struct {
	Domain        []byte
	Version       uint8
	ChainID       *big.Int
	SourceEpoch   uint64
	SelectionRoot common.Hash
	Entropy       common.Hash
}

// RabbitVRFCommitteeSeedV1 derives a VRF-specific seed from an already-closed
// canonical Work selection context.
//
// Block number, parent hash, producer identity, heartbeat state and mutable
// per-block liveness state are deliberately absent. The resulting seed is
// stable for the same closed source epoch.
func RabbitVRFCommitteeSeedV1(
	chainID *big.Int,
	sourceEpoch uint64,
	selectionRoot common.Hash,
	entropy common.Hash,
) (common.Hash, error) {
	if chainID == nil ||
		chainID.Sign() <= 0 ||
		sourceEpoch == 0 ||
		selectionRoot == (common.Hash{}) ||
		entropy == (common.Hash{}) {
		return common.Hash{}, ErrInvalidRabbitVRFCommitteeV1
	}

	blob, err := rlp.EncodeToBytes(rabbitVRFCommitteeSeedPayloadV1{
		Domain:        rabbitVRFCommitteeSeedDomainV1,
		Version:       RabbitVRFCommitteeVersionV1,
		ChainID:       new(big.Int).Set(chainID),
		SourceEpoch:   sourceEpoch,
		SelectionRoot: selectionRoot,
		Entropy:       entropy,
	})
	if err != nil {
		return common.Hash{}, err
	}

	seed := crypto.Keccak256Hash(blob)
	if seed == (common.Hash{}) {
		return common.Hash{}, ErrInvalidRabbitVRFCommitteeV1
	}
	return seed, nil
}

// RabbitVRFCommitteeSizeV1 applies Rabbit's existing dynamic committee sizing
// policy, then caps it to the canonical seats actually present.
//
// Unlike block-role selection, VRF committee derivation does not reserve a
// producer or fallback prefix.
func RabbitVRFCommitteeSizeV1(
	seatCount uint64,
	minSize uint64,
	maxSize uint64,
) uint64 {
	if seatCount == 0 {
		return 0
	}

	size := ComputeCommitteeSizeWithBounds(
		seatCount,
		minSize,
		maxSize,
	)

	if size > seatCount {
		size = seatCount
	}
	return size
}

// RabbitVRFCommitteeForSnapshotV1 derives one deterministic committee
// candidate from a validated CLOSED work-epoch snapshot.
//
// This function deliberately does not assign the committee to a live VRF
// epoch or DKG keyset. That lifecycle is frozen separately.
func RabbitVRFCommitteeForSnapshotV1(
	chainID *big.Int,
	snapshot *WorkEpochSnapshotV1,
	entropy common.Hash,
	minSize uint64,
	maxSize uint64,
) ([]WorkSeatV1, common.Hash, error) {
	if chainID == nil ||
		chainID.Sign() <= 0 ||
		snapshot == nil ||
		entropy == (common.Hash{}) {
		return nil, common.Hash{}, ErrInvalidRabbitVRFCommitteeV1
	}

	if err := snapshot.Validate(chainID); err != nil {
		return nil, common.Hash{}, err
	}

	seed, err := RabbitVRFCommitteeSeedV1(
		chainID,
		snapshot.Epoch,
		snapshot.Root,
		entropy,
	)
	if err != nil {
		return nil, common.Hash{}, err
	}

	size := RabbitVRFCommitteeSizeV1(
		uint64(len(snapshot.Seats)),
		minSize,
		maxSize,
	)
	if size == 0 {
		return []WorkSeatV1{}, seed, nil
	}

	committee, err := DeterministicallySelectWorkSeatsV1(
		snapshot.Seats,
		seed,
		size,
	)
	if err != nil {
		return nil, common.Hash{}, err
	}

	return committee, seed, nil
}
