//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/internal/rabbitvrfstate"
	"github.com/ethereum/go-ethereum/log"
)

// progressDKGV1 runs only for local committee members. It never invents a
// participant or a share: all material comes from authenticated canonical DKG
// commitments and the encrypted evaluation transport.
func (runtime *rabbitVRFDKGRuntime) progressDKGV1() error {
	if runtime == nil || runtime.backend == nil || runtime.backend.vrfDKGTransport == nil ||
		runtime.backend.config == nil || runtime.backend.accountManager == nil || runtime.backend.vrfDKGInstanceDir == "" {
		return nil
	}
	transport := runtime.backend.vrfDKGTransport
	context := runtime.currentContext()
	if context.SessionID == (common.Hash{}) || len(context.Members) == 0 {
		return nil
	}
	if runtime.dkgAlreadyFinalizedV1(context.SessionID) {
		runtime.replayCachedDKGEvaluationsV1(context.SessionID)
		return nil
	}
	if len(context.CanonicalTransportBindings) != len(context.CanonicalMembers) {
		if _, err := transport.persistCanonicalTransportKeySetV1(); err != nil {
			return err
		}
		context = runtime.currentContext()
		if len(context.CanonicalTransportBindings) != len(context.CanonicalMembers) {
			runtime.logDKGWaitV1(context.SessionID, "transport-bindings", "have", len(context.CanonicalTransportBindings), "need", len(context.CanonicalMembers))
			return nil
		}
	}
	// Gather *all* authenticated dealer commitments, not just local artifacts.
	commitments := make([]lqc.RabbitVRFDKGPolynomialCommitmentV1, 0, len(context.CanonicalMembers))
	for _, dealer := range context.CanonicalMembers {
		commitment, ok := transport.commitmentForDealerV1(context.SessionID, dealer.ShareID)
		if !ok {
			runtime.logDKGWaitV1(context.SessionID, "dealer-commitment", "dealer", dealer.ShareID)
			return nil
		}
		commitments = append(commitments, commitment)
	}
	root, publicKey, transcriptRoot, verificationShares, err :=
		lqc.RabbitVRFDKGFinalKeysetV1(context.CanonicalSession, commitments)
	if err != nil {
		return fmt.Errorf("derive canonical rabbit vrf keyset: %w", err)
	}
	password, err := readRabbitVRFDKGPasswordFileV1(runtime.backend.config.RabbitVRFDKGPasswordFile)
	if err != nil {
		return err
	}
	dealerStore, err := rabbitvrfstate.NewStandardDKGDealerPolynomialStoreV1(filepath.Join(runtime.backend.vrfDKGInstanceDir, "rabbit-vrf", "dkg-dealer-polynomial"))
	if err != nil {
		return err
	}
	evalStore, err := rabbitvrfstate.NewStandardDKGVerifiedEvaluationStoreV1(filepath.Join(runtime.backend.vrfDKGInstanceDir, "rabbit-vrf", "dkg-evaluations"))
	if err != nil {
		return err
	}
	wallets := runtime.backend.accountManager.Wallets()
	for _, dealer := range context.Members {
		commitment, ok := transport.commitmentForDealerV1(context.SessionID, dealer.ShareID)
		if !ok {
			return nil
		}
		for index, recipient := range context.CanonicalMembers {
			if runtime.currentContext().SessionID != context.SessionID {
				return nil
			}
			// A local recipient needs the evaluation persisted; a remote recipient
			// needs periodic replay in case its authenticated route was not ready.
			local := false
			for _, member := range context.Members {
				if member.ShareID == recipient.ShareID && member.Participant == recipient.Participant {
					local = true
					break
				}
			}
			if local {
				if _, err := evalStore.Load(context.CanonicalSession, recipient, dealer, password); err == nil {
					continue
				} else if !errors.Is(err, os.ErrNotExist) {
					return err
				}
			} else if !runtime.shouldDispatchEvaluationV1(context.SessionID, dealer.ShareID, recipient.ShareID) {
				continue
			}
			message, cached := runtime.cachedDKGEvaluationV1(context.SessionID, dealer.ShareID, recipient.ShareID)
			if !cached {
				message, err = rabbitVRFDKGBuildEncryptedEvaluationV1(
					dealerStore, wallets, context.CanonicalSession, dealer, commitment, recipient,
					context.CanonicalTransportBindings[index], context.CanonicalTransportEnvelopes[index], password)
				if err != nil {
					return err
				}
				if !local {
					runtime.cacheDKGEvaluationV1(context.SessionID, message)
				}
			}
			if local {
				if err := runtime.validateInboundEncryptedEvaluationV1(message); err != nil {
					return err
				}
				if _, err := runtime.decryptInboundEncryptedEvaluationV1(message); err != nil {
					return err
				}
			} else if err := transport.sendEncryptedEvaluationToRecipientV1(message); err == nil {
				runtime.markDispatchedEvaluationV1(context.SessionID, dealer.ShareID, recipient.ShareID)
			} else {
				runtime.logDKGWaitV1(context.SessionID, "evaluation-route", "dealer", dealer.ShareID, "recipient", recipient.ShareID, "err", err)
			}
		}
	}
	// Do not persist a public keyset unless our *real* decrypted shares have
	// been assembled and match the deterministic commitment-derived public key.
	for _, recipient := range context.Members {
		ready := true
		for _, dealer := range context.CanonicalMembers {
			if _, err := evalStore.Load(context.CanonicalSession, recipient, dealer, password); err != nil {
				if errors.Is(err, os.ErrNotExist) {
					runtime.logDKGWaitV1(context.SessionID, "dealer-evaluation", "dealer", dealer.ShareID, "recipient", recipient.ShareID)
					ready = false
					break
				}
				return err
			}
		}
		if !ready {
			return nil
		}
		share, verification, err := runtime.buildLocalSecretShareV1(recipient)
		if err != nil {
			return err
		}
		if share == nil {
			return fmt.Errorf("nil rabbit vrf dkg final share")
		}
		match := false
		for _, expected := range verificationShares {
			if expected.ShareID == recipient.ShareID && expected.PublicKey == verification.PublicKey() {
				match = true
				break
			}
		}
		if !match {
			return fmt.Errorf("rabbit vrf dkg local share does not match canonical commitments")
		}
	}
	store, err := rabbitvrfstate.NewDKGFinalKeysetStoreV1(filepath.Join(runtime.backend.vrfDKGInstanceDir, "rabbit-vrf", "dkg-final-keysets"))
	if err != nil {
		return err
	}
	if err := store.Store(context.CanonicalSession, rabbitvrfstate.DKGFinalKeysetV1{
		KeysetRoot: root, ThresholdPublicKey: publicKey, TranscriptRoot: transcriptRoot, VerificationShares: verificationShares,
	}); err != nil {
		return err
	}
	if runtime.markDKGFinalizedV1(context.SessionID) {
		log.Info("Rabbit VRF DKG local final keyset persisted", "session", context.SessionID, "root", root)
	}
	return nil
}

