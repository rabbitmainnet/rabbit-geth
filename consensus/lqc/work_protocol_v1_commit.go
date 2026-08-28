package lqc

import (
	"bytes"
	"encoding/binary"
	"errors"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

var (
	ErrInvalidWorkCommitV1        = errors.New("invalid lqc work commit v1")
	ErrWorkCommitWindowOverflowV1 = errors.New("lqc work commit window overflow v1")
)

var workCommitPriorityDomainV1 = []byte("RABBIT-LQC-WORK-COMMIT-PRIORITY-V1")

// WorkCommitCandidateV1 is an INACTIVE local representation of a ticket whose
// RandomX proof hash has already been recomputed by the caller.
//
// Full proof/target/ownership validation remains in
// ValidateRecomputedRandomXWorkV1. This type exists only to define deterministic
// admission priority among already verified candidates.
type WorkCommitCandidateV1 struct {
	Signed    SignedRandomXWorkTicketV1
	ProofHash common.Hash
}

type scoredWorkCommitCandidateV1 struct {
	Candidate WorkCommitCandidateV1
	Priority  common.Hash
}

func cloneWorkCommitCandidateV1(
	input WorkCommitCandidateV1,
) WorkCommitCandidateV1 {
	return WorkCommitCandidateV1{
		Signed:    cloneSignedRandomXWorkTicketV1(input.Signed),
		ProofHash: input.ProofHash,
	}
}

// WorkCommitPriorityV1 is independent of participant identity.
//
// Splitting one controller's fixed proof set across 1, 100 or 5000 addresses
// therefore cannot improve the admission priority of those same proof hashes.
func WorkCommitPriorityV1(
	epoch uint64,
	proofHash common.Hash,
) (common.Hash, error) {
	if epoch == 0 || proofHash == (common.Hash{}) {
		return common.Hash{}, ErrInvalidWorkCommitV1
	}

	var encodedEpoch [8]byte
	binary.BigEndian.PutUint64(encodedEpoch[:], epoch)

	return crypto.Keccak256Hash(
		workCommitPriorityDomainV1,
		encodedEpoch[:],
		proofHash[:],
	), nil
}

// CanonicalWorkCommitCandidatesV1 validates cheap structural properties and
// returns every candidate in deterministic admission-priority order.
//
// IMPORTANT: this does NOT solve global ticket availability. A validator cannot
// prove which valid tickets a producer saw over gossip. This ordering removes
// arbitrary LOCAL cherry-picking among a producer's known verified candidates;
// censorship resistance still relies on the multi-block commit window,
// propagation, independent producers and the 4x transport headroom.
func CanonicalWorkCommitCandidatesV1(
	input []WorkCommitCandidateV1,
	commitEpoch uint64,
) ([]WorkCommitCandidateV1, error) {
	if commitEpoch == 0 {
		return nil, ErrInvalidWorkCommitV1
	}

	scored := make([]scoredWorkCommitCandidateV1, len(input))
	seenProofs := make(map[common.Hash]struct{}, len(input))
	seenSemantic := make(map[workTicketSemanticKeyV3]struct{}, len(input))
	seenParticipants := make(map[common.Address]struct{}, len(input))

	for index, candidate := range input {
		if candidate.ProofHash == (common.Hash{}) {
			return nil, ErrInvalidWorkCommitV1
		}
		if err := validateWorkTicketHeaderShapeV3(
			candidate.Signed,
		); err != nil {
			return nil, err
		}
		if candidate.Signed.Ticket.Epoch != commitEpoch {
			return nil, ErrWorkCommitEpochMismatchV1
		}

		if _, exists := seenProofs[candidate.ProofHash]; exists {
			return nil, ErrDuplicateRandomXWorkHash
		}
		seenProofs[candidate.ProofHash] = struct{}{}

		key := workTicketSemanticKeyV3{
			Epoch:       candidate.Signed.Ticket.Epoch,
			Participant: candidate.Signed.Ticket.Participant,
			Nonce:       candidate.Signed.Ticket.Nonce,
		}
		if _, exists := seenSemantic[key]; exists {
			return nil, ErrDuplicateWorkTicketV3
		}
		seenSemantic[key] = struct{}{}
		if _, exists := seenParticipants[candidate.Signed.Ticket.Participant]; exists {
			return nil, ErrDuplicateWorkParticipantV1
		}
		seenParticipants[candidate.Signed.Ticket.Participant] = struct{}{}

		priority, err := WorkCommitPriorityV1(
			commitEpoch,
			candidate.ProofHash,
		)
		if err != nil {
			return nil, err
		}

		scored[index] = scoredWorkCommitCandidateV1{
			Candidate: cloneWorkCommitCandidateV1(candidate),
			Priority:  priority,
		}
	}

	sort.Slice(scored, func(i, j int) bool {
		if order := bytes.Compare(
			scored[i].Priority[:],
			scored[j].Priority[:],
		); order != 0 {
			return order < 0
		}
		if order := bytes.Compare(
			scored[i].Candidate.ProofHash[:],
			scored[j].Candidate.ProofHash[:],
		); order != 0 {
			return order < 0
		}

		left := scored[i].Candidate.Signed.Ticket
		right := scored[j].Candidate.Signed.Ticket
		if order := left.Participant.Cmp(right.Participant); order != 0 {
			return order < 0
		}
		return left.Nonce < right.Nonce
	})

	out := make([]WorkCommitCandidateV1, len(scored))
	for index := range scored {
		out[index] = scored[index].Candidate
	}
	return out, nil
}

// SelectWorkCommitBatchV1 chooses at most MaxWorkTicketsPerBlockV1 candidates
// from the producer's verified local candidate set.
//
// It enforces one candidate per participant for the commit epoch.
func SelectWorkCommitBatchV1(
	input []WorkCommitCandidateV1,
	commitEpoch uint64,
) ([]WorkCommitCandidateV1, error) {
	canonical, err := CanonicalWorkCommitCandidatesV1(
		input,
		commitEpoch,
	)
	if err != nil {
		return nil, err
	}

	limit := int(MaxWorkTicketsPerBlockV1)
	if len(canonical) < limit {
		limit = len(canonical)
	}
	return append(
		[]WorkCommitCandidateV1(nil),
		canonical[:limit]...,
	), nil
}

// WorkCommitCapacityForBlocksV1 is the maximum number of tickets that honest
// producers can carry in a given number of blocks of one commit epoch.
func WorkCommitCapacityForBlocksV1(
	blocks uint64,
) uint64 {
	if blocks > WorkProtocolEpochLengthV1 {
		blocks = WorkProtocolEpochLengthV1
	}
	return blocks * MaxWorkTicketsPerBlockV1
}

// MinimumHonestCommitBlocksV1 returns the number of non-censoring producer
// blocks required to carry ticketCount tickets, assuming those tickets have
// propagated to those producers.
//
// This is capacity math, NOT a proof that gossip availability is global.
func MinimumHonestCommitBlocksV1(
	ticketCount uint64,
) (uint64, error) {
	if ticketCount > WorkTicketCommitCapacityPerEpochV1 {
		return 0, ErrWorkCommitWindowOverflowV1
	}
	if ticketCount == 0 {
		return 0, nil
	}

	blocks := ticketCount / MaxWorkTicketsPerBlockV1
	if ticketCount%MaxWorkTicketsPerBlockV1 != 0 {
		blocks++
	}
	return blocks, nil
}

// MinimumHonestCommitFractionBpsV1 returns ceil(honestBlocks / epochLength)
// in basis points.
func MinimumHonestCommitFractionBpsV1(
	ticketCount uint64,
) (uint64, error) {
	blocks, err := MinimumHonestCommitBlocksV1(ticketCount)
	if err != nil {
		return 0, err
	}
	if blocks == 0 {
		return 0, nil
	}

	numerator := blocks*10_000 + WorkProtocolEpochLengthV1 - 1
	return numerator / WorkProtocolEpochLengthV1, nil
}
