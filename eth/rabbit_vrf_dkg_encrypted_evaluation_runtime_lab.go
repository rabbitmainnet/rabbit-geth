package eth

import (
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"path/filepath"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	gethcrypto "github.com/ethereum/go-ethereum/crypto"
	rabbitvrf "github.com/ethereum/go-ethereum/crypto/rabbitvrf"
	"github.com/ethereum/go-ethereum/internal/rabbitvrfstate"
	"github.com/ethereum/go-ethereum/p2p"
)

func rabbitVRFDKGSignEncryptedEvaluationV1(
	wallets []accounts.Wallet,
	context lqc.RabbitVRFDKGSessionContextV1,
	dealer lqc.RabbitVRFCommitteeMemberV1,
	message lqc.RabbitVRFDKGEncryptedEvaluationV1,
) (
	lqc.RabbitVRFDKGEncryptedEvaluationV1,
	error,
) {
	if message.DealerShareID != dealer.ShareID ||
		message.DealerParticipant != dealer.Participant {
		return lqc.RabbitVRFDKGEncryptedEvaluationV1{},
			fmt.Errorf("rabbit vrf dkg encrypted evaluation dealer mismatch")
	}

	signingData, err :=
		lqc.RabbitVRFDKGEncryptedEvaluationSigningDataV1(
			context,
			message,
		)
	if err != nil {
		return lqc.RabbitVRFDKGEncryptedEvaluationV1{},
			fmt.Errorf(
				"encode rabbit vrf dkg encrypted evaluation signing data: %w",
				err,
			)
	}

	var lastErr error

	for _, wallet := range wallets {
		for _, account := range wallet.Accounts() {
			if account.Address != dealer.Participant {
				continue
			}

			signature, signErr := wallet.SignData(
				account,
				accounts.MimetypeClique,
				signingData,
			)
			if signErr != nil {
				lastErr = signErr
				continue
			}

			out := message
			out.Signature = append(
				[]byte(nil),
				signature...,
			)

			if verifyErr :=
				lqc.VerifyRabbitVRFDKGEncryptedEvaluationSignatureV1(
					context,
					dealer,
					out,
				); verifyErr != nil {
				lastErr = verifyErr
				continue
			}

			return out, nil
		}
	}

	if lastErr != nil {
		return lqc.RabbitVRFDKGEncryptedEvaluationV1{},
			fmt.Errorf(
				"sign rabbit vrf dkg encrypted evaluation: %w",
				lastErr,
			)
	}

	return lqc.RabbitVRFDKGEncryptedEvaluationV1{},
		fmt.Errorf(
			"rabbit vrf dkg dealer wallet unavailable for %s",
			dealer.Participant,
		)
}

func rabbitVRFDKGBuildEncryptedEvaluationV1(
	store *rabbitvrfstate.DKGDealerPolynomialStoreV1,
	wallets []accounts.Wallet,
	context lqc.RabbitVRFDKGSessionContextV1,
	dealer lqc.RabbitVRFCommitteeMemberV1,
	dealerCommitment lqc.RabbitVRFDKGPolynomialCommitmentV1,
	recipient lqc.RabbitVRFCommitteeMemberV1,
	recipientBinding lqc.RabbitVRFDKGTransportKeyBindingV1,
	recipientBindingEnvelope lqc.RabbitVRFDKGEnvelopeV1,
	password string,
) (
	lqc.RabbitVRFDKGEncryptedEvaluationV1,
	error,
) {
	if store == nil || password == "" {
		return lqc.RabbitVRFDKGEncryptedEvaluationV1{},
			fmt.Errorf("invalid rabbit vrf dkg dealer polynomial runtime")
	}

	polynomial, persistedCommitment, _, err :=
		store.Load(
			context,
			dealer,
			password,
		)
	if err != nil {
		return lqc.RabbitVRFDKGEncryptedEvaluationV1{},
			fmt.Errorf(
				"load rabbit vrf dkg dealer polynomial: %w",
				err,
			)
	}
	defer polynomial.Destroy()

	if !rabbitVRFDKGPolynomialCommitmentsEqualV1(
		persistedCommitment,
		dealerCommitment,
	) {
		return lqc.RabbitVRFDKGEncryptedEvaluationV1{},
			fmt.Errorf(
				"rabbit vrf dkg persisted dealer commitment mismatch",
			)
	}

	evaluation, err :=
		polynomial.Evaluate(
			recipient.ShareID,
		)
	if err != nil {
		return lqc.RabbitVRFDKGEncryptedEvaluationV1{},
			fmt.Errorf(
				"evaluate rabbit vrf dkg dealer polynomial: %w",
				err,
			)
	}

	message, _, _, err :=
		lqc.NewRabbitVRFDKGEncryptedEvaluationV1(
			context,
			dealer,
			dealerCommitment,
			recipient,
			recipientBinding,
			recipientBindingEnvelope,
			evaluation,
		)
	if err != nil {
		return lqc.RabbitVRFDKGEncryptedEvaluationV1{},
			fmt.Errorf(
				"encrypt rabbit vrf dkg private evaluation: %w",
				err,
			)
	}

	signed, err :=
		rabbitVRFDKGSignEncryptedEvaluationV1(
			wallets,
			context,
			dealer,
			message,
		)
	if err != nil {
		return lqc.RabbitVRFDKGEncryptedEvaluationV1{},
			err
	}

	return signed, nil
}

