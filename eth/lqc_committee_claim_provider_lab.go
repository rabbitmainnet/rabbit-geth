//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"sync"

	"github.com/ethereum/go-ethereum/consensus/lqc"
)

type lqcCommitteeClaimProviderLab struct {
	mu        sync.Mutex
	backend   *Ethereum
	pool      *lqcCommitteeClaimPool
	transport *lqcWorkV1Transport
	reserved  uint64
	groups    []lqc.CommitteeParticipationClaimGroupV1
}

func cloneLQCCommitteeClaimGroups(
	input []lqc.CommitteeParticipationClaimGroupV1,
) []lqc.CommitteeParticipationClaimGroupV1 {
	out := make([]lqc.CommitteeParticipationClaimGroupV1, len(input))
	for i, group := range input {
		out[i].TargetBlock = group.TargetBlock
		out[i].Participations = make([]lqc.CompactCommitteeParticipationV1, len(group.Participations))
		for j, item := range group.Participations {
			out[i].Participations[j] = cloneLQCCommitteeClaim(item)
		}
	}
	return out
}

func (p *lqcCommitteeClaimProviderLab) canonicalClaimed(
	inclusionBlock uint64,
) map[lqcCommitteeClaimKey]struct{} {
	claimed := make(map[lqcCommitteeClaimKey]struct{})
	if p == nil || p.backend == nil || p.backend.blockchain == nil || inclusionBlock <= 1 {
		return claimed
	}
	start := uint64(1)
	if inclusionBlock > lqc.CommitteeParticipationClaimWindowV1 {
		start = inclusionBlock - lqc.CommitteeParticipationClaimWindowV1
	}
	for number := start; number < inclusionBlock; number++ {
		header := p.backend.blockchain.GetHeaderByNumber(number)
		if header == nil {
			continue
		}
		envelope, err := lqc.DecodeLQCHeaderExtraV4(
			header.Extra, lqc.MaxWorkTicketsPerBlockV1,
		)
		if err != nil {
			continue
		}
		for _, group := range envelope.CommitteeParticipationClaims {
			for _, item := range group.Participations {
				claimed[lqcCommitteeClaimKey{target: group.TargetBlock, position: item.Position}] = struct{}{}
			}
		}
	}
	return claimed
}

func (p *lqcCommitteeClaimProviderLab) provide(
	inclusionBlock uint64,
) ([]lqc.CommitteeParticipationClaimGroupV1, error) {
	if p == nil || p.pool == nil || inclusionBlock <= 1 {
		return nil, errLQCWorkV1Context
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.reserved == inclusionBlock {
		return cloneLQCCommitteeClaimGroups(p.groups), nil
	}
	claimed := p.canonicalClaimed(inclusionBlock)
	groups := p.pool.pending(inclusionBlock, func(target uint64, position uint8) bool {
		_, ok := claimed[lqcCommitteeClaimKey{target: target, position: position}]
		return ok
	})
	// A claim is branch-bound. Recheck each compact proof against the current
	// canonical target context so a reorg cannot poison block preparation with
	// a signature made for the replaced branch.
	filtered := make([]lqc.CommitteeParticipationClaimGroupV1, 0, len(groups))
	for _, group := range groups {
		valid := lqc.CommitteeParticipationClaimGroupV1{TargetBlock: group.TargetBlock}
		for _, item := range group.Participations {
			single := lqc.CommitteeParticipationClaimGroupV1{
				TargetBlock:    group.TargetBlock,
				Participations: []lqc.CompactCommitteeParticipationV1{item},
			}
			if p.transport != nil && p.transport.verifyCommitteeClaimGroup(single) == nil {
				valid.Participations = append(valid.Participations, item)
			}
		}
		if len(valid.Participations) > 0 {
			filtered = append(filtered, valid)
		}
	}
	canonical, err := lqc.CanonicalCommitteeParticipationClaimGroupsV1(
		inclusionBlock, filtered,
	)
	if err != nil {
		return nil, err
	}
	p.reserved = inclusionBlock
	p.groups = cloneLQCCommitteeClaimGroups(canonical)
	return cloneLQCCommitteeClaimGroups(p.groups), nil
}

func wireLQCCommitteeClaimProviderLab(
	backend *Ethereum,
	transport *lqcWorkV1Transport,
	engine *lqc.LQC,
) error {
	if backend == nil || transport == nil || transport.claimPool == nil || engine == nil {
		return errLQCWorkV1Context
	}
	provider := &lqcCommitteeClaimProviderLab{
		backend:   backend,
		pool:      transport.claimPool,
		transport: transport,
	}
	return lqc.SetWorkV1EngineLabCommitteeClaimProvider(engine, provider.provide)
}