func (runtime *rabbitVRFDKGRuntime) shouldDispatchEvaluationV1(session common.Hash, dealer, recipient uint64) bool {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.current.SessionID != session {
		return false
	}
	if runtime.evalDispatchSession != session {
		runtime.evalDispatchSession = session
		runtime.evalLastSent = make(map[[2]uint64]time.Time)
	}
	return time.Since(runtime.evalLastSent[[2]uint64{dealer, recipient}]) >= 10*time.Second
}
func (runtime *rabbitVRFDKGRuntime) markDispatchedEvaluationV1(session common.Hash, dealer, recipient uint64) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.current.SessionID == session && runtime.evalDispatchSession == session {
		runtime.evalLastSent[[2]uint64{dealer, recipient}] = time.Now()
	}
}
func (runtime *rabbitVRFDKGRuntime) markDKGFinalizedV1(session common.Hash) bool {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.current.SessionID != session || runtime.dkgFinalizedSession == session {
		return false
	}
	runtime.dkgFinalizedSession = session
	return true
}

// Report a wait at most once per ten seconds per session. Never log secret material.
func (runtime *rabbitVRFDKGRuntime) logDKGWaitV1(session common.Hash, stage string, fields ...interface{}) {
	runtime.mu.Lock()
	if runtime.current.SessionID != session || (runtime.dkgWaitSession == session && time.Since(runtime.dkgWaitAt) < 10*time.Second) {
		runtime.mu.Unlock()
		return
	}
	runtime.dkgWaitSession = session
	runtime.dkgWaitAt = time.Now()
	runtime.mu.Unlock()
	args := []interface{}{"session", session, "stage", stage}
	args = append(args, fields...)
	log.Info("Rabbit VRF DKG progress waiting", args...)
}
