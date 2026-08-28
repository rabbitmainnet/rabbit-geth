package lqc

import (
	"errors"
	"sync"

	"github.com/ethereum/go-ethereum/common"
)

const (
	// DefaultWorkCommitPoolCapacityV1 is a NON-CONSENSUS relay-memory bound.
	// It holds four full 1024-ticket commit windows while remaining independent
	// of participant/address count.
	DefaultWorkCommitPoolCapacityV1 uint64 = WorkTicketCommitCapacityPerEpochV1 * 4
)

var (
	ErrWorkCommitPoolFullV1          = errors.New("lqc work commit pool v1 full")
	ErrWorkCommitPoolUninitializedV1 = errors.New("lqc work commit pool v1 uninitialized")
	ErrWorkCommitCandidateMismatchV1 = errors.New("lqc work commit candidate v1 mismatch")
)

// WorkCommitPoolStatusV1 is a read-only local relay status.
//
// Capacity is NOT a consensus parameter. Consensus transport remains bounded by
// MaxWorkTicketsPerBlockV1 and WorkTicketCommitCapacityPerEpochV1.
type WorkCommitPoolStatusV1 struct {
	Epoch    uint64
	Count    uint64
	Capacity uint64
}

// WorkCommitPoolPersistenceV1 stores the already-verified local relay pool.
// Implementations are node-local and NON-CONSENSUS. Loaded candidates are
// structurally validated again by WorkCommitPoolV1 before becoming pending.
type WorkCommitPoolPersistenceV1 interface {
	LoadWorkCommitPoolV1(
		epoch uint64,
	) ([]WorkCommitCandidateV1, error)
	StoreWorkCommitPoolV1(
		epoch uint64,
		candidates []WorkCommitCandidateV1,
	) error
}

// WorkCommitPoolV1 stores only the current commit epoch's ALREADY-VERIFIED
// candidates.
//
// It has:
//   - one candidate per participant and commit epoch,
//   - no sequence chain,
//   - no identity-based priority.
//
// Pending selection delegates to SelectWorkCommitBatchV1, so the same fixed
// proof hashes have the same admission order regardless of wallet splitting.
//
// IMPORTANT: this remains node-local relay state, with optional persistence.
// It does not make gossip visibility globally provable and therefore does not
// by itself solve cartel censorship.
type WorkCommitPoolV1 struct {
	mu sync.RWMutex

	epoch    uint64
	capacity uint64

	byProof  map[common.Hash]WorkCommitCandidateV1
	semantic map[workTicketSemanticKeyV3]common.Hash

	persistence WorkCommitPoolPersistenceV1
}

func NewWorkCommitPoolV1(
	capacity uint64,
) *WorkCommitPoolV1 {
	return NewPersistentWorkCommitPoolV1(capacity, nil)
}

func NewPersistentWorkCommitPoolV1(
	capacity uint64,
	persistence WorkCommitPoolPersistenceV1,
) *WorkCommitPoolV1 {
	if capacity == 0 {
		capacity = DefaultWorkCommitPoolCapacityV1
	}
	return &WorkCommitPoolV1{
		capacity:    capacity,
		byProof:     make(map[common.Hash]WorkCommitCandidateV1),
		semantic:    make(map[workTicketSemanticKeyV3]common.Hash),
		persistence: persistence,
	}
}

func buildWorkCommitPoolStateV1(
	epoch uint64,
	capacity uint64,
	candidates []WorkCommitCandidateV1,
) (
	map[common.Hash]WorkCommitCandidateV1,
	map[workTicketSemanticKeyV3]common.Hash,
	error,
) {
	if epoch == 0 {
		return nil, nil, ErrInvalidWorkCommitV1
	}
	if uint64(len(candidates)) > capacity {
		return nil, nil, ErrWorkCommitPoolFullV1
	}

	byProof := make(
		map[common.Hash]WorkCommitCandidateV1,
		len(candidates),
	)
	semantic := make(
		map[workTicketSemanticKeyV3]common.Hash,
		len(candidates),
	)
	seenParticipants := make(map[common.Address]struct{}, len(candidates))
	for _, candidate := range candidates {
		if candidate.ProofHash == (common.Hash{}) {
			return nil, nil, ErrInvalidWorkCommitV1
		}
		if err := validateWorkTicketHeaderShapeV3(
			candidate.Signed,
		); err != nil {
			return nil, nil, err
		}
		if candidate.Signed.Ticket.Epoch != epoch {
			return nil, nil, ErrWorkCommitEpochMismatchV1
		}
		if _, exists := byProof[candidate.ProofHash]; exists {
			return nil, nil, ErrDuplicateRandomXWorkHash
		}

		key := workTicketSemanticKeyV3{
			Epoch:       candidate.Signed.Ticket.Epoch,
			Participant: candidate.Signed.Ticket.Participant,
			Nonce:       candidate.Signed.Ticket.Nonce,
		}
		if _, exists := semantic[key]; exists {
			return nil, nil, ErrDuplicateWorkTicketV3
		}
		if _, exists := seenParticipants[candidate.Signed.Ticket.Participant]; exists {
			return nil, nil, ErrDuplicateWorkParticipantV1
		}
		seenParticipants[candidate.Signed.Ticket.Participant] = struct{}{}

		cloned := cloneWorkCommitCandidateV1(candidate)
		byProof[cloned.ProofHash] = cloned
		semantic[key] = cloned.ProofHash
	}
	return byProof, semantic, nil
}

