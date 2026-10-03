//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/ethereum/go-ethereum/consensus/lqc"
	rabbitvrfstate "github.com/ethereum/go-ethereum/internal/rabbitvrfstate"
)

func persistRabbitVRFDKGFinalKeysetV1(
	instanceDir string,
	session lqc.RabbitVRFDKGSessionContextV1,
	commitments []lqc.RabbitVRFDKGPolynomialCommitmentV1,
) (rabbitvrfstate.DKGFinalKeysetV1, error) {
	var zero rabbitvrfstate.DKGFinalKeysetV1
	if instanceDir == "" {
		return zero, fmt.Errorf("rabbit vrf dkg instance directory unavailable")
	}
	root, publicKey, transcript, shares, err :=
		lqc.RabbitVRFDKGFinalKeysetV1(session, commitments)
	if err != nil {
		return zero, fmt.Errorf("derive finalized rabbit vrf dkg keyset: %w", err)
	}
	value := rabbitvrfstate.DKGFinalKeysetV1{
		KeysetRoot:         root,
		ThresholdPublicKey: publicKey,
		TranscriptRoot:     transcript,
		VerificationShares: shares,
	}
	store, err := rabbitvrfstate.NewDKGFinalKeysetStoreV1(
		filepath.Join(instanceDir, "rabbit-vrf", "dkg-final-keysets"),
	)
	if err != nil {
		return zero, err
	}
	if err := store.Store(session, value); err != nil {
		return zero, fmt.Errorf("persist finalized rabbit vrf dkg keyset: %w", err)
	}
	return store.Load(session)
}

func (runtime *rabbitVRFDKGRuntime) ensureFinalKeysetV1() (bool, error) {
	if runtime == nil || runtime.backend == nil {
		return false, errRabbitVRFDKGRuntimeV1
	}
	if !runtime.secretReadyV1() {
		return false, nil
	}
	context := runtime.currentContext()
	if len(context.Members) == 0 {
		return false, nil
	}
	sessionID, err := lqc.RabbitVRFDKGSessionIDV1(context.CanonicalSession)
	if err != nil || sessionID != context.SessionID {
		return false, errRabbitVRFDKGArtifactSessionMismatch
	}
	if uint64(len(context.CanonicalMembers)) != context.CanonicalSession.CommitteeSize {
		return false, fmt.Errorf("rabbit vrf final keyset canonical committee unavailable")
	}
	if runtime.backend.vrfDKGInstanceDir == "" {
		return false, fmt.Errorf("rabbit vrf final keyset persistence unavailable")
	}
	store, err := rabbitvrfstate.NewDKGFinalKeysetStoreV1(
		filepath.Join(runtime.backend.vrfDKGInstanceDir, "rabbit-vrf", "dkg-final-keysets"),
	)
	if err != nil {
		return false, err
	}
	if _, err := store.Load(context.CanonicalSession); err == nil {
		return true, nil
	} else if !errors.Is(err, rabbitvrfstate.ErrDKGTransportKeyStoreMissingV1) {
		return false, err
	}
	transport := runtime.backend.vrfDKGTransport
	if transport == nil {
		return false, fmt.Errorf("rabbit vrf final keyset transport unavailable")
	}
	commitments := make([]lqc.RabbitVRFDKGPolynomialCommitmentV1, 0, len(context.CanonicalMembers))
	for index, member := range context.CanonicalMembers {
		if member.ShareID != uint64(index+1) {
			return false, fmt.Errorf("rabbit vrf final keyset committee ordering mismatch")
		}
		commitment, ok := transport.commitmentForDealerV1(context.SessionID, member.ShareID)
		if !ok {
			return false, nil
		}
		commitments = append(commitments, commitment)
	}
	if runtime.currentContext().SessionID != context.SessionID {
		return false, errRabbitVRFDKGArtifactSessionMismatch
	}
	if _, err := persistRabbitVRFDKGFinalKeysetV1(
		runtime.backend.vrfDKGInstanceDir, context.CanonicalSession, commitments,
	); err != nil {
		return false, err
	}
	return true, nil
}
