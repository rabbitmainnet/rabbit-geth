//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/internal/rabbitvrfstate"
	"github.com/ethereum/go-ethereum/p2p"
)

func (n *rabbitVRFDKGTransport) currentKeysetCertificateBaseV1() (
	lqc.RabbitVRFDKGSessionContextV1,
	[]lqc.RabbitVRFCommitteeMemberV1,
	lqc.RabbitVRFKeysetCertificateV1,
	common.Hash,
	bool,
	error,
) {
	var (
		session     lqc.RabbitVRFDKGSessionContextV1
		certificate lqc.RabbitVRFKeysetCertificateV1
	)

	if n == nil ||
		n.runtime == nil ||
		n.runtime.backend == nil ||
		n.runtime.backend.vrfDKGInstanceDir == "" {
		return session, nil, certificate, common.Hash{}, false, nil
	}

	current := n.runtime.currentContext()
	if current.SessionID == (common.Hash{}) {
		return session, nil, certificate, common.Hash{}, false, nil
	}

	session = current.CanonicalSession

	sessionID, err := lqc.RabbitVRFDKGSessionIDV1(session)
	if err != nil {
		return session, nil, certificate, common.Hash{}, false, err
	}
	if sessionID != current.SessionID {
		return session, nil, certificate, common.Hash{}, false,
			errRabbitVRFDKGArtifactSessionMismatch
	}

	members := append(
		[]lqc.RabbitVRFCommitteeMemberV1(nil),
		current.CanonicalMembers...,
	)
	if uint64(len(members)) != session.CommitteeSize {
		return session, nil, certificate, common.Hash{}, false,
			errors.New("rabbit vrf canonical certificate committee mismatch")
	}

	keysetStore, err := rabbitvrfstate.NewDKGFinalKeysetStoreV1(
		filepath.Join(
			n.runtime.backend.vrfDKGInstanceDir,
			"rabbit-vrf",
			"dkg-final-keysets",
		),
	)
	if err != nil {
		return session, nil, certificate, common.Hash{}, false, err
	}

	keyset, err := keysetStore.Load(session)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) ||
			errors.Is(
				err,
				rabbitvrfstate.ErrDKGTransportKeyStoreMissingV1,
			) {
			return session, members, certificate, common.Hash{},
				false, nil
		}
		return session, nil, certificate, common.Hash{}, false, err
	}

	certificate = lqc.RabbitVRFKeysetCertificateV1{
		Version:            lqc.RabbitVRFKeysetCertificateVersionV1,
		SessionID:          sessionID,
		KeysetRoot:         keyset.KeysetRoot,
		ThresholdPublicKey: keyset.ThresholdPublicKey,
		TranscriptRoot:     keyset.TranscriptRoot,
	}

	payloadHash, err :=
		lqc.RabbitVRFKeysetCertificatePayloadHashV1(
			session,
			certificate,
		)
	if err != nil {
		return session, nil, certificate, common.Hash{}, false, err
	}

	return session, members, certificate, payloadHash, true, nil
}

func rabbitVRFDKGSignKeysetCertificateEnvelopeV1(
	wallets []accounts.Wallet,
	session lqc.RabbitVRFDKGSessionContextV1,
	member lqc.RabbitVRFCommitteeMemberV1,
	payloadHash common.Hash,
) (
	lqc.RabbitVRFDKGEnvelopeV1,
	bool,
	error,
) {
	var empty lqc.RabbitVRFDKGEnvelopeV1

	envelope, err := lqc.NewRabbitVRFDKGEnvelopeV1(
		session,
		member,
		lqc.RabbitVRFDKGMessageKeysetCertificateV1,
		payloadHash,
	)
	if err != nil {
		return empty, false, err
	}

	signingData, err :=
		lqc.RabbitVRFDKGEnvelopeSigningDataV1(
			session,
			envelope,
		)
	if err != nil {
		return empty, false, err
	}

	var lastErr error
	found := false

	for _, wallet := range wallets {
		for _, account := range wallet.Accounts() {
			if account.Address != member.Participant {
				continue
			}

			found = true

			signature, signErr := wallet.SignData(
				account,
				accounts.MimetypeClique,
				signingData,
			)
			if signErr != nil {
				lastErr = signErr
				continue
			}

			envelope.Signature =
				append([]byte(nil), signature...)

			if verifyErr :=
				lqc.VerifyRabbitVRFDKGEnvelopeV1(
					session,
					member,
					envelope,
				); verifyErr != nil {
				lastErr = verifyErr
				envelope.Signature = nil
				continue
			}

			return envelope, true, nil
		}
	}

	if found && lastErr != nil {
		return empty, false, lastErr
	}

	return empty, false, nil
}