func (p *WorkCommitPoolV1) persistLockedV1() error {
	if p.persistence == nil {
		return nil
	}

	all := make([]WorkCommitCandidateV1, 0, len(p.byProof))
	for _, candidate := range p.byProof {
		all = append(all, cloneWorkCommitCandidateV1(candidate))
	}
	canonical, err := CanonicalWorkCommitCandidatesV1(all, p.epoch)
	if err != nil {
		return err
	}
	return p.persistence.StoreWorkCommitPoolV1(
		p.epoch,
		canonical,
	)
}

// NewVerifiedWorkCommitCandidateV1 joins the signed wire ticket to the exact
// VerifiedRandomXWorkTicketV1 returned by the work verifier.
//
// This does not recompute RandomX. Future runtime wiring must only call it after
// RandomXWorkInputV1 was hashed by the canonical RandomX engine and
// ValidateRecomputedRandomXWorkV1 succeeded.
func NewVerifiedWorkCommitCandidateV1(
	signed SignedRandomXWorkTicketV1,
	verified VerifiedRandomXWorkTicketV1,
) (WorkCommitCandidateV1, error) {
	if signed.Ticket != verified.Ticket ||
		verified.Hash == (common.Hash{}) {
		return WorkCommitCandidateV1{}, ErrWorkCommitCandidateMismatchV1
	}
	if err := validateWorkTicketHeaderShapeV3(signed); err != nil {
		return WorkCommitCandidateV1{}, err
	}
	return WorkCommitCandidateV1{
		Signed:    cloneSignedRandomXWorkTicketV1(signed),
		ProofHash: verified.Hash,
	}, nil
}

