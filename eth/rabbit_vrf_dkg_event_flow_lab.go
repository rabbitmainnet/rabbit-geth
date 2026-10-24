//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/core"
)

// DKG always reads CurrentHeader rather than processing each historical event.
// Drain the chain feed independently of expensive cryptography. One pending wake
// is sufficient; slow DKG work must never block canonical block insertion.
func rabbitVRFDKGCoalesceHeadsV1(in <-chan core.ChainHeadEvent, wake chan<- core.ChainHeadEvent, stop <-chan struct{}) {
	for {
		select {
		case <-stop:
			return
		case head, ok := <-in:
			if !ok {
				return
			}
			select {
			case wake <- head:
			default:
			}
		}
	}
}

func cloneRabbitVRFDKGCiphertextV1(message lqc.RabbitVRFDKGEncryptedEvaluationV1) lqc.RabbitVRFDKGEncryptedEvaluationV1 {
	message.Ciphertext = append([]byte(nil), message.Ciphertext...)
	message.Signature = append([]byte(nil), message.Signature...)
	return message
}

// Cache only already authenticated encrypted outbound messages, never plaintext
// evaluations, passwords or private polynomial coefficients.
func (runtime *rabbitVRFDKGRuntime) cacheDKGEvaluationV1(session common.Hash, message lqc.RabbitVRFDKGEncryptedEvaluationV1) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.current.SessionID != session || message.SessionID != session {
		return
	}
	if runtime.evalCiphertextSession != session {
		runtime.evalCiphertextSession = session
		runtime.evalCiphertexts = nil
	}
	if runtime.evalCiphertexts == nil {
		runtime.evalCiphertexts = make(map[[2]uint64]lqc.RabbitVRFDKGEncryptedEvaluationV1)
	}
	runtime.evalCiphertexts[[2]uint64{message.DealerShareID, message.RecipientShareID}] = cloneRabbitVRFDKGCiphertextV1(message)
}

func (runtime *rabbitVRFDKGRuntime) cachedDKGEvaluationV1(session common.Hash, dealer, recipient uint64) (lqc.RabbitVRFDKGEncryptedEvaluationV1, bool) {
	runtime.mu.RLock()
	defer runtime.mu.RUnlock()
	if runtime.current.SessionID != session || runtime.evalCiphertextSession != session {
		return lqc.RabbitVRFDKGEncryptedEvaluationV1{}, false
	}
	message, ok := runtime.evalCiphertexts[[2]uint64{dealer, recipient}]
	if !ok {
		return lqc.RabbitVRFDKGEncryptedEvaluationV1{}, false
	}
	return cloneRabbitVRFDKGCiphertextV1(message), true
}

func (runtime *rabbitVRFDKGRuntime) dkgAlreadyFinalizedV1(session common.Hash) bool {
	runtime.mu.RLock()
	defer runtime.mu.RUnlock()
	return session != (common.Hash{}) && runtime.current.SessionID == session && runtime.dkgFinalizedSession == session
}

// A finalized dealer still serves lagging recipients. Replay ciphertexts prepared
// before finalization without reopening encrypted secret stores on every head.
func (runtime *rabbitVRFDKGRuntime) replayCachedDKGEvaluationsV1(session common.Hash) {
	runtime.mu.RLock()
	if runtime.current.SessionID != session || runtime.evalCiphertextSession != session {
		runtime.mu.RUnlock()
		return
	}
	messages := make([]lqc.RabbitVRFDKGEncryptedEvaluationV1, 0, len(runtime.evalCiphertexts))
	for _, message := range runtime.evalCiphertexts {
		messages = append(messages, cloneRabbitVRFDKGCiphertextV1(message))
	}
	runtime.mu.RUnlock()
	for _, message := range messages {
		if !runtime.shouldDispatchEvaluationV1(session, message.DealerShareID, message.RecipientShareID) {
			continue
		}
		if runtime.currentContext().SessionID != session {
			return
		}
		if err := runtime.backend.vrfDKGTransport.sendEncryptedEvaluationToRecipientV1(message, true); err != nil {
			runtime.logDKGWaitV1(session, "evaluation-route", "dealer", message.DealerShareID, "recipient", message.RecipientShareID, "err", err)
			continue
		}
		runtime.markDispatchedEvaluationV1(session, message.DealerShareID, message.RecipientShareID)
	}
}
