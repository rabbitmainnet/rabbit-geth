//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/internal/rabbitvrfstate"
)

func (runtime *rabbitVRFDKGRuntime) polynomialArtifactsReadyV1(
	context lqc.RabbitVRFDKGSessionContextV1,
	sessionID common.Hash,
	members []lqc.RabbitVRFCommitteeMemberV1,
) bool {
	runtime.mu.RLock()
	defer runtime.mu.RUnlock()

	commitments := runtime.current.PolynomialCommitments
	envelopes := runtime.current.PolynomialEnvelopes

	if runtime.current.SessionID != sessionID ||
		len(members) == 0 ||
		len(commitments) != len(members) ||
		len(envelopes) != len(members) {
		return false
	}

	for index := range members {
		commitment := commitments[index]
		envelope := envelopes[index]

		if commitment.SessionID != sessionID ||
			commitment.DealerShareID != members[index].ShareID ||
			envelope.SessionID != sessionID ||
			envelope.SenderShareID != members[index].ShareID ||
			envelope.Participant != members[index].Participant ||
			envelope.MessageType != lqc.RabbitVRFDKGMessagePolynomialCommitmentV1 {
			return false
		}

		root, err :=
			lqc.RabbitVRFDKGPolynomialCommitmentPayloadHashV1(
				context,
				commitment,
			)
		if err != nil || envelope.PayloadHash != root {
			return false
		}

		if err := lqc.VerifyRabbitVRFDKGEnvelopeV1(
			context,
			members[index],
			envelope,
		); err != nil {
			return false
		}
	}

	return true
}

func (runtime *rabbitVRFDKGRuntime) setPolynomialArtifactsV1(
	sessionID common.Hash,
	commitments []lqc.RabbitVRFDKGPolynomialCommitmentV1,
	envelopes []lqc.RabbitVRFDKGEnvelopeV1,
) bool {
	if sessionID == (common.Hash{}) ||
		len(commitments) == 0 ||
		len(commitments) != len(envelopes) {
		return false
	}

	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	if runtime.current.SessionID != sessionID {
		return false
	}

	runtime.current.PolynomialCommitments =
		cloneRabbitVRFDKGPolynomialCommitmentsV1(commitments)
	runtime.current.PolynomialEnvelopes =
		cloneRabbitVRFDKGTransportEnvelopesV1(envelopes)

	return true
}

func rabbitVRFDKGLoadOrCreateDealerCommitmentV1(
	store *rabbitvrfstate.DKGDealerPolynomialStoreV1,
	context lqc.RabbitVRFDKGSessionContextV1,
	member lqc.RabbitVRFCommitteeMemberV1,
	password string,
) (
	lqc.RabbitVRFDKGPolynomialCommitmentV1,
	error,
) {
	var empty lqc.RabbitVRFDKGPolynomialCommitmentV1

	polynomial, commitment, _, err :=
		store.Load(
			context,
			member,
			password,
		)
	if err == nil {
		polynomial.Destroy()
		return commitment, nil
	}

	if !errors.Is(
		err,
		rabbitvrfstate.ErrDKGTransportKeyStoreMissingV1,
	) {
		return empty, err
	}

	polynomial, commitment, _, err =
		store.Create(
			context,
			member,
			password,
		)
	if err == nil {
		polynomial.Destroy()
		return commitment, nil
	}

	if !errors.Is(
		err,
		rabbitvrfstate.ErrDKGTransportKeyStoreAlreadyExistsV1,
	) {
		return empty, err
	}

	polynomial, commitment, _, err =
		store.Load(
			context,
			member,
			password,
		)
	if err != nil {
		return empty, err
	}

	polynomial.Destroy()
	return commitment, nil
}