func (n *rabbitVRFDKGTransport) collectKeysetCertificateEnvelopeV1(
	envelope lqc.RabbitVRFDKGEnvelopeV1,
) (
	bool,
	error,
) {
	session, members, certificate, payloadHash, ready, err :=
		n.currentKeysetCertificateBaseV1()
	if err != nil {
		return false, err
	}
	if !ready {
		return false, errRabbitVRFDKGArtifactSessionMismatch
	}

	if envelope.SessionID != certificate.SessionID ||
		envelope.MessageType !=
			lqc.RabbitVRFDKGMessageKeysetCertificateV1 ||
		envelope.PayloadHash != payloadHash ||
		envelope.SenderShareID == 0 ||
		envelope.SenderShareID > uint64(len(members)) {
		return false, errors.New(
			"invalid rabbit vrf keyset certificate envelope binding",
		)
	}

	member := members[envelope.SenderShareID-1]

	if member.ShareID != envelope.SenderShareID ||
		member.Participant != envelope.Participant {
		return false, errors.New(
			"rabbit vrf keyset certificate member mismatch",
		)
	}

	if err := lqc.VerifyRabbitVRFDKGEnvelopeV1(
		session,
		member,
		envelope,
	); err != nil {
		return false, err
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	if n.keysetCertificateSession != certificate.SessionID ||
		n.keysetCertificateRoot != certificate.KeysetRoot {
		n.keysetCertificateSession = certificate.SessionID
		n.keysetCertificateRoot = certificate.KeysetRoot
		n.keysetCertificateSignatures =
			make(map[uint64][]byte)
		n.keysetCertificate =
			lqc.RabbitVRFKeysetCertificateV1{}
	}

	if n.keysetCertificate.Version ==
		lqc.RabbitVRFKeysetCertificateVersionV1 &&
		n.keysetCertificate.SessionID == certificate.SessionID &&
		n.keysetCertificate.KeysetRoot == certificate.KeysetRoot {
		return false, nil
	}

	if n.keysetCertificateSignatures == nil {
		n.keysetCertificateSignatures =
			make(map[uint64][]byte)
	}

	if _, exists :=
		n.keysetCertificateSignatures[envelope.SenderShareID]; exists {
		return false, nil
	}

	n.keysetCertificateSignatures[envelope.SenderShareID] = append([]byte(nil), envelope.Signature...)

	if uint64(len(n.keysetCertificateSignatures)) < session.Threshold {
		return true, nil
	}

	certificate.Signatures =
		make([][]byte, len(members))

	for index, expected := range members {
		signature, ok :=
			n.keysetCertificateSignatures[expected.ShareID]
		if !ok {
			continue
		}

		certificate.Signatures[index] =
			append([]byte(nil), signature...)
	}

	validated, err :=
		lqc.ValidateRabbitVRFKeysetCertificateV1(
			session,
			members,
			certificate,
		)
	if err != nil {
		return false, err
	}

	store, err := n.keysetCertificateStoreV1()
	if err != nil {
		return false, err
	}
	if err := store.Store(session, members, validated); err != nil {
		return false, err
	}

	n.keysetCertificate = validated
	return true, nil
}

func (n *rabbitVRFDKGTransport) keysetCertificatePeerAuthenticatedV1(
	peer *rabbitVRFDKGPeer,
	envelope lqc.RabbitVRFDKGEnvelopeV1,
) bool {
	if n == nil ||
		peer == nil ||
		envelope.SessionID == (common.Hash{}) ||
		envelope.SenderShareID == 0 {
		return false
	}

	n.mu.RLock()
	defer n.mu.RUnlock()

	if n.closed ||
		n.routeSession != envelope.SessionID {
		return false
	}

	peerID, ok := n.routes[envelope.SenderShareID]
	return ok && peerID == peer.id()
}

func (n *rabbitVRFDKGTransport) handleInboundKeysetCertificateEnvelopeV1(
	peer *rabbitVRFDKGPeer,
	envelope lqc.RabbitVRFDKGEnvelopeV1,
) (
	bool,
	error,
) {
	if !n.keysetCertificatePeerAuthenticatedV1(
		peer,
		envelope,
	) {
		return false, errors.New(
			"rabbit vrf keyset certificate sender route mismatch",
		)
	}

	return n.collectKeysetCertificateEnvelopeV1(envelope)
}

func (peer *rabbitVRFDKGPeer) sendKeysetCertificateEnvelopeV1(
	envelope lqc.RabbitVRFDKGEnvelopeV1,
) error {
	if peer == nil || peer.rw == nil {
		return errors.New(
			"invalid rabbit vrf keyset certificate peer transport",
		)
	}

	messageID := crypto.Keccak256Hash(
		envelope.PayloadHash[:],
		envelope.Signature,
	)

	peer.mu.Lock()
	defer peer.mu.Unlock()

	if peer.known == nil {
		peer.known = make(map[common.Hash]struct{})
	}

	if _, known := peer.known[messageID]; known {
		return nil
	}

	if err := p2p.Send(
		peer.rw,
		rabbitVRFDKGKeysetCertificateMsg,
		envelope,
	); err != nil {
		return err
	}

	if len(peer.known) >= rabbitVRFDKGMaxKnownPerPeer {
		clear(peer.known)
	}
	peer.known[messageID] = struct{}{}

	return nil
}

func (n *rabbitVRFDKGTransport) broadcastKeysetCertificateEnvelopeV1(
	envelope lqc.RabbitVRFDKGEnvelopeV1,
	excludePeerID string,
) {
	if n == nil {
		return
	}

	n.mu.RLock()
	peers := make([]*rabbitVRFDKGPeer, 0, len(n.peers))
	for id, peer := range n.peers {
		if id != excludePeerID && peer != nil {
			peers = append(peers, peer)
		}
	}
	n.mu.RUnlock()

	for _, peer := range peers {
		_ = peer.sendKeysetCertificateEnvelopeV1(envelope)
	}
}

func (n *rabbitVRFDKGTransport) sendRetainedKeysetCertificateSignaturesV1(
	peer *rabbitVRFDKGPeer,
) error {
	if n == nil || peer == nil {
		return nil
	}

	if err := n.restorePersistedKeysetCertificateV1(); err != nil {
		return err
	}

	session, members, certificate, payloadHash, ready, err :=
		n.currentKeysetCertificateBaseV1()
	if err != nil || !ready {
		return err
	}

	n.mu.RLock()
	signatures := make(map[uint64][]byte)

	if n.keysetCertificateSession == certificate.SessionID &&
		n.keysetCertificateRoot == certificate.KeysetRoot {
		for shareID, signature := range n.keysetCertificateSignatures {
			signatures[shareID] =
				append([]byte(nil), signature...)
		}
	}
	n.mu.RUnlock()

	for _, member := range members {
		signature, ok := signatures[member.ShareID]
		if !ok {
			continue
		}

		envelope, err := lqc.NewRabbitVRFDKGEnvelopeV1(
			session,
			member,
			lqc.RabbitVRFDKGMessageKeysetCertificateV1,
			payloadHash,
		)
		if err != nil {
			return err
		}

		envelope.Signature =
			append([]byte(nil), signature...)

		if err := peer.sendKeysetCertificateEnvelopeV1(
			envelope,
		); err != nil {
			return err
		}
	}

	return nil
}

func (n *rabbitVRFDKGTransport) publishLocalKeysetCertificateSignaturesV1() error {
	session, members, certificate, payloadHash, ready, err :=
		n.currentKeysetCertificateBaseV1()
	if err != nil || !ready {
		return err
	}

	if n.runtime.backend.accountManager == nil {
		return nil
	}

	wallets := n.runtime.backend.accountManager.Wallets()

	for _, member := range members {
		n.mu.RLock()
		sameCertificate :=
			n.keysetCertificateSession ==
				certificate.SessionID &&
				n.keysetCertificateRoot ==
					certificate.KeysetRoot
		_, already :=
			n.keysetCertificateSignatures[member.ShareID]
		n.mu.RUnlock()

		if sameCertificate && already {
			continue
		}

		envelope, found, signErr :=
			rabbitVRFDKGSignKeysetCertificateEnvelopeV1(
				wallets,
				session,
				member,
				payloadHash,
			)
		if signErr != nil {
			return fmt.Errorf(
				"sign rabbit vrf keyset certificate share %d: %w",
				member.ShareID,
				signErr,
			)
		}
		if !found {
			continue
		}

		if _, collectErr :=
			n.collectKeysetCertificateEnvelopeV1(
				envelope,
			); collectErr != nil {
			return fmt.Errorf(
				"collect local rabbit vrf keyset certificate share %d: %w",
				member.ShareID,
				collectErr,
			)
		}
	}

	n.mu.RLock()
	signatures :=
		make(map[uint64][]byte,
			len(n.keysetCertificateSignatures))
	if n.keysetCertificateSession ==
		certificate.SessionID &&
		n.keysetCertificateRoot ==
			certificate.KeysetRoot {
		for shareID, signature := range n.keysetCertificateSignatures {
			signatures[shareID] =
				append([]byte(nil), signature...)
		}
	}
	n.mu.RUnlock()

	for _, member := range members {
		signature, ok := signatures[member.ShareID]
		if !ok {
			continue
		}

		envelope, envelopeErr :=
			lqc.NewRabbitVRFDKGEnvelopeV1(
				session,
				member,
				lqc.RabbitVRFDKGMessageKeysetCertificateV1,
				payloadHash,
			)
		if envelopeErr != nil {
			return envelopeErr
		}

		envelope.Signature =
			append([]byte(nil), signature...)

		n.broadcastKeysetCertificateEnvelopeV1(
			envelope,
			"",
		)
	}

	return nil
}

func cloneRabbitVRFKeysetCertificateV1(
	certificate lqc.RabbitVRFKeysetCertificateV1,
) lqc.RabbitVRFKeysetCertificateV1 {
	out := certificate
	out.Signatures =
		make([][]byte, len(certificate.Signatures))

	for index, signature := range certificate.Signatures {
		out.Signatures[index] =
			append([]byte(nil), signature...)
	}

	return out
}

func (n *rabbitVRFDKGTransport) rabbitVRFKeysetCertificateForBlockV1(
	blockNumber uint64,
) (
	lqc.RabbitVRFKeysetCertificateV1,
	bool,
	error,
) {
	var empty lqc.RabbitVRFKeysetCertificateV1

	if n == nil || n.runtime == nil {
		return empty, false, nil
	}

	if err := n.restorePersistedKeysetCertificateV1(); err != nil {
		return empty, false, err
	}

	if err :=
		n.publishLocalKeysetCertificateSignaturesV1(); err != nil {
		return empty, false, err
	}

	n.mu.RLock()
	certificate :=
		cloneRabbitVRFKeysetCertificateV1(
			n.keysetCertificate,
		)
	ready :=
		certificate.Version ==
			lqc.RabbitVRFKeysetCertificateVersionV1 &&
			len(certificate.Signatures) > 0
	n.mu.RUnlock()

	if !ready {
		return empty, false, nil
	}

	finalizations, err :=
		n.rabbitVRFFinalizationsForBlockV1(blockNumber)
	if err != nil {
		return empty, false, err
	}
	if len(finalizations) == 0 {
		return empty, false, nil
	}

	return certificate, true, nil
}
