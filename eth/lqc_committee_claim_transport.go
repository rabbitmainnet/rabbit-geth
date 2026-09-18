package eth

import (
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/rlp"
)

func lqcCommitteeClaimGroupHash(group lqc.CommitteeParticipationClaimGroupV1) common.Hash {
	encoded, _ := rlp.EncodeToBytes(group)
	return crypto.Keccak256Hash(encoded)
}

func (n *lqcWorkV1Transport) verifyCommitteeClaimGroup(
	group lqc.CommitteeParticipationClaimGroupV1,
) error {
	if n == nil || n.committeeContext == nil || n.claimPool == nil ||
		group.TargetBlock == 0 || len(group.Participations) == 0 ||
		len(group.Participations) > lqc.MaxCommitteeParticipationsV1 {
		return errLQCWorkV1Context
	}
	ctx, err := n.committeeContext(group.TargetBlock)
	if err != nil {
		return err
	}
	_, err = lqc.VerifyCommitteeParticipationClaimGroupV1(ctx, group)
	return err
}

func (n *lqcWorkV1Transport) acceptCommitteeClaims(
	groups []lqc.CommitteeParticipationClaimGroupV1,
	peer *lqcWorkV1Peer,
) error {
	if len(groups) == 0 {
		return errors.New("empty lqc committee claims packet")
	}
	total := 0
	accepted := make([]lqc.CommitteeParticipationClaimGroupV1, 0, len(groups))
	for _, group := range groups {
		total += len(group.Participations)
		if total > lqc.MaxCommitteeParticipationsV1 {
			return lqc.ErrTooManyCommitteeParticipationsV1
		}
		hash := lqcCommitteeClaimGroupHash(group)
		if peer != nil && peer.knows(hash) {
			continue
		}
		if n.claimPool.contains(group) {
			if peer != nil {
				peer.markKnown(hash)
			}
			continue
		}
		if err := n.verifyCommitteeClaimGroup(group); err != nil {
			return err
		}
		changed, err := n.claimPool.add(group)
		if err != nil && !errors.Is(err, lqc.ErrDuplicateCommitteeParticipationClaimV1) {
			return err
		}
		if peer != nil {
			peer.markKnown(hash)
		}
		if changed {
			accepted = append(accepted, group)
		}
	}
	if len(accepted) > 0 {
		except := ""
		if peer != nil {
			except = peer.id()
		}
		n.BroadcastCommitteeClaims(accepted, except)
	}
	return nil
}

func (n *lqcWorkV1Transport) SubmitCommitteeClaim(
	group lqc.CommitteeParticipationClaimGroupV1,
) error {
	if n == nil {
		return errLQCWorkV1TransportDisabled
	}
	return n.acceptCommitteeClaims([]lqc.CommitteeParticipationClaimGroupV1{group}, nil)
}

func (n *lqcWorkV1Transport) pendingCommitteeClaims(
	inclusionBlock uint64,
	claimed func(uint64, uint8) bool,
) []lqc.CommitteeParticipationClaimGroupV1 {
	if n == nil || n.claimPool == nil {
		return nil
	}
	return n.claimPool.pending(inclusionBlock, claimed)
}

func (n *lqcWorkV1Transport) BroadcastCommitteeClaims(
	groups []lqc.CommitteeParticipationClaimGroupV1,
	except string,
) {
	if n == nil || len(groups) == 0 {
		return
	}
	n.mu.RLock()
	peers := make([]*lqcWorkV1Peer, 0, len(n.peers))
	for id, peer := range n.peers {
		if id != except {
			peers = append(peers, peer)
		}
	}
	n.mu.RUnlock()
	for _, peer := range peers {
		peer := peer
		go func() {
			if err := peer.sendCommitteeClaims(groups); err != nil {
				peer.peer.Log().Debug("LQC committee claim broadcast failed", "err", err)
			}
		}()
	}
}

func (p *lqcWorkV1Peer) knows(hash common.Hash) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, ok := p.known[hash]
	return ok
}

func (p *lqcWorkV1Peer) sendCommitteeClaims(
	groups []lqc.CommitteeParticipationClaimGroupV1,
) error {
	if len(groups) == 0 {
		return nil
	}
	total := 0
	for _, group := range groups {
		total += len(group.Participations)
	}
	if total > lqc.MaxCommitteeParticipationsV1 {
		return fmt.Errorf("too many outbound lqc committee claims: %d", total)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	unknown := make([]lqc.CommitteeParticipationClaimGroupV1, 0, len(groups))
	for _, group := range groups {
		hash := lqcCommitteeClaimGroupHash(group)
		if _, ok := p.known[hash]; !ok {
			unknown = append(unknown, group)
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	if err := p2p.Send(p.rw, lqcWorkV1ClaimsMsg, unknown); err != nil {
		return err
	}
	for _, group := range unknown {
		if len(p.known) >= lqcWorkV1MaxKnownPerPeer {
			clear(p.known)
		}
		p.known[lqcCommitteeClaimGroupHash(group)] = struct{}{}
	}
	return nil
}
