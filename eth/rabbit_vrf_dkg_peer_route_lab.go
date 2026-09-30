//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"fmt"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/enode"
)

type rabbitVRFDKGPeerRouteProofPacketV1 struct {
	PeerID   common.Hash
	Envelope lqc.RabbitVRFDKGEnvelopeV1
}

func rabbitVRFDKGSignPeerRouteProofV1(wallets []accounts.Wallet, context lqc.RabbitVRFDKGSessionContextV1, member lqc.RabbitVRFCommitteeMemberV1, peerID common.Hash) (rabbitVRFDKGPeerRouteProofPacketV1, error) {
	var empty rabbitVRFDKGPeerRouteProofPacketV1
	root, err := lqc.RabbitVRFDKGPeerRoutePayloadHashV1(context, member, peerID)
	if err != nil {
		return empty, err
	}
	envelope, err := lqc.NewRabbitVRFDKGEnvelopeV1(context, member, lqc.RabbitVRFDKGMessagePeerRouteV1, root)
	if err != nil {
		return empty, err
	}
	signingData, err := lqc.RabbitVRFDKGEnvelopeSigningDataV1(context, envelope)
	if err != nil {
		return empty, err
	}
	var lastErr error
	for _, wallet := range wallets {
		for _, account := range wallet.Accounts() {
			if account.Address != member.Participant {
				continue
			}
			signature, signErr := wallet.SignData(account, accounts.MimetypeClique, signingData)
			if signErr != nil {
				lastErr = signErr
				continue
			}
			envelope.Signature = append([]byte(nil), signature...)
			if verifyErr := lqc.VerifyRabbitVRFDKGEnvelopeV1(context, member, envelope); verifyErr != nil {
				lastErr = verifyErr
				envelope.Signature = nil
				continue
			}
			return rabbitVRFDKGPeerRouteProofPacketV1{PeerID: peerID, Envelope: envelope}, nil
		}
	}
	if lastErr != nil {
		return empty, fmt.Errorf("sign rabbit vrf dkg peer route: %w", lastErr)
	}
	return empty, fmt.Errorf("rabbit vrf dkg route wallet unavailable for %s", member.Participant)
}

func rabbitVRFDKGVerifyPeerRouteProofV1(context lqc.RabbitVRFDKGSessionContextV1, member lqc.RabbitVRFCommitteeMemberV1, expectedPeerID common.Hash, packet rabbitVRFDKGPeerRouteProofPacketV1) error {
	if expectedPeerID == (common.Hash{}) || packet.PeerID != expectedPeerID {
		return fmt.Errorf("rabbit vrf dkg peer route peer id mismatch")
	}
	root, err := lqc.RabbitVRFDKGPeerRoutePayloadHashV1(context, member, packet.PeerID)
	if err != nil {
		return err
	}
	if packet.Envelope.MessageType != lqc.RabbitVRFDKGMessagePeerRouteV1 || packet.Envelope.PayloadHash != root || packet.Envelope.SenderShareID != member.ShareID || packet.Envelope.Participant != member.Participant {
		return fmt.Errorf("rabbit vrf dkg peer route payload mismatch")
	}
	return lqc.VerifyRabbitVRFDKGEnvelopeV1(context, member, packet.Envelope)
}

func (n *rabbitVRFDKGTransport) storePeerRouteProofV1(peer *rabbitVRFDKGPeer, packet rabbitVRFDKGPeerRouteProofPacketV1) error {
	if n == nil || n.runtime == nil || peer == nil || peer.peer == nil {
		return fmt.Errorf("rabbit vrf dkg peer route runtime unavailable")
	}
	context := n.runtime.currentContext()
	if context.SessionID == (common.Hash{}) || packet.Envelope.SessionID != context.SessionID {
		return errRabbitVRFDKGArtifactSessionMismatch
	}
	var member lqc.RabbitVRFCommitteeMemberV1
	found := false
	for _, candidate := range context.CanonicalMembers {
		if candidate.ShareID == packet.Envelope.SenderShareID && candidate.Participant == packet.Envelope.Participant {
			member = candidate
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("rabbit vrf dkg peer route sender not in canonical committee")
	}
	expectedPeerID := common.Hash(peer.peer.ID())
	if err := rabbitVRFDKGVerifyPeerRouteProofV1(context.CanonicalSession, member, expectedPeerID, packet); err != nil {
		return err
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.closed {
		return errRabbitVRFDKGProtocolClosed
	}
	if n.routeSession != context.SessionID {
		n.routeSession = context.SessionID
		n.routes = make(map[uint64]string)
	}
	peerID := peer.id()
	if existing, ok := n.routes[member.ShareID]; ok && existing != peerID {
		return errRabbitVRFDKGArtifactConflict
	}
	n.routes[member.ShareID] = peerID
	return nil
}

func (n *rabbitVRFDKGTransport) peerForShareV1(sessionID common.Hash, shareID uint64) (*rabbitVRFDKGPeer, bool) {
	if n == nil || sessionID == (common.Hash{}) || shareID == 0 {
		return nil, false
	}
	n.mu.RLock()
	defer n.mu.RUnlock()
	if n.closed || n.routeSession != sessionID {
		return nil, false
	}
	peerID, ok := n.routes[shareID]
	if !ok {
		return nil, false
	}
	peer, ok := n.peers[peerID]
	if !ok || peer == nil {
		return nil, false
	}
	return peer, true
}

func (peer *rabbitVRFDKGPeer) sendPeerRouteProofV1(packet rabbitVRFDKGPeerRouteProofPacketV1) error {
	if peer == nil || peer.rw == nil {
		return fmt.Errorf("invalid rabbit vrf dkg peer route transport")
	}
	peer.mu.Lock()
	defer peer.mu.Unlock()
	return p2p.Send(peer.rw, rabbitVRFDKGPeerRouteMsg, packet)
}

func (n *rabbitVRFDKGTransport) sendLocalPeerRouteProofsV1(peer *rabbitVRFDKGPeer) error {
	if n == nil || n.runtime == nil || peer == nil || n.runtime.backend == nil || n.runtime.backend.accountManager == nil || n.runtime.backend.p2pServer == nil || n.runtime.backend.p2pServer.PrivateKey == nil {
		return nil
	}
	context := n.runtime.currentContext()
	if context.SessionID == (common.Hash{}) || len(context.Members) == 0 {
		return nil
	}
	localPeerID := common.Hash(enode.PubkeyToIDV4(&n.runtime.backend.p2pServer.PrivateKey.PublicKey))
	if localPeerID == (common.Hash{}) {
		return fmt.Errorf("zero rabbit vrf dkg local peer id")
	}
	wallets := n.runtime.backend.accountManager.Wallets()
	for _, member := range context.Members {
		packet, err := rabbitVRFDKGSignPeerRouteProofV1(wallets, context.CanonicalSession, member, localPeerID)
		if err != nil {
			return err
		}
		if err := peer.sendPeerRouteProofV1(packet); err != nil {
			return err
		}
	}
	return nil
}
