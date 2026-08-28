//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"bytes"
	"errors"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
)

var errWorkV1EnginePoolProviderLab = errors.New(
	"invalid Work V1 lqcw -> engine provider laboratory state",
)

type workV1EnginePoolProviderLab struct {
	mu sync.Mutex

	pending    func() ([]lqc.WorkCommitCandidateV1, error)
	includedAt func(
		blockNumber uint64,
	) ([]lqc.SignedRandomXWorkTicketV1, bool)
	removeIncluded func([]common.Hash) uint64
	readmitRemoved func(lqc.WorkCommitCandidateV1) bool
	epochLength    uint64

	reservedBlock  uint64
	reservedEpoch  uint64
	reserved       []lqc.WorkCommitCandidateV1
	removed        []workV1EnginePoolRemovalLab
	scannedEpoch   uint64
	scannedThrough uint64
}

type workV1EnginePoolRemovalLab struct {
	block      uint64
	epoch      uint64
	candidates []lqc.WorkCommitCandidateV1
}

func cloneWorkV1EnginePoolCandidateLab(
	in lqc.WorkCommitCandidateV1,
) lqc.WorkCommitCandidateV1 {
	out := in
	out.Signed.Signature = append(
		[]byte(nil),
		in.Signed.Signature...,
	)
	return out
}

func cloneWorkV1EnginePoolCandidatesLab(
	in []lqc.WorkCommitCandidateV1,
) []lqc.WorkCommitCandidateV1 {
	out := make([]lqc.WorkCommitCandidateV1, 0, len(in))
	for _, candidate := range in {
		out = append(
			out,
			cloneWorkV1EnginePoolCandidateLab(candidate),
		)
	}
	return out
}

func workV1EnginePoolSignedEqualLab(
	left,
	right lqc.SignedRandomXWorkTicketV1,
) bool {
	return left.Ticket.Version == right.Ticket.Version &&
		left.Ticket.Epoch == right.Ticket.Epoch &&
		left.Ticket.Participant == right.Ticket.Participant &&
		left.Ticket.Nonce == right.Ticket.Nonce &&
		bytes.Equal(left.Signature, right.Signature)
}

func workV1EnginePoolSignedTicketsLab(
	candidates []lqc.WorkCommitCandidateV1,
) []lqc.SignedRandomXWorkTicketV1 {
	out := make(
		[]lqc.SignedRandomXWorkTicketV1,
		0,
		len(candidates),
	)
	for _, candidate := range candidates {
		signed := candidate.Signed
		signed.Signature = append(
			[]byte(nil),
			candidate.Signed.Signature...,
		)
		out = append(out, signed)
	}
	return out
}

func (p *workV1EnginePoolProviderLab) clearReservationLocked() {
	p.reservedBlock = 0
	p.reservedEpoch = 0
	p.reserved = nil
}

func (p *workV1EnginePoolProviderLab) retainRemovedLocked(
	blockNumber uint64,
	commitEpoch uint64,
	candidates []lqc.WorkCommitCandidateV1,
) {
	if len(candidates) == 0 {
		return
	}
	p.removed = append(p.removed, workV1EnginePoolRemovalLab{
		block: blockNumber,
		epoch: commitEpoch,
		candidates: cloneWorkV1EnginePoolCandidatesLab(
			candidates,
		),
	})
}

// reconcileRemovedLocked keeps canonically included candidates quarantined
// while their exact signed tickets remain in the canonical Header V3. If a
// reorg replaces that block without the ticket, the already-verified candidate
// is readmitted to the lqcw pool.
//
// Candidates from a completed commit epoch are discarded. The transport pool
// itself accepts only the active commit epoch and resets at the epoch boundary,
// so reinjecting an older ticket into the new window would be invalid.
func (p *workV1EnginePoolProviderLab) reconcileRemovedLocked(
	commitEpoch uint64,
) {
	if p == nil || len(p.removed) == 0 {
		return
	}

	retained := make(
		[]workV1EnginePoolRemovalLab,
		0,
		len(p.removed),
	)
	for _, removal := range p.removed {
		if removal.epoch != commitEpoch {
			continue
		}

		included, canonical := p.includedAt(removal.block)
		remaining := make(
			[]lqc.WorkCommitCandidateV1,
			0,
			len(removal.candidates),
		)
		for _, candidate := range removal.candidates {
			stillIncluded := false
			if canonical {
				for _, signed := range included {
					if workV1EnginePoolSignedEqualLab(
						candidate.Signed,
						signed,
					) {
						stillIncluded = true
						break
					}
				}
			}

			if stillIncluded ||
				p.readmitRemoved == nil ||
				!p.readmitRemoved(candidate) {
				remaining = append(
					remaining,
					cloneWorkV1EnginePoolCandidateLab(
						candidate,
					),
				)
			}
		}

		if len(remaining) > 0 {
			retained = append(
				retained,
				workV1EnginePoolRemovalLab{
					block:      removal.block,
					epoch:      removal.epoch,
					candidates: remaining,
				},
			)
		}
	}
	p.removed = retained
}