// ResetCommitEpochV1 switches the local relay pool to the canonical commit
// epoch and clears prior local relay state.
//
// It intentionally permits moving backward after a reorg: the caller supplies
// the canonical epoch derived from the new head.
func (p *WorkCommitPoolV1) ResetCommitEpochV1(
	epoch uint64,
) error {
	if p == nil || epoch == 0 {
		return ErrInvalidWorkCommitV1
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.epoch == epoch {
		return nil
	}

	var restored []WorkCommitCandidateV1
	if p.persistence != nil {
		var err error
		restored, err = p.persistence.LoadWorkCommitPoolV1(epoch)
		if err != nil {
			return err
		}
	}
	byProof, semantic, err := buildWorkCommitPoolStateV1(
		epoch,
		p.capacity,
		restored,
	)
	if err != nil {
		return err
	}

	p.epoch = epoch
	p.byProof = byProof
	p.semantic = semantic
	return nil
}

// AddVerifiedV1 accepts one already-verified candidate into local relay memory.
//
// Duplicate proof hashes and duplicate epoch+participant+nonce identities are
// rejected independently of wallet count.
func (p *WorkCommitPoolV1) AddVerifiedV1(
	candidate WorkCommitCandidateV1,
) error {
	if p == nil {
		return ErrWorkCommitPoolUninitializedV1
	}
	if candidate.ProofHash == (common.Hash{}) {
		return ErrInvalidWorkCommitV1
	}
	if err := validateWorkTicketHeaderShapeV3(
		candidate.Signed,
	); err != nil {
		return err
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.epoch == 0 {
		return ErrWorkCommitPoolUninitializedV1
	}
	if candidate.Signed.Ticket.Epoch != p.epoch {
		return ErrWorkCommitEpochMismatchV1
	}

	if _, exists := p.byProof[candidate.ProofHash]; exists {
		return ErrDuplicateRandomXWorkHash
	}

	key := workTicketSemanticKeyV3{
		Epoch:       candidate.Signed.Ticket.Epoch,
		Participant: candidate.Signed.Ticket.Participant,
		Nonce:       candidate.Signed.Ticket.Nonce,
	}
	if _, exists := p.semantic[key]; exists {
		return ErrDuplicateWorkTicketV3
	}
	for _, existing := range p.byProof {
		if existing.Signed.Ticket.Participant == candidate.Signed.Ticket.Participant {
			return ErrDuplicateWorkParticipantV1
		}
	}

	if uint64(len(p.byProof)) >= p.capacity {
		return ErrWorkCommitPoolFullV1
	}

	cloned := cloneWorkCommitCandidateV1(candidate)
	p.byProof[cloned.ProofHash] = cloned
	p.semantic[key] = cloned.ProofHash
	if err := p.persistLockedV1(); err != nil {
		delete(p.byProof, cloned.ProofHash)
		delete(p.semantic, key)
		return err
	}
	return nil
}

// PendingV1 returns at most MaxWorkTicketsPerBlockV1 candidates in the exact
// canonical local admission order.
func (p *WorkCommitPoolV1) PendingV1() ([]WorkCommitCandidateV1, error) {
	if p == nil {
		return nil, ErrWorkCommitPoolUninitializedV1
	}

	p.mu.RLock()
	if p.epoch == 0 {
		p.mu.RUnlock()
		return nil, ErrWorkCommitPoolUninitializedV1
	}

	epoch := p.epoch
	all := make([]WorkCommitCandidateV1, 0, len(p.byProof))
	for _, candidate := range p.byProof {
		all = append(all, cloneWorkCommitCandidateV1(candidate))
	}
	p.mu.RUnlock()

	return SelectWorkCommitBatchV1(all, epoch)
}

// AllCanonicalV1 returns the complete local pool in canonical commit priority,
// useful for relay/debugging but not directly inserted wholesale into a block.
func (p *WorkCommitPoolV1) AllCanonicalV1() ([]WorkCommitCandidateV1, error) {
	if p == nil {
		return nil, ErrWorkCommitPoolUninitializedV1
	}

	p.mu.RLock()
	if p.epoch == 0 {
		p.mu.RUnlock()
		return nil, ErrWorkCommitPoolUninitializedV1
	}

	epoch := p.epoch
	all := make([]WorkCommitCandidateV1, 0, len(p.byProof))
	for _, candidate := range p.byProof {
		all = append(all, cloneWorkCommitCandidateV1(candidate))
	}
	p.mu.RUnlock()

	return CanonicalWorkCommitCandidatesV1(all, epoch)
}

// RemoveIncludedV1 removes proof hashes observed in canonical committed blocks.
// Missing hashes are harmless and ignored.
func (p *WorkCommitPoolV1) RemoveIncludedV1(
	proofHashes []common.Hash,
) uint64 {
	if p == nil {
		return 0
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	removedCandidates := make(
		[]WorkCommitCandidateV1,
		0,
		len(proofHashes),
	)
	for _, proofHash := range proofHashes {
		candidate, exists := p.byProof[proofHash]
		if !exists {
			continue
		}

		key := workTicketSemanticKeyV3{
			Epoch:       candidate.Signed.Ticket.Epoch,
			Participant: candidate.Signed.Ticket.Participant,
			Nonce:       candidate.Signed.Ticket.Nonce,
		}
		delete(p.byProof, proofHash)
		delete(p.semantic, key)
		removedCandidates = append(
			removedCandidates,
			cloneWorkCommitCandidateV1(candidate),
		)
	}
	if len(removedCandidates) == 0 {
		return 0
	}
	if err := p.persistLockedV1(); err != nil {
		for _, candidate := range removedCandidates {
			key := workTicketSemanticKeyV3{
				Epoch:       candidate.Signed.Ticket.Epoch,
				Participant: candidate.Signed.Ticket.Participant,
				Nonce:       candidate.Signed.Ticket.Nonce,
			}
			p.byProof[candidate.ProofHash] = candidate
			p.semantic[key] = candidate.ProofHash
		}
		return 0
	}
	return uint64(len(removedCandidates))
}

func (p *WorkCommitPoolV1) StatusV1() WorkCommitPoolStatusV1 {
	if p == nil {
		return WorkCommitPoolStatusV1{}
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

	return WorkCommitPoolStatusV1{
		Epoch:    p.epoch,
		Count:    uint64(len(p.byProof)),
		Capacity: p.capacity,
	}
}

func (p *WorkCommitPoolV1) LenV1() uint64 {
	return p.StatusV1().Count
}
