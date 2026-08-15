package lqc

import (
	"errors"
	"math/big"
	"sort"
	"sync"

	"github.com/ethereum/go-ethereum/common"
)

const (
	MaxWorkTicketPoolEntries = 4096
	MaxWorkTicketsPerLane    = 64
)

var (
	ErrWorkTicketPoolDisabled  = errors.New("lqc work ticket pool disabled")
	ErrWorkTicketPoolFull      = errors.New("lqc work ticket pool full")
	ErrWorkTicketKnown         = errors.New("lqc work ticket already known")
	ErrWorkTicketSequenceKnown = errors.New("lqc work ticket sequence already pending")
	ErrWorkTicketLaneFull      = errors.New("lqc work ticket participant lane is full")
)

// WorkTicketPool is relay-only memory. Canonical snapshots, not this pool,
// decide whether a ticket can be committed.
type WorkTicketPool struct {
	mu            sync.RWMutex
	byHash        map[common.Hash]WorkTicket
	byParticipant map[common.Address]map[uint64]common.Hash
}

type WorkTicketPoolStatus struct {
	Pending      int `json:"pending"`
	Participants int `json:"participants"`
	Capacity     int `json:"capacity"`
	PerLane      int `json:"perLane"`
}

func NewWorkTicketPool() *WorkTicketPool {
	return &WorkTicketPool{
		byHash:        make(map[common.Hash]WorkTicket),
		byParticipant: make(map[common.Address]map[uint64]common.Hash),
	}
}

func WorkTicketHash(chainID *big.Int, ticket WorkTicket) common.Hash {
	return WorkTicketSigningHash(chainID, ticket)
}

func cloneWorkTicket(ticket WorkTicket) WorkTicket {
	clone := ticket
	clone.Signature = append([]byte(nil), ticket.Signature...)
	return clone
}

// Add performs full cryptographic verification before retaining a ticket.
func (p *WorkTicketPool) Add(chainID *big.Int, ticket WorkTicket) (common.Hash, error) {
	if p == nil {
		return common.Hash{}, ErrWorkTicketPoolDisabled
	}
	if err := ValidateWorkTicketCryptography(chainID, ticket); err != nil {
		return common.Hash{}, err
	}
	hash := WorkTicketHash(chainID, ticket)
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, exists := p.byHash[hash]; exists {
		return hash, ErrWorkTicketKnown
	}
	if len(p.byHash) >= MaxWorkTicketPoolEntries {
		return common.Hash{}, ErrWorkTicketPoolFull
	}
	lane := p.byParticipant[ticket.Participant]
	if lane == nil {
		lane = make(map[uint64]common.Hash)
		p.byParticipant[ticket.Participant] = lane
	}
	if _, exists := lane[ticket.Sequence]; exists {
		return common.Hash{}, ErrWorkTicketSequenceKnown
	}
	if len(lane) >= MaxWorkTicketsPerLane {
		return common.Hash{}, ErrWorkTicketLaneFull
	}
	p.byHash[hash] = cloneWorkTicket(ticket)
	lane[ticket.Sequence] = hash
	return hash, nil
}

// Pending returns continuous tickets in participant rounds, then canonicalizes
// their wire order. One deep lane cannot consume the batch before other lanes.
func (p *WorkTicketPool) Pending(states map[common.Address]WorkTicketLaneState, limit int) []WorkTicket {
	if p == nil || limit <= 0 {
		return nil
	}
	if limit > MaxWorkTicketsPerBlock {
		limit = MaxWorkTicketsPerBlock
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	addresses := make([]common.Address, 0, len(states))
	working := cloneWorkTicketLaneStates(states)
	for address := range states {
		addresses = append(addresses, address)
	}
	sort.Slice(addresses, func(i, j int) bool { return addresses[i].Cmp(addresses[j]) < 0 })
	selected := make([]WorkTicket, 0, limit)
	for len(selected) < limit {
		advanced := false
		for _, address := range addresses {
			if len(selected) >= limit {
				break
			}
			state := working[address]
			lane := p.byParticipant[address]
			hash, exists := lane[state.NextSequence]
			if !exists {
				continue
			}
			ticket := p.byHash[hash]
			if ticket.Epoch != state.Epoch || ticket.Previous != state.Previous {
				continue
			}
			selected = append(selected, cloneWorkTicket(ticket))
			state.NextSequence++
			state.Previous = ticket.Proof
			working[address] = state
			advanced = true
		}
		if !advanced {
			break
		}
	}
	return CanonicalWorkTickets(selected)
}

func (p *WorkTicketPool) RemoveIncluded(tickets []WorkTicket) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, ticket := range tickets {
		lane := p.byParticipant[ticket.Participant]
		hash, exists := lane[ticket.Sequence]
		if !exists {
			continue
		}
		delete(lane, ticket.Sequence)
		delete(p.byHash, hash)
		if len(lane) == 0 {
			delete(p.byParticipant, ticket.Participant)
		}
	}
}

// Prune removes tickets that cannot extend the supplied canonical lanes.
func (p *WorkTicketPool) Prune(anchor common.Hash, epoch uint64, states map[common.Address]WorkTicketLaneState) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for hash, ticket := range p.byHash {
		state, known := states[ticket.Participant]
		if !known || ticket.Anchor != anchor || ticket.Epoch != epoch || state.Epoch != epoch || ticket.Sequence < state.NextSequence {
			delete(p.byHash, hash)
			lane := p.byParticipant[ticket.Participant]
			delete(lane, ticket.Sequence)
			if len(lane) == 0 {
				delete(p.byParticipant, ticket.Participant)
			}
		}
	}
}

func (p *WorkTicketPool) Status() WorkTicketPoolStatus {
	status := WorkTicketPoolStatus{Capacity: MaxWorkTicketPoolEntries, PerLane: MaxWorkTicketsPerLane}
	if p == nil {
		return status
	}
	p.mu.RLock()
	status.Pending = len(p.byHash)
	status.Participants = len(p.byParticipant)
	p.mu.RUnlock()
	return status
}

func (p *WorkTicketPool) Has(hash common.Hash) bool {
	if p == nil {
		return false
	}
	p.mu.RLock()
	_, exists := p.byHash[hash]
	p.mu.RUnlock()
	return exists
}

// All returns a canonical, deeply copied view of the relay pool. It is used by
// bounded initial P2P synchronization only; canonical snapshots, not this
// arrival-order-independent view, remain authoritative for consensus.
func (p *WorkTicketPool) All(limit int) []WorkTicket {
	if p == nil || limit <= 0 {
		return nil
	}
	if limit > MaxWorkTicketPoolEntries {
		limit = MaxWorkTicketPoolEntries
	}
	p.mu.RLock()
	tickets := make([]WorkTicket, 0, len(p.byHash))
	for _, ticket := range p.byHash {
		tickets = append(tickets, cloneWorkTicket(ticket))
	}
	p.mu.RUnlock()
	tickets = CanonicalWorkTickets(tickets)
	if len(tickets) > limit {
		tickets = tickets[:limit]
	}
	return tickets
}
