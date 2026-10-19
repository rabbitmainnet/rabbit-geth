//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"errors"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
)

// This cache holds ciphertext only. One packet per canonical dealer/recipient
// slot bounds memory independently of randomized ECIES message identifiers.
type rabbitVRFEncryptedRelayCacheV1 struct {
	session common.Hash
	packets map[[2]uint64]lqc.RabbitVRFDKGEncryptedEvaluationV1
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

func (n *rabbitVRFDKGTransport) broadcastEncryptedRelayPacketV1(p lqc.RabbitVRFDKGEncryptedEvaluationV1, exclude string) error {
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
		if err := peer.sendEncryptedEvaluationV1(p); err != nil && first == nil {
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
		// Only the recipient opens its transport key and decrypts. Duplicate sends
		// retry local processing when synchronization previously delayed it.
		_, err := n.runtime.decryptInboundEncryptedEvaluationV1(p)
		return err
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
		if err := peer.sendEncryptedEvaluationV1(p); err != nil {
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