// reconcileReservationLocked removes a proof from the local lqcw pool only
// after the exact signed ticket is observed in the canonical Header V3 at the
// reserved block.
//
// If the reserved block never became canonical (abandoned payload/reorg), no
// proof is removed. The candidate remains available for another block.
func (p *workV1EnginePoolProviderLab) reconcileReservationLocked() {
	if p == nil || p.reservedBlock == 0 {
		return
	}
	if p.includedAt == nil || p.removeIncluded == nil {
		p.clearReservationLocked()
		return
	}

	included, canonical := p.includedAt(p.reservedBlock)
	if !canonical {
		p.clearReservationLocked()
		return
	}

	removed := make(
		[]lqc.WorkCommitCandidateV1,
		0,
		len(p.reserved),
	)
	for _, candidate := range p.reserved {
		for _, signed := range included {
			if workV1EnginePoolSignedEqualLab(
				candidate.Signed,
				signed,
			) {
				if p.removeIncluded(
					[]common.Hash{candidate.ProofHash},
				) > 0 {
					removed = append(
						removed,
						candidate,
					)
				}
				break
			}
		}
	}
	p.retainRemovedLocked(
		p.reservedBlock,
		p.reservedEpoch,
		removed,
	)
	p.clearReservationLocked()
}

// reconcileCanonicalPendingLocked reconstructs the reservation reconciliation
// that may have been lost when the process stopped immediately after a block
// containing Work V1 tickets became canonical. The scan is bounded to the
// active commit window (one configured epoch), never to the whole chain.
func (p *workV1EnginePoolProviderLab) reconcileCanonicalPendingLocked(
	blockNumber uint64,
	commitEpoch uint64,
) error {
	if p == nil || p.pending == nil || p.includedAt == nil ||
		p.removeIncluded == nil || p.epochLength == 0 ||
		blockNumber == 0 || commitEpoch == 0 {
		return nil
	}
	if commitEpoch > (^uint64(0)-1)/p.epochLength {
		return errWorkV1EnginePoolProviderLab
	}
	windowStart := commitEpoch*p.epochLength + 1
	if blockNumber <= windowStart {
		return nil
	}
	if p.scannedEpoch != commitEpoch ||
		p.scannedThrough < windowStart-1 ||
		p.scannedThrough >= blockNumber {
		p.scannedEpoch = commitEpoch
		p.scannedThrough = windowStart - 1
	}

	pending, err := p.pending()
	if err != nil {
		return err
	}
	remaining := make(map[common.Hash]lqc.WorkCommitCandidateV1, len(pending))
	for _, candidate := range pending {
		if candidate.Signed.Ticket.Epoch == commitEpoch {
			remaining[candidate.ProofHash] = candidate
		}
	}
	for number := p.scannedThrough + 1; number < blockNumber; number++ {
		included, canonical := p.includedAt(number)
		if !canonical || len(included) == 0 || len(remaining) == 0 {
			p.scannedThrough = number
			continue
		}
		removed := make([]lqc.WorkCommitCandidateV1, 0)
		for proofHash, candidate := range remaining {
			for _, signed := range included {
				if !workV1EnginePoolSignedEqualLab(candidate.Signed, signed) {
					continue
				}
				if p.removeIncluded([]common.Hash{proofHash}) > 0 {
					removed = append(removed, candidate)
				}
				delete(remaining, proofHash)
				break
			}
		}
		p.retainRemovedLocked(number, commitEpoch, removed)
		p.scannedThrough = number
	}
	return nil
}