func rabbitVRFDKGPolynomialCommitmentsEqualV1(
	left lqc.RabbitVRFDKGPolynomialCommitmentV1,
	right lqc.RabbitVRFDKGPolynomialCommitmentV1,
) bool {
	if left.Version != right.Version ||
		left.SessionID != right.SessionID ||
		left.DealerShareID != right.DealerShareID ||
		len(left.Coefficients) != len(right.Coefficients) {
		return false
	}

	for index := range left.Coefficients {
		if left.Coefficients[index] != right.Coefficients[index] {
			return false
		}
	}

	return true
}

func rabbitVRFDKGEvaluationBytesV1(
	evaluation rabbitvrf.DKGPolynomialEvaluationV1,
) []byte {
	out := make([]byte, len(evaluation))
	copy(out, evaluation[:])
	return out
}

func (peer *rabbitVRFDKGPeer) sendEncryptedEvaluationV1(message lqc.RabbitVRFDKGEncryptedEvaluationV1) error {
	if peer == nil || peer.rw == nil {
		return fmt.Errorf("invalid rabbit vrf dkg encrypted evaluation transport")
	}
	peer.mu.Lock()
	defer peer.mu.Unlock()
	return p2p.Send(peer.rw, rabbitVRFDKGEncryptedEvaluationMsg, message)
}

func (n *rabbitVRFDKGTransport) sendEncryptedEvaluationToRecipientV1(message lqc.RabbitVRFDKGEncryptedEvaluationV1) error {
	if n == nil || message.SessionID == ([32]byte{}) || message.RecipientShareID == 0 {
		return fmt.Errorf("invalid rabbit vrf dkg encrypted evaluation route")
	}
	peer, ok := n.peerForShareV1(message.SessionID, message.RecipientShareID)
	if !ok {
		return fmt.Errorf("rabbit vrf dkg recipient peer unavailable for share %d", message.RecipientShareID)
	}
	return peer.sendEncryptedEvaluationV1(message)
}

func (runtime *rabbitVRFDKGRuntime) validateInboundEncryptedEvaluationV1(message lqc.RabbitVRFDKGEncryptedEvaluationV1) error {
	if runtime == nil {
		return fmt.Errorf("rabbit vrf dkg encrypted evaluation runtime unavailable")
	}
	context := runtime.currentContext()
	if context.SessionID == ([32]byte{}) || message.SessionID != context.SessionID {
		return errRabbitVRFDKGArtifactSessionMismatch
	}
	var recipient lqc.RabbitVRFCommitteeMemberV1
	recipientFound := false
	for _, member := range context.Members {
		if member.ShareID == message.RecipientShareID && member.Participant == message.RecipientParticipant {
			recipient = member
			recipientFound = true
			break
		}
	}
	_ = recipient
	if !recipientFound {
		return fmt.Errorf("rabbit vrf dkg encrypted evaluation recipient is not local")
	}
	var dealer lqc.RabbitVRFCommitteeMemberV1
	dealerFound := false
	for _, member := range context.CanonicalMembers {
		if member.ShareID == message.DealerShareID && member.Participant == message.DealerParticipant {
			dealer = member
			dealerFound = true
			break
		}
	}
	if !dealerFound {
		return fmt.Errorf("rabbit vrf dkg encrypted evaluation dealer not in canonical committee")
	}
	if err := lqc.VerifyRabbitVRFDKGEncryptedEvaluationSignatureV1(context.CanonicalSession, dealer, message); err != nil {
		return fmt.Errorf("verify rabbit vrf dkg encrypted evaluation signature: %w", err)
	}
	return nil
}

