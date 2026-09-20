package lqc

import (
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
)

var ErrInvalidRabbitVRFCommitteeCommitmentV1 = errors.New(
	"invalid rabbit vrf committee commitment v1",
)

var rabbitVRFCommitteeRootDomainV1 = []byte(
	"RABBIT-VRF-COMMITTEE-ROOT-V1",
)

// RabbitVRFCommitteeMemberV1 is one immutable member position inside a
// deterministically ordered Rabbit VRF committee.
//
// ShareID is consensus-derived from committee position:
//
//	ShareID = position + 1
//
// ShareID zero is therefore impossible and members are never renumbered by
// DKG participation, liveness, complaints or later qualification decisions.
type RabbitVRFCommitteeMemberV1 struct {
	ShareID     uint64
	TicketHash  common.Hash
	Participant common.Address
}

type rabbitVRFCommitteeRootPayloadV1 struct {
	Domain        []byte
	Version       uint8
	ChainID       *big.Int
	SourceEpoch   uint64
	SelectionRoot common.Hash
	CommitteeSeed common.Hash
	Members       []RabbitVRFCommitteeMemberV1
}

// RabbitVRFCommitteeMembersV1 assigns immutable ShareIDs to an already
// canonically ordered VRF committee.
//
// The input order is preserved deliberately. Reordering the committee changes
// ShareIDs and therefore changes the committee commitment.
func RabbitVRFCommitteeMembersV1(
	committee []WorkSeatV1,
) ([]RabbitVRFCommitteeMemberV1, error) {
	if len(committee) == 0 {
		return []RabbitVRFCommitteeMemberV1{}, nil
	}

	seenTickets := make(
		map[common.Hash]struct{},
		len(committee),
	)
	seenParticipants := make(
		map[common.Address]struct{},
		len(committee),
	)

	members := make(
		[]RabbitVRFCommitteeMemberV1,
		len(committee),
	)

	for index, seat := range committee {
		if seat.TicketHash == (common.Hash{}) ||
			seat.Participant == (common.Address{}) {
			return nil, ErrInvalidRabbitVRFCommitteeCommitmentV1
		}

		if _, exists := seenTickets[seat.TicketHash]; exists {
			return nil, ErrInvalidRabbitVRFCommitteeCommitmentV1
		}
		seenTickets[seat.TicketHash] = struct{}{}

		if _, exists := seenParticipants[seat.Participant]; exists {
			return nil, ErrInvalidRabbitVRFCommitteeCommitmentV1
		}
		seenParticipants[seat.Participant] = struct{}{}

		members[index] = RabbitVRFCommitteeMemberV1{
			ShareID:     uint64(index) + 1,
			TicketHash:  seat.TicketHash,
			Participant: seat.Participant,
		}
	}

	return members, nil
}

// RabbitVRFCommitteeRootV1 commits to the exact ordered VRF committee and its
// immutable ShareID assignment.
//
// VRF epoch number, threshold public key, verification shares and DKG transcript
// are intentionally absent. Those belong to the future keyset commitment.
func RabbitVRFCommitteeRootV1(
	chainID *big.Int,
	sourceEpoch uint64,
	selectionRoot common.Hash,
	committeeSeed common.Hash,
	committee []WorkSeatV1,
) (
	common.Hash,
	[]RabbitVRFCommitteeMemberV1,
	error,
) {
	if chainID == nil ||
		chainID.Sign() <= 0 ||
		sourceEpoch == 0 ||
		selectionRoot == (common.Hash{}) ||
		committeeSeed == (common.Hash{}) {
		return common.Hash{},
			nil,
			ErrInvalidRabbitVRFCommitteeCommitmentV1
	}

	members, err := RabbitVRFCommitteeMembersV1(
		committee,
	)
	if err != nil {
		return common.Hash{}, nil, err
	}

	if len(members) == 0 {
		return common.Hash{},
			nil,
			ErrInvalidRabbitVRFCommitteeCommitmentV1
	}

	encoded, err := rlp.EncodeToBytes(
		rabbitVRFCommitteeRootPayloadV1{
			Domain:        rabbitVRFCommitteeRootDomainV1,
			Version:       RabbitVRFCommitteeVersionV1,
			ChainID:       new(big.Int).Set(chainID),
			SourceEpoch:   sourceEpoch,
			SelectionRoot: selectionRoot,
			CommitteeSeed: committeeSeed,
			Members:       members,
		},
	)
	if err != nil {
		return common.Hash{}, nil, err
	}

	root := crypto.Keccak256Hash(encoded)
	if root == (common.Hash{}) {
		return common.Hash{},
			nil,
			ErrInvalidRabbitVRFCommitteeCommitmentV1
	}

	return root, members, nil
}

// RabbitVRFCommitteeCommitmentForSnapshotV1 derives the committee from the
// validated closed Work snapshot and immediately commits to the resulting
// canonical ordering and ShareID assignment.
//
// This prevents callers from supplying their own committee ordering.
func RabbitVRFCommitteeCommitmentForSnapshotV1(
	chainID *big.Int,
	snapshot *WorkEpochSnapshotV1,
	entropy common.Hash,
	minSize uint64,
	maxSize uint64,
) (
	common.Hash,
	common.Hash,
	[]RabbitVRFCommitteeMemberV1,
	error,
) {
	committee, seed, err := RabbitVRFCommitteeForSnapshotV1(
		chainID,
		snapshot,
		entropy,
		minSize,
		maxSize,
	)
	if err != nil {
		return common.Hash{}, common.Hash{}, nil, err
	}

	if len(committee) == 0 ||
		snapshot == nil {
		return common.Hash{},
			common.Hash{},
			nil,
			ErrInvalidRabbitVRFCommitteeCommitmentV1
	}

	root, members, err := RabbitVRFCommitteeRootV1(
		chainID,
		snapshot.Epoch,
		snapshot.Root,
		seed,
		committee,
	)
	if err != nil {
		return common.Hash{}, common.Hash{}, nil, err
	}

	return root, seed, members, nil
}