func (p *workV1EnginePoolProviderLab) reconcileCanonical(
	blockNumber uint64,
	commitEpoch uint64,
) error {
	if p == nil || blockNumber == 0 || commitEpoch == 0 {
		return errWorkV1EnginePoolProviderLab
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	p.reconcileRemovedLocked(commitEpoch)
	return p.reconcileCanonicalPendingLocked(blockNumber, commitEpoch)
}

func (p *workV1EnginePoolProviderLab) canonicalIncludes(
	blockNumber uint64,
	commitEpoch uint64,
	candidate lqc.WorkCommitCandidateV1,
) bool {
	if p == nil || p.includedAt == nil || p.epochLength == 0 ||
		blockNumber == 0 || commitEpoch == 0 ||
		candidate.Signed.Ticket.Epoch != commitEpoch ||
		commitEpoch > (^uint64(0)-1)/p.epochLength {
		return false
	}

	windowStart := commitEpoch*p.epochLength + 1
	if blockNumber <= windowStart {
		return false
	}
	for number := windowStart; number < blockNumber; number++ {
		included, canonical := p.includedAt(number)
		if !canonical {
			continue
		}
		for _, signed := range included {
			if signed.Ticket.Epoch == commitEpoch &&
				signed.Ticket.Participant == candidate.Signed.Ticket.Participant {
				return true
			}
		}
	}
	return false
}

// provide is the engine-facing callback.
//
// Repeated Prepare calls for the same block receive the same reservation.
// Advancing to another block first reconciles the previous reservation against
// the canonical Header V3, then obtains the next canonical Pending() batch.
func (p *workV1EnginePoolProviderLab) provide(
	blockNumber uint64,
	commitEpoch uint64,
) ([]lqc.SignedRandomXWorkTicketV1, error) {
	if p == nil ||
		p.pending == nil ||
		blockNumber == 0 ||
		commitEpoch == 0 {
		return nil, errWorkV1EnginePoolProviderLab
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	p.reconcileRemovedLocked(commitEpoch)
	if err := p.reconcileCanonicalPendingLocked(
		blockNumber,
		commitEpoch,
	); err != nil {
		return nil, err
	}

	if p.reservedBlock == blockNumber &&
		p.reservedEpoch == commitEpoch {
		return workV1EnginePoolSignedTicketsLab(
			p.reserved,
		), nil
	}

	if p.reservedBlock != 0 {
		// A lower/equal-but-different request can happen around a reorg or an
		// abandoned payload. Do not remove anything without canonical proof.
		if blockNumber > p.reservedBlock {
			p.reconcileReservationLocked()
		} else {
			p.clearReservationLocked()
		}
	}

	pending, err := p.pending()
	if err != nil {
		return nil, err
	}

	usable := make(
		[]lqc.WorkCommitCandidateV1,
		0,
		lqc.MaxWorkTicketsPerBlockV1,
	)
	for _, candidate := range pending {
		if candidate.Signed.Ticket.Epoch != commitEpoch {
			continue
		}
		usable = append(
			usable,
			cloneWorkV1EnginePoolCandidateLab(candidate),
		)
		if len(usable) >= int(lqc.MaxWorkTicketsPerBlockV1) {
			break
		}
	}

	p.reservedBlock = blockNumber
	p.reservedEpoch = commitEpoch
	p.reserved = cloneWorkV1EnginePoolCandidatesLab(usable)

	return workV1EnginePoolSignedTicketsLab(
		p.reserved,
	), nil
}

func wireWorkV1EngineTicketProviderMaybeLab(
	backend *Ethereum,
	transport *lqcWorkV1Transport,
) error {
	if backend == nil ||
		backend.blockchain == nil ||
		transport == nil ||
		transport.pool == nil {
		return errWorkV1EnginePoolProviderLab
	}
	engine, ok := backend.engine.(*lqc.LQC)
	if !ok || engine == nil {
		return errWorkV1EnginePoolProviderLab
	}

	provider := &workV1EnginePoolProviderLab{
		pending: transport.pendingRaw,
		includedAt: func(
			blockNumber uint64,
		) ([]lqc.SignedRandomXWorkTicketV1, bool) {
			header := backend.blockchain.GetHeaderByNumber(
				blockNumber,
			)
			if header == nil {
				return nil, false
			}
			envelope, err := lqc.DecodeLQCHeaderExtraV3(
				header.Extra,
				lqc.MaxWorkTicketsPerBlockV1,
			)
			if err != nil {
				return nil, false
			}
			out := append(
				[]lqc.SignedRandomXWorkTicketV1(nil),
				envelope.WorkTickets...,
			)
			for i := range out {
				out[i].Signature = append(
					[]byte(nil),
					out[i].Signature...,
				)
			}
			return out, true
		},
		removeIncluded: transport.pool.RemoveIncludedV1,
		readmitRemoved: func(
			candidate lqc.WorkCommitCandidateV1,
		) bool {
			ctx, err := transport.currentContextRaw()
			if err != nil ||
				ctx.Epoch != candidate.Signed.Ticket.Epoch {
				return false
			}

			err = transport.pool.AddVerifiedV1(candidate)
			if err == nil {
				transport.BroadcastCandidates(
					[]lqc.WorkCommitCandidateV1{candidate},
					"",
				)
				return true
			}
			return errors.Is(
				err,
				lqc.ErrDuplicateRandomXWorkHash,
			) ||
				errors.Is(err, lqc.ErrDuplicateWorkTicketV3) ||
				transport.pool.WorkCommitPoolContainsCandidateV1(
					candidate,
				)
		},
		epochLength: backend.blockchain.Config().LQC.EpochLength,
	}
	transport.reconcile = func(commitEpoch uint64) error {
		head := backend.blockchain.CurrentHeader()
		if head == nil || head.Number == nil || head.Number.Sign() < 0 ||
			head.Number.Uint64() == ^uint64(0) {
			return errWorkV1EnginePoolProviderLab
		}
		return provider.reconcileCanonical(
			head.Number.Uint64()+1,
			commitEpoch,
		)
	}
	transport.included = func(
		candidate lqc.WorkCommitCandidateV1,
		commitEpoch uint64,
	) bool {
		head := backend.blockchain.CurrentHeader()
		if head == nil || head.Number == nil || head.Number.Sign() < 0 ||
			head.Number.Uint64() == ^uint64(0) {
			return false
		}
		return provider.canonicalIncludes(
			head.Number.Uint64()+1,
			commitEpoch,
			candidate,
		)
	}

	return lqc.SetWorkV1EngineLabTicketProvider(
		engine,
		provider.provide,
	)
}