func (n *rabbitVRFDKGTransport) commitmentForDealerV1(sessionID [32]byte, dealerShareID uint64) (lqc.RabbitVRFDKGPolynomialCommitmentV1, bool) {
	var empty lqc.RabbitVRFDKGPolynomialCommitmentV1
	if n == nil || n.runtime == nil || sessionID == ([32]byte{}) || dealerShareID == 0 {
		return empty, false
	}
	context := n.runtime.currentContext()
	if context.SessionID != sessionID {
		return empty, false
	}
	for _, commitment := range context.PolynomialCommitments {
		if commitment.SessionID == sessionID && commitment.DealerShareID == dealerShareID {
			return commitment, true
		}
	}
	n.mu.RLock()
	defer n.mu.RUnlock()
	if n.closed || n.remoteSession != sessionID {
		return empty, false
	}
	packet, ok := n.remoteCommitments[dealerShareID]
	if !ok || packet.Commitment.SessionID != sessionID {
		return empty, false
	}
	return cloneRabbitVRFDKGPolynomialCommitmentPacketV1(packet).Commitment, true
}

func (runtime *rabbitVRFDKGRuntime) decryptInboundEncryptedEvaluationV1(message lqc.RabbitVRFDKGEncryptedEvaluationV1) (rabbitvrf.DKGPolynomialEvaluationV1, error) {
	var zero rabbitvrf.DKGPolynomialEvaluationV1
	if runtime == nil || runtime.backend == nil || runtime.backend.config == nil || runtime.backend.vrfDKGInstanceDir == "" || runtime.backend.vrfDKGTransport == nil {
		return zero, fmt.Errorf("rabbit vrf dkg encrypted evaluation runtime unavailable")
	}
	if !runtime.beginSecretOperationV1() {
		return zero, fmt.Errorf("rabbit vrf dkg secret operation unavailable")
	}
	defer runtime.endSecretOperationV1()
	context := runtime.currentContext()
	if context.SessionID == ([32]byte{}) || context.SessionID != message.SessionID {
		return zero, errRabbitVRFDKGArtifactSessionMismatch
	}
	var dealer lqc.RabbitVRFCommitteeMemberV1
	var recipient lqc.RabbitVRFCommitteeMemberV1
	dealerFound := false
	recipientFound := false
	for _, member := range context.CanonicalMembers {
		if member.ShareID == message.DealerShareID && member.Participant == message.DealerParticipant {
			dealer = member
			dealerFound = true
		}
	}
	for _, member := range context.Members {
		if member.ShareID == message.RecipientShareID && member.Participant == message.RecipientParticipant {
			recipient = member
			recipientFound = true
			break
		}
	}
	if !dealerFound || !recipientFound {
		return zero, fmt.Errorf("rabbit vrf dkg encrypted evaluation member mismatch")
	}
	commitment, ok := runtime.backend.vrfDKGTransport.commitmentForDealerV1(context.SessionID, dealer.ShareID)
	if !ok {
		return zero, fmt.Errorf("rabbit vrf dkg dealer commitment unavailable for share %d", dealer.ShareID)
	}
	var binding lqc.RabbitVRFDKGTransportKeyBindingV1
	var envelope lqc.RabbitVRFDKGEnvelopeV1
	artifactFound := false
	for index := range context.CanonicalTransportBindings {
		if context.CanonicalTransportBindings[index].ShareID == recipient.ShareID && context.CanonicalTransportBindings[index].Participant == recipient.Participant && index < len(context.CanonicalTransportEnvelopes) {
			binding = context.CanonicalTransportBindings[index]
			envelope = context.CanonicalTransportEnvelopes[index]
			artifactFound = true
			break
		}
	}
	if !artifactFound {
		return zero, fmt.Errorf("rabbit vrf dkg recipient transport artifact unavailable")
	}
	password, err := readRabbitVRFDKGPasswordFileV1(runtime.backend.config.RabbitVRFDKGPasswordFile)
	if err != nil {
		return zero, fmt.Errorf("read rabbit vrf dkg password file: %w", err)
	}
	store, err := rabbitvrfstate.NewStandardDKGTransportKeyStoreV1(filepath.Join(runtime.backend.vrfDKGInstanceDir, "rabbit-vrf", "dkg-transport"))
	if err != nil {
		return zero, fmt.Errorf("open rabbit vrf dkg transport store: %w", err)
	}
	privateKey, persistedBinding, err := store.Load(context.CanonicalSession, recipient, password)
	if err != nil {
		return zero, fmt.Errorf("load rabbit vrf dkg recipient transport key: %w", err)
	}
	if privateKey == nil {
		return zero, fmt.Errorf("nil rabbit vrf dkg recipient transport key")
	}
	defer zeroRabbitVRFDKGPrivateKeyV1(privateKey)
	if persistedBinding != binding {
		return zero, fmt.Errorf("rabbit vrf dkg recipient transport binding mismatch")
	}
	evaluation, err := lqc.DecryptRabbitVRFDKGEncryptedEvaluationV1(context.CanonicalSession, dealer, commitment, recipient, binding, envelope, privateKey, message)
	if err != nil {
		return zero, fmt.Errorf("decrypt rabbit vrf dkg encrypted evaluation: %w", err)
	}
	evaluationStore, err := rabbitvrfstate.NewStandardDKGVerifiedEvaluationStoreV1(filepath.Join(runtime.backend.vrfDKGInstanceDir, "rabbit-vrf", "dkg-evaluations"))
	if err != nil {
		return zero, fmt.Errorf("open rabbit vrf dkg verified evaluation store: %w", err)
	}
	if err := evaluationStore.Store(context.CanonicalSession, recipient, dealer, evaluation, password); err != nil {
		return zero, fmt.Errorf("persist rabbit vrf dkg verified evaluation: %w", err)
	}
	return evaluation, nil
}

