//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"errors"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	"sync"
)

// This cache holds ciphertext and public delivery IDs, never secrets. One packet per dealer/recipient
// slot bounds memory independently of randomized ECIES message identifiers.
type rabbitVRFEncryptedRelayCacheV1 struct {
	recipientMu     sync.Mutex                // Serializes recipient persistence without holding the transport lock.
	delivered       map[[2]uint64]common.Hash // IDs only; populated after successful verified persistence.
	deliveredRecent map[[2]uint64][]common.Hash
	session         common.Hash
	packets         map[[2]uint64]lqc.RabbitVRFDKGEncryptedEvaluationV1
}

// A restart or independent encryption can yield multiple valid ciphertext IDs
// for the same evaluation slot. Remember a bounded window of completed IDs so
// alternating authenticated replays do not repeatedly reopen encrypted stores.
const rabbitVRFDeliveredReplayWindowV1 = 8

// The transport mutex protects both methods. No plaintext or key is retained.
func (cache *rabbitVRFEncryptedRelayCacheV1) deliveredIDV1(slot [2]uint64, id common.Hash) bool {
	if cache.delivered[slot] == id && id != (common.Hash{}) {
		return true
	}
	for _, previous := range cache.deliveredRecent[slot] {
		if previous == id {
			return true
		}
	}
	return false
}

func (cache *rabbitVRFEncryptedRelayCacheV1) rememberDeliveredIDV1(slot [2]uint64, id common.Hash) {
	if id == (common.Hash{}) || cache.deliveredIDV1(slot, id) {
		return
	}
	if cache.delivered == nil {
		cache.delivered = make(map[[2]uint64]common.Hash)
	}
	if cache.deliveredRecent == nil {
		cache.deliveredRecent = make(map[[2]uint64][]common.Hash)
	}
	ids := cache.deliveredRecent[slot]
	if len(ids) == rabbitVRFDeliveredReplayWindowV1 {
		copy(ids, ids[1:])
		ids = ids[:len(ids)-1]
	}
	cache.deliveredRecent[slot] = append(ids, id)
	cache.delivered[slot] = id
}

func cloneRabbitVRFEncryptedRelayPacketV1(p lqc.RabbitVRFDKGEncryptedEvaluationV1) lqc.RabbitVRFDKGEncryptedEvaluationV1 {
	p.Ciphertext = append([]byte(nil), p.Ciphertext...)
	p.Signature = append([]byte(nil), p.Signature...)
	return p
}

