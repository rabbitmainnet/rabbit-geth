//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	rabbitvrfstate "github.com/ethereum/go-ethereum/internal/rabbitvrfstate"
)

type rabbitVRFDKGEvaluationDispatchEntryV1 struct {
	packet         lqc.RabbitVRFDKGEncryptedEvaluationV1
	deliveredLocal bool
	peer           *rabbitVRFDKGPeer
	lastSent       time.Time
}

type rabbitVRFDKGEvaluationDispatchV1 struct {
	mu            sync.Mutex
	session       common.Hash
	transportRoot common.Hash
	entries       map[[2]uint64]*rabbitVRFDKGEvaluationDispatchEntryV1
}

func (runtime *rabbitVRFDKGRuntime) buildOutgoingEvaluationV1(
	context rabbitVRFDKGLocalContextV1,
	dealer lqc.RabbitVRFCommitteeMemberV1,
	commitment lqc.RabbitVRFDKGPolynomialCommitmentV1,
	recipient lqc.RabbitVRFCommitteeMemberV1,
	binding lqc.RabbitVRFDKGTransportKeyBindingV1,
	envelope lqc.RabbitVRFDKGEnvelopeV1,
) (lqc.RabbitVRFDKGEncryptedEvaluationV1, error) {
	var zero lqc.RabbitVRFDKGEncryptedEvaluationV1
	if !runtime.beginSecretOperationV1() {
		return zero, fmt.Errorf("rabbit vrf evaluation secret operation unavailable")
	}
	defer runtime.endSecretOperationV1()
	if runtime.currentContext().SessionID != context.SessionID {
		return zero, errRabbitVRFDKGArtifactSessionMismatch
	}
	password, err := readRabbitVRFDKGPasswordFileV1(
		runtime.backend.config.RabbitVRFDKGPasswordFile,
	)
	if err != nil {
		return zero, err
	}
	store, err := rabbitvrfstate.NewStandardDKGDealerPolynomialStoreV1(
		filepath.Join(runtime.backend.vrfDKGInstanceDir, "rabbit-vrf", "dkg-dealer-polynomial"),
	)
	if err != nil {
		return zero, err
	}
	packet, err := rabbitVRFDKGBuildEncryptedEvaluationV1(
		store, runtime.backend.accountManager.Wallets(),
		context.CanonicalSession, dealer, commitment,
		recipient, binding, envelope, password,
	)
	if err != nil {
		return zero, err
	}
	if runtime.currentContext().SessionID != context.SessionID {
		return zero, errRabbitVRFDKGArtifactSessionMismatch
	}
	return packet, nil
}

func (runtime *rabbitVRFDKGRuntime) dispatchLocalEvaluationsV1() error {
	if runtime == nil || runtime.backend == nil {
		return errRabbitVRFDKGRuntimeV1
	}
	if !runtime.secretReadyV1() {
		return nil
	}
	context := runtime.currentContext()
	if len(context.Members) == 0 {
		return nil
	}
	if context.CanonicalTransportKeySetRoot == (common.Hash{}) ||
		uint64(len(context.CanonicalMembers)) != context.CanonicalSession.CommitteeSize ||
		len(context.CanonicalTransportBindings) != len(context.CanonicalMembers) ||
		len(context.CanonicalTransportEnvelopes) != len(context.CanonicalMembers) {
		return nil
	}
	id, err := lqc.RabbitVRFDKGSessionIDV1(context.CanonicalSession)
	if err != nil || id != context.SessionID {
		return errRabbitVRFDKGArtifactSessionMismatch
	}
	if runtime.backend.config == nil || runtime.backend.accountManager == nil ||
		runtime.backend.vrfDKGInstanceDir == "" || runtime.backend.vrfDKGTransport == nil {
		return fmt.Errorf("rabbit vrf evaluation dispatch backend unavailable")
	}
	for i, member := range context.CanonicalMembers {
		binding := context.CanonicalTransportBindings[i]
		if member.ShareID != uint64(i+1) ||
			binding.ShareID != member.ShareID || binding.Participant != member.Participant {
			return fmt.Errorf("rabbit vrf evaluation canonical recipient mismatch")
		}
		if err := lqc.VerifyRabbitVRFDKGTransportKeyEnvelopeV1(
			context.CanonicalSession, member, binding,
			context.CanonicalTransportEnvelopes[i],
		); err != nil {
			return err
		}
	}

	state := &runtime.evaluationDispatch
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.session != context.SessionID ||
		state.transportRoot != context.CanonicalTransportKeySetRoot {
		state.session = context.SessionID
		state.transportRoot = context.CanonicalTransportKeySetRoot
		state.entries = make(map[[2]uint64]*rabbitVRFDKGEvaluationDispatchEntryV1)
	}
	transport := runtime.backend.vrfDKGTransport
	var firstErr error
	for _, dealer := range context.Members {
		if dealer.ShareID == 0 || dealer.ShareID > uint64(len(context.CanonicalMembers)) ||
			context.CanonicalMembers[dealer.ShareID-1] != dealer {
			return fmt.Errorf("rabbit vrf evaluation local dealer mismatch")
		}
		commitment, ok := transport.commitmentForDealerV1(context.SessionID, dealer.ShareID)
		if !ok {
			continue
		}
		for i, recipient := range context.CanonicalMembers {
			if !runtime.secretReadyV1() || runtime.currentContext().SessionID != context.SessionID {
				return errRabbitVRFDKGArtifactSessionMismatch
			}
			local := false
			for _, member := range context.Members {
				if member == recipient {
					local = true
					break
				}
			}
			key := [2]uint64{dealer.ShareID, recipient.ShareID}
			entry := state.entries[key]
			var peer *rabbitVRFDKGPeer
			if local {
				if entry != nil && entry.deliveredLocal {
					continue
				}
			} else {
				var found bool
				peer, found = transport.peerForShareV1(context.SessionID, recipient.ShareID)
				if !found {
					continue
				}
				if entry != nil && entry.peer == peer &&
					time.Since(entry.lastSent) < 30*time.Second {
					continue
				}
			}
			if entry == nil {
				packet, err := runtime.buildOutgoingEvaluationV1(
					context, dealer, commitment, recipient,
					context.CanonicalTransportBindings[i],
					context.CanonicalTransportEnvelopes[i],
				)
				if err != nil {
					if firstErr == nil {
						firstErr = err
					}
					continue
				}
				entry = &rabbitVRFDKGEvaluationDispatchEntryV1{packet: packet}
				state.entries[key] = entry
			}
			if !runtime.secretReadyV1() || runtime.currentContext().SessionID != context.SessionID {
				return errRabbitVRFDKGArtifactSessionMismatch
			}
			if local {
				err := runtime.validateInboundEncryptedEvaluationV1(entry.packet)
				if err == nil {
					_, err = runtime.decryptInboundEncryptedEvaluationV1(entry.packet)
				}
				if err != nil {
					if firstErr == nil {
						firstErr = err
					}
					continue
				}
				entry.deliveredLocal = true
			} else {
				if err := peer.sendEncryptedEvaluationV1(entry.packet); err != nil {
					if firstErr == nil {
						firstErr = err
					}
					continue
				}
				entry.peer = peer
				entry.lastSent = time.Now()
			}
		}
	}
	return firstErr
}
