package eth

import (
	"fmt"
	"path/filepath"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/consensus/lqc"
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