func (n *rabbitVRFDKGTransport) retainEncryptedRelayPacketV1(p lqc.RabbitVRFDKGEncryptedEvaluationV1) (bool, error) {
	if err := n.runtime.validateInboundEncryptedEvaluationV1(p); err != nil {
		return false, err
	}
	context := n.runtime.currentContext()
	if p.RecipientShareID > uint64(len(context.CanonicalTransportBindings)) ||
		p.RecipientShareID > uint64(len(context.CanonicalTransportEnvelopes)) {
		return false, errRabbitVRFDKGSecretTemporarilyUnavailableV1
	}
	recipient := context.CanonicalMembers[p.RecipientShareID-1]
	binding := context.CanonicalTransportBindings[p.RecipientShareID-1]
	envelope := context.CanonicalTransportEnvelopes[p.RecipientShareID-1]
	if err := lqc.VerifyRabbitVRFDKGTransportKeyEnvelopeV1(context.CanonicalSession, recipient, binding, envelope); err != nil {
		return false, err
	}
	root, err := lqc.RabbitVRFDKGTransportKeyRootV1(context.CanonicalSession, binding)
	if err != nil || root != p.RecipientTransportKeyRoot {
		return false, errors.New("rabbit vrf encrypted relay recipient binding mismatch")
	}
	commitment, found := n.commitmentForDealerV1(p.SessionID, p.DealerShareID)
	if !found {
		return false, errRabbitVRFDKGSecretTemporarilyUnavailableV1
	}
	root, err = lqc.ValidateRabbitVRFDKGPolynomialCommitmentV1(context.CanonicalSession, commitment)
	if err != nil || root != p.CommitmentRoot {
		return false, errors.New("rabbit vrf encrypted relay commitment mismatch")
	}
	id, err := lqc.RabbitVRFDKGEncryptedEvaluationIDV1(p)
	if err != nil {
		return false, err
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.closed {
		return false, errors.New("rabbit vrf relay closed")
	}
	if n.encryptedRelay.session != p.SessionID {
		n.encryptedRelay.session = p.SessionID
		n.encryptedRelay.delivered = make(map[[2]uint64]common.Hash)
		n.encryptedRelay.deliveredRecent = make(map[[2]uint64][]common.Hash)
		n.encryptedRelay.packets = make(map[[2]uint64]lqc.RabbitVRFDKGEncryptedEvaluationV1)
	}
	slot := [2]uint64{p.DealerShareID, p.RecipientShareID}
	if previous, ok := n.encryptedRelay.packets[slot]; ok {
		previousID, err := lqc.RabbitVRFDKGEncryptedEvaluationIDV1(previous)
		if err != nil {
			return false, err
		}
		if previousID == id {
			return false, nil
		}
	}

	n.encryptedRelay.packets[slot] = cloneRabbitVRFEncryptedRelayPacketV1(p)
	return true, nil
}

func (n *rabbitVRFDKGTransport) broadcastEncryptedRelayPacketV1(p lqc.RabbitVRFDKGEncryptedEvaluationV1, exclude string, replay ...bool) error {
	n.mu.RLock()
	peers := make([]*rabbitVRFDKGPeer, 0, len(n.peers))
	for id, peer := range n.peers {
		if peer != nil && id != exclude {
			peers = append(peers, peer)
		}
	}
	n.mu.RUnlock()
	if len(peers) == 0 {
		return fmt.Errorf("rabbit vrf encrypted relay has no connected peer")
	}
	var first error
	for _, peer := range peers {
		if err := peer.sendEncryptedEvaluationV1(p, replay...); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func (n *rabbitVRFDKGTransport) handleInboundEncryptedRelayV1(peer *rabbitVRFDKGPeer, p lqc.RabbitVRFDKGEncryptedEvaluationV1) error {
	inserted, err := n.retainEncryptedRelayPacketV1(p)
	if err != nil {
		return err
	}
	context := n.runtime.currentContext()
	local := false
	for _, member := range context.Members {
		if member.ShareID == p.RecipientShareID && member.Participant == p.RecipientParticipant {
			local = true
			break
		}
	}
	if local {
		// Retention above authenticates every replay before this ID cache is
		// consulted. Cache no secrets and never mark a deferred or failed write.
		n.encryptedRelay.recipientMu.Lock()
		defer n.encryptedRelay.recipientMu.Unlock()
		id, err := lqc.RabbitVRFDKGEncryptedEvaluationIDV1(p)
		if err != nil {
			return err
		}
		slot := [2]uint64{p.DealerShareID, p.RecipientShareID}
		n.mu.RLock()
		done := n.encryptedRelay.session == p.SessionID && n.encryptedRelay.deliveredIDV1(slot, id)
		n.mu.RUnlock()
		if done {
			return nil
		}
		if _, err := n.runtime.decryptInboundEncryptedEvaluationV1(p); err != nil {
			return err
		}
		n.mu.Lock()
		if !n.closed && n.encryptedRelay.session == p.SessionID {
			n.encryptedRelay.rememberDeliveredIDV1(slot, id)
		}
		n.mu.Unlock()
		return nil
	}
	if inserted {
		// A relay never decrypts. Failed links are retried by periodic cache replay.
		_ = n.broadcastEncryptedRelayPacketV1(p, peer.id())
	}
	return nil
}

func (n *rabbitVRFDKGTransport) sendPendingAuthenticatedRelayV1(peer *rabbitVRFDKGPeer) error {
	context := n.runtime.currentContext()
	n.mu.RLock()
	encrypted := make([]lqc.RabbitVRFDKGEncryptedEvaluationV1, 0)
	if n.encryptedRelay.session == context.SessionID {
		for _, p := range n.encryptedRelay.packets {
			encrypted = append(encrypted, cloneRabbitVRFEncryptedRelayPacketV1(p))
		}
	}
	partials := make([]lqc.RabbitVRFThresholdPartialV1, 0)
	if n.partialSession == context.SessionID {
		for _, bucket := range n.partials {
			for _, p := range bucket {
				partials = append(partials, p)
			}
		}
	}
	n.mu.RUnlock()
	for _, p := range encrypted {
		if err := peer.sendEncryptedEvaluationV1(p, true); err != nil {
			return err
		}
	}
	for _, p := range partials {
		if err := peer.sendThresholdPartialV1(p); err != nil {
			return err
		}
	}
	return nil
}