func (runtime *rabbitVRFDKGRuntime) buildLocalSecretShareV1(recipient lqc.RabbitVRFCommitteeMemberV1) (*rabbitvrf.SecretShare, rabbitvrf.VerificationShare, error) {
	var zero rabbitvrf.VerificationShare
	if runtime == nil || runtime.backend == nil || runtime.backend.config == nil || runtime.backend.vrfDKGInstanceDir == "" || recipient.ShareID == 0 {
		return nil, zero, fmt.Errorf("rabbit vrf dkg secret share runtime unavailable")
	}
	if !runtime.beginSecretOperationV1() {
		return nil, zero, fmt.Errorf("rabbit vrf dkg secret operation unavailable")
	}
	defer runtime.endSecretOperationV1()
	context := runtime.currentContext()
	if context.SessionID == ([32]byte{}) || len(context.CanonicalMembers) == 0 {
		return nil, zero, fmt.Errorf("rabbit vrf dkg canonical members unavailable")
	}
	local := false
	for _, member := range context.Members {
		if member.ShareID == recipient.ShareID && member.Participant == recipient.Participant {
			local = true
			break
		}
	}
	if !local {
		return nil, zero, fmt.Errorf("rabbit vrf dkg secret share recipient is not local")
	}
	password, err := readRabbitVRFDKGPasswordFileV1(runtime.backend.config.RabbitVRFDKGPasswordFile)
	if err != nil {
		return nil, zero, fmt.Errorf("read rabbit vrf dkg password file: %w", err)
	}
	store, err := rabbitvrfstate.NewStandardDKGVerifiedEvaluationStoreV1(filepath.Join(runtime.backend.vrfDKGInstanceDir, "rabbit-vrf", "dkg-evaluations"))
	if err != nil {
		return nil, zero, fmt.Errorf("open rabbit vrf dkg verified evaluation store: %w", err)
	}
	evaluations := make([]rabbitvrf.DKGPolynomialEvaluationV1, 0, len(context.CanonicalMembers))
	for _, dealer := range context.CanonicalMembers {
		evaluation, err := store.Load(context.CanonicalSession, recipient, dealer, password)
		if err != nil {
			return nil, zero, fmt.Errorf("load rabbit vrf dkg evaluation dealer share %d for recipient share %d: %w", dealer.ShareID, recipient.ShareID, err)
		}
		evaluations = append(evaluations, evaluation)
	}
	share, err := rabbitvrf.AggregateDKGPolynomialEvaluationsV1(recipient.ShareID, evaluations)
	if err != nil {
		return nil, zero, fmt.Errorf("aggregate rabbit vrf dkg secret share %d: %w", recipient.ShareID, err)
	}
	verification, err := share.VerificationShare()
	if err != nil {
		return nil, zero, fmt.Errorf("derive rabbit vrf dkg verification share %d: %w", recipient.ShareID, err)
	}
	shareStore, err := rabbitvrfstate.NewStandardDKGSecretShareStoreV1(filepath.Join(runtime.backend.vrfDKGInstanceDir, "rabbit-vrf", "dkg-secret-shares"))
	if err != nil {
		return nil, zero, fmt.Errorf("open rabbit vrf dkg secret share store: %w", err)
	}
	if err := shareStore.Store(context.CanonicalSession, recipient, share, password); err != nil {
		return nil, zero, fmt.Errorf("persist rabbit vrf dkg secret share %d: %w", recipient.ShareID, err)
	}
	return share, verification, nil
}

