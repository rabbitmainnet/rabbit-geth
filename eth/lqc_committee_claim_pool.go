package eth

import (
	"bytes"
	"errors"
	"sort"
	"sync"

	"github.com/ethereum/go-ethereum/consensus/lqc"
)

var errLQCCommitteeClaimPool = errors.New("invalid lqc committee claim pool operation")

type lqcCommitteeClaimKey struct {
	target   uint64
	position uint8
}

// lqcCommitteeClaimPool is intentionally bounded by the consensus claim
// window: at most 128 positions for each of eight target blocks.
type lqcCommitteeClaimPool struct {
	mu     sync.Mutex
	claims map[lqcCommitteeClaimKey]lqc.CompactCommitteeParticipationV1
}

func newLQCCommitteeClaimPool() *lqcCommitteeClaimPool {
	return &lqcCommitteeClaimPool{
		claims: make(map[lqcCommitteeClaimKey]lqc.CompactCommitteeParticipationV1),
	}
}

func cloneLQCCommitteeClaim(item lqc.CompactCommitteeParticipationV1) lqc.CompactCommitteeParticipationV1 {
	return lqc.CompactCommitteeParticipationV1{
		Position:  item.Position,
		Signature: append([]byte(nil), item.Signature...),
	}
}

func (p *lqcCommitteeClaimPool) add(group lqc.CommitteeParticipationClaimGroupV1) (bool, error) {
	if p == nil || group.TargetBlock == 0 || len(group.Participations) == 0 ||
		len(group.Participations) > lqc.MaxCommitteeParticipationsV1 {
		return false, errLQCCommitteeClaimPool
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, item := range group.Participations {
		key := lqcCommitteeClaimKey{target: group.TargetBlock, position: item.Position}
		if previous, ok := p.claims[key]; ok && !bytes.Equal(previous.Signature, item.Signature) {
			return false, lqc.ErrDuplicateCommitteeParticipationClaimV1
		}
	}
	changed := false
	for _, item := range group.Participations {
		key := lqcCommitteeClaimKey{target: group.TargetBlock, position: item.Position}
		if _, ok := p.claims[key]; ok {
			continue
		}
		p.claims[key] = cloneLQCCommitteeClaim(item)
		changed = true
	}
	return changed, nil
}

func (p *lqcCommitteeClaimPool) contains(
	group lqc.CommitteeParticipationClaimGroupV1,
) bool {
	if p == nil || group.TargetBlock == 0 || len(group.Participations) == 0 {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, item := range group.Participations {
		stored, ok := p.claims[lqcCommitteeClaimKey{
			target: group.TargetBlock, position: item.Position,
		}]
		if !ok || !bytes.Equal(stored.Signature, item.Signature) {
			return false
		}
	}
	return true
}

func (p *lqcCommitteeClaimPool) pending(inclusionBlock uint64, claimed func(uint64, uint8) bool) []lqc.CommitteeParticipationClaimGroupV1 {
	if p == nil || inclusionBlock <= 1 {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	byTarget := make(map[uint64][]lqc.CompactCommitteeParticipationV1)
	for key, item := range p.claims {
		if key.target >= inclusionBlock || inclusionBlock-key.target > lqc.CommitteeParticipationClaimWindowV1 {
			delete(p.claims, key)
			continue
		}
		if claimed != nil && claimed(key.target, key.position) {
			continue
		}
		byTarget[key.target] = append(byTarget[key.target], cloneLQCCommitteeClaim(item))
	}
	targets := make([]uint64, 0, len(byTarget))
	for target := range byTarget {
		targets = append(targets, target)
	}
	sort.Slice(targets, func(i, j int) bool { return targets[i] < targets[j] })

	groups := make([]lqc.CommitteeParticipationClaimGroupV1, 0, len(targets))
	total := 0
	for _, target := range targets {
		items := byTarget[target]
		sort.Slice(items, func(i, j int) bool { return items[i].Position < items[j].Position })
		if remaining := lqc.MaxCommitteeParticipationsV1 - total; len(items) > remaining {
			items = items[:remaining]
		}
		if len(items) > 0 {
			groups = append(groups, lqc.CommitteeParticipationClaimGroupV1{
				TargetBlock: target, Participations: items,
			})
			total += len(items)
		}
		if total == lqc.MaxCommitteeParticipationsV1 {
			break
		}
	}
	return groups
}