func (runtime *rabbitVRFDKGRuntime) ensurePolynomialCommitmentsV1(
	bridge lqc.RabbitVRFDKGBridgeV1,
	members []lqc.RabbitVRFCommitteeMemberV1,
) error {
	if runtime == nil || runtime.backend == nil {
		return errRabbitVRFDKGRuntimeV1
	}
	if len(members) == 0 {
		return nil
	}

	if runtime.polynomialArtifactsReadyV1(
		bridge.Session,
		bridge.SessionID,
		members,
	) {
		return nil
	}

	if !runtime.beginSecretOperationV1() {
		return nil
	}
	defer runtime.endSecretOperationV1()

	if runtime.polynomialArtifactsReadyV1(
		bridge.Session,
		bridge.SessionID,
		members,
	) {
		return nil
	}

	if runtime.backend.config == nil ||
		runtime.backend.accountManager == nil ||
		runtime.backend.vrfDKGInstanceDir == "" {
		return errRabbitVRFDKGRuntimeV1
	}

	password, err :=
		readRabbitVRFDKGPasswordFileV1(
			runtime.backend.config.RabbitVRFDKGPasswordFile,
		)
	if err != nil {
		return fmt.Errorf(
			"read rabbit vrf dkg password file: %w",
			err,
		)
	}

	store, err :=
		rabbitvrfstate.NewStandardDKGDealerPolynomialStoreV1(
			filepath.Join(
				runtime.backend.vrfDKGInstanceDir,
				"rabbit-vrf",
				"dkg-dealer-polynomial",
			),
		)
	if err != nil {
		return fmt.Errorf(
			"open rabbit vrf dkg dealer polynomial store: %w",
			err,
		)
	}

	wallets := runtime.backend.accountManager.Wallets()

	commitments := make(
		[]lqc.RabbitVRFDKGPolynomialCommitmentV1,
		0,
		len(members),
	)
	envelopes := make(
		[]lqc.RabbitVRFDKGEnvelopeV1,
		0,
		len(members),
	)

	for _, member := range members {
		commitment, err :=
			rabbitVRFDKGLoadOrCreateDealerCommitmentV1(
				store,
				bridge.Session,
				member,
				password,
			)
		if err != nil {
			return fmt.Errorf(
				"prepare rabbit vrf dkg dealer polynomial share %d: %w",
				member.ShareID,
				err,
			)
		}

		envelope, err :=
			rabbitVRFDKGSignPolynomialCommitmentEnvelopeV1(
				wallets,
				bridge.Session,
				member,
				commitment,
			)
		if err != nil {
			return fmt.Errorf(
				"authenticate rabbit vrf dkg polynomial commitment share %d: %w",
				member.ShareID,
				err,
			)
		}

		commitments = append(commitments, commitment)
		envelopes = append(envelopes, envelope)
	}

	if !runtime.setPolynomialArtifactsV1(
		bridge.SessionID,
		commitments,
		envelopes,
	) {
		return errRabbitVRFDKGRuntimeV1
	}

	if runtime.backend.vrfDKGTransport != nil {
		runtime.backend.vrfDKGTransport.publishLocalPolynomialCommitmentsV1()
	}

	return nil
}

func (n *rabbitVRFDKGTransport) publishLocalPolynomialCommitmentsV1() {
	if n == nil || n.runtime == nil {
		return
	}

	context := n.runtime.currentContext()

	if context.SessionID == (common.Hash{}) ||
		len(context.PolynomialCommitments) == 0 ||
		len(context.PolynomialCommitments) != len(context.PolynomialEnvelopes) {
		return
	}

	n.mu.RLock()
	peers := make([]*rabbitVRFDKGPeer, 0, len(n.peers))
	for _, peer := range n.peers {
		peers = append(peers, peer)
	}
	n.mu.RUnlock()

	for index := range context.PolynomialCommitments {
		packet := rabbitVRFDKGPolynomialCommitmentPacketV1{
			Commitment: context.PolynomialCommitments[index],
			Envelope:   context.PolynomialEnvelopes[index],
		}

		if err := n.validatePolynomialCommitmentV1(packet); err != nil {
			continue
		}

		for _, peer := range peers {
			_ = peer.sendPolynomialCommitmentV1(
				cloneRabbitVRFDKGPolynomialCommitmentPacketV1(packet),
			)
		}
	}
}