func (runtime *rabbitVRFDKGRuntime) validateInboundThresholdPartialV1(packet lqc.RabbitVRFThresholdPartialV1) error {
	if runtime == nil || runtime.backend == nil || runtime.backend.vrfDKGInstanceDir == "" {
		return fmt.Errorf("rabbit vrf threshold partial runtime unavailable")
	}
	context := runtime.currentContext()
	if context.SessionID == (common.Hash{}) || packet.SessionID != context.SessionID {
		return errRabbitVRFDKGArtifactSessionMismatch
	}
	if packet.KeysetRoot == (common.Hash{}) || packet.RequestID == (common.Hash{}) || packet.MessageHash == (common.Hash{}) || packet.ShareID == 0 {
		return fmt.Errorf("invalid rabbit vrf threshold partial binding")
	}
	if _, err := runtime.canonicalPendingRequestV1(packet.RequestID); err != nil {
		return fmt.Errorf("validate rabbit vrf canonical pending request: %w", err)
	}
	keysetStore, err := rabbitvrfstate.NewDKGFinalKeysetStoreV1(filepath.Join(runtime.backend.vrfDKGInstanceDir, "rabbit-vrf", "dkg-final-keysets"))
	if err != nil {
		return fmt.Errorf("open rabbit vrf dkg final keyset store: %w", err)
	}
	keyset, err := keysetStore.Load(context.CanonicalSession)
	if err != nil {
		return fmt.Errorf("load rabbit vrf dkg final keyset: %w", err)
	}
	if packet.KeysetRoot != keyset.KeysetRoot {
		return fmt.Errorf("rabbit vrf threshold partial keyset mismatch")
	}
	message, messageHash, err := lqc.RabbitVRFThresholdMessageV1(context.CanonicalSession, keyset.KeysetRoot, packet.RequestID)
	if err != nil {
		return fmt.Errorf("derive rabbit vrf threshold message: %w", err)
	}
	if packet.MessageHash != messageHash {
		return fmt.Errorf("rabbit vrf threshold partial message mismatch")
	}
	var publicShare lqc.RabbitVRFVerificationShareV1
	found := false
	for _, candidate := range keyset.VerificationShares {
		if candidate.ShareID == packet.ShareID {
			publicShare = candidate
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("rabbit vrf threshold partial share %d is not in finalized keyset", packet.ShareID)
	}
	verificationShare, err := rabbitvrf.NewVerificationShare(packet.ShareID, publicShare.PublicKey)
	if err != nil {
		return fmt.Errorf("construct rabbit vrf threshold verification share %d: %w", packet.ShareID, err)
	}
	partial := rabbitvrf.PartialSignature{ShareID: packet.ShareID, Signature: packet.Signature}
	if _, err := rabbitvrf.VerifyPartial(verificationShare, message, partial); err != nil {
		return fmt.Errorf("verify rabbit vrf threshold partial share %d: %w", packet.ShareID, err)
	}
	if _, err := lqc.RabbitVRFThresholdPartialMessageIDV1(packet); err != nil {
		return fmt.Errorf("derive rabbit vrf threshold partial message id: %w", err)
	}
	if context.CanonicalTransportKeySetRoot == (common.Hash{}) ||
		uint64(len(context.CanonicalMembers)) != context.CanonicalSession.CommitteeSize ||
		len(context.CanonicalTransportBindings) != len(context.CanonicalMembers) ||
		packet.ShareID > uint64(len(context.CanonicalMembers)) {
		return fmt.Errorf("rabbit vrf canonical participation context unavailable")
	}
	index := packet.ShareID - 1
	member := context.CanonicalMembers[index]
	binding := context.CanonicalTransportBindings[index]
	if err := lqc.ValidateRabbitVRFThresholdPartialParticipationV1(
		context.CanonicalSession,
		context.CanonicalTransportKeySetRoot,
		member,
		binding,
		packet,
	); err != nil {
		return fmt.Errorf("verify rabbit vrf threshold participation share %d: %w", packet.ShareID, err)
	}
	return nil
}

func rabbitVRFSignThresholdPartialWithKeysetV1(share *rabbitvrf.SecretShare, recipientShareID uint64, keyset rabbitvrfstate.DKGFinalKeysetV1, message []byte) (rabbitvrf.PartialSignature, rabbitvrf.VerifiedPartialSignature, error) {
	var partialZero rabbitvrf.PartialSignature
	var verifiedZero rabbitvrf.VerifiedPartialSignature
	if share == nil || recipientShareID == 0 || len(message) == 0 {
		return partialZero, verifiedZero, fmt.Errorf("invalid rabbit vrf threshold signing state")
	}
	sharePublicKey, err := share.PublicKey()
	if err != nil {
		return partialZero, verifiedZero, fmt.Errorf("derive rabbit vrf local verification public key: %w", err)
	}
	var publicVerification lqc.RabbitVRFVerificationShareV1
	found := false
	for _, candidate := range keyset.VerificationShares {
		if candidate.ShareID == recipientShareID {
			publicVerification = candidate
			found = true
			break
		}
	}
	if !found || publicVerification.PublicKey != sharePublicKey {
		return partialZero, verifiedZero, fmt.Errorf("rabbit vrf local secret share does not match finalized keyset")
	}
	verificationShare, err := rabbitvrf.NewVerificationShare(recipientShareID, publicVerification.PublicKey)
	if err != nil {
		return partialZero, verifiedZero, fmt.Errorf("construct rabbit vrf verification share: %w", err)
	}
	partial, err := share.SignPartial(message)
	if err != nil {
		return partialZero, verifiedZero, fmt.Errorf("sign rabbit vrf threshold partial: %w", err)
	}
	verified, err := rabbitvrf.VerifyPartial(verificationShare, message, partial)
	if err != nil {
		return partialZero, verifiedZero, fmt.Errorf("verify rabbit vrf local threshold partial: %w", err)
	}
	return partial, verified, nil
}

func rabbitVRFCombineThresholdPartialsWithKeysetV1(threshold uint64, keyset rabbitvrfstate.DKGFinalKeysetV1, message []byte, partials []rabbitvrf.PartialSignature) (rabbitvrf.Signature, common.Hash, error) {
	var signatureZero rabbitvrf.Signature
	if threshold == 0 || threshold > uint64(len(keyset.VerificationShares)) || len(message) == 0 || len(partials) == 0 {
		return signatureZero, common.Hash{}, fmt.Errorf("invalid rabbit vrf threshold aggregation state")
	}
	verificationByShareID := make(map[uint64]rabbitvrf.VerificationShare, len(keyset.VerificationShares))
	for _, publicShare := range keyset.VerificationShares {
		verificationShare, err := rabbitvrf.NewVerificationShare(publicShare.ShareID, publicShare.PublicKey)
		if err != nil {
			return signatureZero, common.Hash{}, fmt.Errorf("construct rabbit vrf verification share %d: %w", publicShare.ShareID, err)
		}
		verificationByShareID[publicShare.ShareID] = verificationShare
	}
	verifiedPartials := make([]rabbitvrf.VerifiedPartialSignature, 0, len(partials))
	for _, partial := range partials {
		verificationShare, ok := verificationByShareID[partial.ShareID]
		if !ok {
			return signatureZero, common.Hash{}, fmt.Errorf("rabbit vrf partial share %d is not in finalized keyset", partial.ShareID)
		}
		verified, err := rabbitvrf.VerifyPartial(verificationShare, message, partial)
		if err != nil {
			return signatureZero, common.Hash{}, fmt.Errorf("verify rabbit vrf partial share %d: %w", partial.ShareID, err)
		}
		verifiedPartials = append(verifiedPartials, verified)
	}
	signature, err := rabbitvrf.CombineVerifiedPartials(keyset.ThresholdPublicKey, message, verifiedPartials, int(threshold))
	if err != nil {
		return signatureZero, common.Hash{}, fmt.Errorf("combine rabbit vrf threshold partials: %w", err)
	}
	randomness, err := rabbitvrf.VerifyAndDeriveRandomness(keyset.ThresholdPublicKey, message, signature)
	if err != nil {
		return signatureZero, common.Hash{}, fmt.Errorf("verify rabbit vrf threshold signature: %w", err)
	}
	return signature, randomness, nil
}

func (runtime *rabbitVRFDKGRuntime) signLocalThresholdPartialV1(recipient lqc.RabbitVRFCommitteeMemberV1, message []byte) (rabbitvrf.PartialSignature, rabbitvrf.VerifiedPartialSignature, error) {
	var partialZero rabbitvrf.PartialSignature
	var verifiedZero rabbitvrf.VerifiedPartialSignature
	if runtime == nil || runtime.backend == nil || runtime.backend.config == nil || runtime.backend.vrfDKGInstanceDir == "" || recipient.ShareID == 0 || len(message) == 0 {
		return partialZero, verifiedZero, fmt.Errorf("rabbit vrf threshold signing runtime unavailable")
	}
	if !runtime.beginSecretOperationV1() {
		return partialZero, verifiedZero, fmt.Errorf("rabbit vrf threshold secret operation unavailable")
	}
	defer runtime.endSecretOperationV1()

	context := runtime.currentContext()
	if context.SessionID == ([32]byte{}) {
		return partialZero, verifiedZero, fmt.Errorf("rabbit vrf canonical dkg session unavailable")
	}

	local := false
	for _, member := range context.Members {
		if member.ShareID == recipient.ShareID && member.Participant == recipient.Participant {
			local = true
			break
		}
	}
	if !local {
		return partialZero, verifiedZero, fmt.Errorf("rabbit vrf threshold signer is not local")
	}

	password, err := readRabbitVRFDKGPasswordFileV1(runtime.backend.config.RabbitVRFDKGPasswordFile)
	if err != nil {
		return partialZero, verifiedZero, fmt.Errorf("read rabbit vrf dkg password file: %w", err)
	}

	shareStore, err := rabbitvrfstate.NewStandardDKGSecretShareStoreV1(filepath.Join(runtime.backend.vrfDKGInstanceDir, "rabbit-vrf", "dkg-secret-shares"))
	if err != nil {
		return partialZero, verifiedZero, fmt.Errorf("open rabbit vrf dkg secret share store: %w", err)
	}
	share, err := shareStore.Load(context.CanonicalSession, recipient, password)
	if err != nil {
		return partialZero, verifiedZero, fmt.Errorf("load rabbit vrf dkg secret share %d: %w", recipient.ShareID, err)
	}

	keysetStore, err := rabbitvrfstate.NewDKGFinalKeysetStoreV1(filepath.Join(runtime.backend.vrfDKGInstanceDir, "rabbit-vrf", "dkg-final-keysets"))
	if err != nil {
		return partialZero, verifiedZero, fmt.Errorf("open rabbit vrf dkg final keyset store: %w", err)
	}
	keyset, err := keysetStore.Load(context.CanonicalSession)
	if err != nil {
		return partialZero, verifiedZero, fmt.Errorf("load rabbit vrf dkg final keyset: %w", err)
	}

	return rabbitVRFSignThresholdPartialWithKeysetV1(share, recipient.ShareID, keyset, message)
}

func (runtime *rabbitVRFDKGRuntime) signLocalThresholdParticipationV1(
	recipient lqc.RabbitVRFCommitteeMemberV1,
	packet lqc.RabbitVRFThresholdPartialV1,
) (lqc.RabbitVRFParticipationSignatureV1, error) {
	var zero lqc.RabbitVRFParticipationSignatureV1
	if runtime == nil ||
		runtime.backend == nil ||
		runtime.backend.config == nil ||
		runtime.backend.vrfDKGInstanceDir == "" ||
		recipient.ShareID == 0 ||
		packet.ShareID != recipient.ShareID {
		return zero, fmt.Errorf("rabbit vrf participation signing runtime unavailable")
	}
	if !runtime.beginSecretOperationV1() {
		return zero, fmt.Errorf("rabbit vrf participation secret operation unavailable")
	}
	defer runtime.endSecretOperationV1()

	context := runtime.currentContext()
	if context.SessionID == (common.Hash{}) ||
		packet.SessionID != context.SessionID ||
		context.CanonicalTransportKeySetRoot == (common.Hash{}) ||
		uint64(len(context.CanonicalMembers)) != context.CanonicalSession.CommitteeSize ||
		len(context.CanonicalTransportBindings) != len(context.CanonicalMembers) ||
		recipient.ShareID > uint64(len(context.CanonicalMembers)) {
		return zero, fmt.Errorf("rabbit vrf canonical participation context unavailable")
	}

	index := recipient.ShareID - 1
	member := context.CanonicalMembers[index]
	binding := context.CanonicalTransportBindings[index]
	if member != recipient ||
		binding.ShareID != recipient.ShareID ||
		binding.Participant != recipient.Participant {
		return zero, fmt.Errorf("rabbit vrf canonical participation member mismatch")
	}

	partialMessageID, err := lqc.RabbitVRFThresholdPartialMessageIDV1(packet)
	if err != nil {
		return zero, fmt.Errorf("derive rabbit vrf threshold partial message id: %w", err)
	}
	signingHash, err := lqc.RabbitVRFParticipationSigningHashV1(
		context.CanonicalSession,
		context.CanonicalTransportKeySetRoot,
		packet.KeysetRoot,
		packet.RequestID,
		packet.MessageHash,
		partialMessageID,
		member,
	)
	if err != nil {
		return zero, fmt.Errorf("derive rabbit vrf participation signing hash: %w", err)
	}

	password, err := readRabbitVRFDKGPasswordFileV1(runtime.backend.config.RabbitVRFDKGPasswordFile)
	if err != nil {
		return zero, fmt.Errorf("read rabbit vrf dkg password file: %w", err)
	}
	store, err := rabbitvrfstate.NewStandardDKGTransportKeyStoreV1(
		filepath.Join(runtime.backend.vrfDKGInstanceDir, "rabbit-vrf", "dkg-transport"),
	)
	if err != nil {
		return zero, fmt.Errorf("open rabbit vrf dkg transport store: %w", err)
	}
	privateKey, persistedBinding, err := store.Load(context.CanonicalSession, member, password)
	if err != nil {
		return zero, fmt.Errorf("load rabbit vrf participation transport key: %w", err)
	}
	if privateKey == nil {
		return zero, fmt.Errorf("nil rabbit vrf participation transport key")
	}
	defer zeroRabbitVRFDKGPrivateKeyV1(privateKey)
	if persistedBinding != binding {
		return zero, fmt.Errorf("rabbit vrf participation transport binding mismatch")
	}

	signature, err := gethcrypto.Sign(signingHash[:], privateKey)
	if err != nil {
		return zero, fmt.Errorf("sign rabbit vrf participation receipt: %w", err)
	}
	if len(signature) != gethcrypto.SignatureLength {
		return zero, fmt.Errorf("invalid rabbit vrf participation signature size")
	}
	copy(zero[:], signature[:lqc.RabbitVRFParticipationSignatureSizeV1])

	verifiedPacket := packet
	verifiedPacket.ParticipationSignature = zero
	if err := lqc.ValidateRabbitVRFThresholdPartialParticipationV1(
		context.CanonicalSession,
		context.CanonicalTransportKeySetRoot,
		member,
		binding,
		verifiedPacket,
	); err != nil {
		return lqc.RabbitVRFParticipationSignatureV1{}, fmt.Errorf("verify local rabbit vrf participation receipt: %w", err)
	}
	return zero, nil
}

func (runtime *rabbitVRFDKGRuntime) combineThresholdPartialsV1(message []byte, partials []rabbitvrf.PartialSignature) (rabbitvrf.Signature, common.Hash, error) {
	var signatureZero rabbitvrf.Signature
	if runtime == nil || runtime.backend == nil || runtime.backend.vrfDKGInstanceDir == "" || len(message) == 0 || len(partials) == 0 {
		return signatureZero, common.Hash{}, fmt.Errorf("rabbit vrf threshold aggregation runtime unavailable")
	}
	if !runtime.beginSecretOperationV1() {
		return signatureZero, common.Hash{}, fmt.Errorf("rabbit vrf threshold aggregation unavailable")
	}
	defer runtime.endSecretOperationV1()

	context := runtime.currentContext()
	if context.SessionID == ([32]byte{}) || context.CanonicalSession.Threshold == 0 {
		return signatureZero, common.Hash{}, fmt.Errorf("rabbit vrf canonical dkg session unavailable")
	}

	keysetStore, err := rabbitvrfstate.NewDKGFinalKeysetStoreV1(filepath.Join(runtime.backend.vrfDKGInstanceDir, "rabbit-vrf", "dkg-final-keysets"))
	if err != nil {
		return signatureZero, common.Hash{}, fmt.Errorf("open rabbit vrf dkg final keyset store: %w", err)
	}
	keyset, err := keysetStore.Load(context.CanonicalSession)
	if err != nil {
		return signatureZero, common.Hash{}, fmt.Errorf("load rabbit vrf dkg final keyset: %w", err)
	}

	return rabbitVRFCombineThresholdPartialsWithKeysetV1(context.CanonicalSession.Threshold, keyset, message, partials)
}
