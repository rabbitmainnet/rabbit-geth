//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"crypto/ecdsa"
	"errors"
	"path/filepath"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/crypto/rabbitvrf"
	"github.com/ethereum/go-ethereum/internal/rabbitvrfstate"
	"github.com/ethereum/go-ethereum/log"
)

// Clear both active and prepared material on synchronization/shutdown. Promotion
// transfers ownership only for an exact session ID; normal load still checks
// ciphertext and password fingerprints before returning any promoted material.
func (cache *rabbitVRFSigningUnlockCacheV1) selectSessionLockedV2(session common.Hash) {
	var promoted map[string]rabbitVRFSigningUnlockEntryV1
	if cache.preparedSession == session && session != (common.Hash{}) {
		promoted = cache.preparedEntries
		cache.preparedEntries = nil
		cache.preparedSession = common.Hash{}
		cache.preparedBindingRoot = common.Hash{}
	}
	cache.clearLocked()
	cache.session = session
	cache.entries = promoted
}

func (cache *rabbitVRFSigningUnlockCacheV1) preparedReadyV2(active, session, root common.Hash) bool {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	return active != (common.Hash{}) && cache.session == active &&
		cache.preparedSession == session && cache.preparedBindingRoot == root &&
		len(cache.preparedEntries) > 0
}

// Takes ownership even on error; callers must detach entries from their scratch
// cache before calling. No secret byte slice is shared with that scratch cache.
func (cache *rabbitVRFSigningUnlockCacheV1) installPreparedV2(active, session, root common.Hash, entries map[string]rabbitVRFSigningUnlockEntryV1) error {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if active == (common.Hash{}) || session == (common.Hash{}) || root == (common.Hash{}) ||
		session == active || cache.session != active || len(entries) == 0 || len(entries) > 256 {
		for _, entry := range entries {
			rabbitVRFClearSigningBytesV1(entry.material.secret)
		}
		return errors.New("prepared signing cache context changed")
	}
	for _, entry := range cache.preparedEntries {
		rabbitVRFClearSigningBytesV1(entry.material.secret)
	}
	cache.preparedSession = session
	cache.preparedBindingRoot = root
	cache.preparedEntries = entries
	return nil
}

// Existing signing call sites always use the active cache. The WithCache
// variants are also used to decrypt preparation material without signing it.
func (runtime *rabbitVRFDKGRuntime) unlockedThresholdShareV1(context lqc.RabbitVRFDKGSessionContextV1, member lqc.RabbitVRFCommitteeMemberV1) (*rabbitvrf.SecretShare, error) {
	return runtime.unlockedThresholdShareWithCacheV2(&runtime.signingUnlock, context, member)
}

func (runtime *rabbitVRFDKGRuntime) unlockedThresholdTransportV1(context lqc.RabbitVRFDKGSessionContextV1, member lqc.RabbitVRFCommitteeMemberV1) (*ecdsa.PrivateKey, lqc.RabbitVRFDKGTransportKeyBindingV1, error) {
	return runtime.unlockedThresholdTransportWithCacheV2(&runtime.signingUnlock, context, member)
}

// Runs on the existing DKG preparation goroutine, not the request scan lane.
// This method does not sign, send, count partials, or authorize a future session.
func (runtime *rabbitVRFDKGRuntime) warmPreparationSigningKeysV2() error {
	if runtime == nil || runtime.backend == nil || runtime.backend.config == nil ||
		runtime.backend.blockchain == nil || runtime.backend.vrfDKGInstanceDir == "" {
		return nil
	}
	if !runtime.beginSecretOperationV1() {
		return nil
	}
	defer runtime.endSecretOperationV1()

	active := runtime.thresholdContextV1()
	prepared := runtime.currentContext()
	if active.SessionID == (common.Hash{}) || prepared.SessionID == (common.Hash{}) ||
		prepared.SessionID == active.SessionID || len(prepared.Members) == 0 ||
		active.TargetVRFEpoch == 0 || active.TargetVRFEpoch == ^uint64(0) ||
		prepared.TargetVRFEpoch != active.TargetVRFEpoch+1 ||
		prepared.CanonicalSession.TargetVRFEpoch != prepared.TargetVRFEpoch ||
		prepared.CanonicalTransportKeySetRoot == (common.Hash{}) ||
		uint64(len(prepared.CanonicalMembers)) != prepared.CanonicalSession.CommitteeSize ||
		len(prepared.CanonicalTransportBindings) != len(prepared.CanonicalMembers) {
		return nil
	}
	sid, err := lqc.RabbitVRFDKGSessionIDV1(prepared.CanonicalSession)
	if err != nil || sid != prepared.SessionID {
		return errRabbitVRFDKGArtifactSessionMismatch
	}
	canonical := runtime.backend.blockchain.GetHeaderByNumber(prepared.HeadNumber)
	if canonical == nil || canonical.Hash() != prepared.HeadHash {
		return errRabbitVRFDKGArtifactSessionMismatch
	}
	if runtime.signingUnlock.preparedReadyV2(active.SessionID, sid, prepared.CanonicalTransportKeySetRoot) {
		return nil
	}
	store, err := rabbitvrfstate.NewDKGFinalKeysetStoreV1(filepath.Join(runtime.backend.vrfDKGInstanceDir, "rabbit-vrf", "dkg-final-keysets"))
	if err != nil {
		return err
	}
	if _, err := store.Load(prepared.CanonicalSession); err != nil {
		return err
	}

	started := time.Now()
	var scratch rabbitVRFSigningUnlockCacheV1
	defer scratch.clear()
	for _, member := range prepared.Members {
		if member.ShareID == 0 || member.ShareID > uint64(len(prepared.CanonicalMembers)) ||
			prepared.CanonicalMembers[member.ShareID-1] != member {
			return errors.New("prepared signing member mismatch")
		}
		share, err := runtime.unlockedThresholdShareWithCacheV2(&scratch, prepared.CanonicalSession, member)
		if err != nil {
			return err
		}
		share.Destroy()
		key, binding, err := runtime.unlockedThresholdTransportWithCacheV2(&scratch, prepared.CanonicalSession, member)
		if err != nil {
			return err
		}
		zeroRabbitVRFDKGPrivateKeyV1(key)
		if binding != prepared.CanonicalTransportBindings[member.ShareID-1] {
			return errors.New("prepared signing transport binding mismatch")
		}
	}
	current := runtime.currentContext()
	canonical = runtime.backend.blockchain.GetHeaderByNumber(prepared.HeadNumber)
	if runtime.thresholdContextV1().SessionID != active.SessionID ||
		current.SessionID != sid || current.CanonicalTransportKeySetRoot != prepared.CanonicalTransportKeySetRoot ||
		canonical == nil || canonical.Hash() != prepared.HeadHash {
		return errRabbitVRFDKGArtifactSessionMismatch
	}
	scratch.mu.Lock()
	entries := scratch.entries
	scratch.entries = nil
	scratch.mu.Unlock()
	if err := runtime.signingUnlock.installPreparedV2(active.SessionID, sid, prepared.CanonicalTransportKeySetRoot, entries); err != nil {
		return err
	}
	log.Info("Rabbit VRF next epoch signing material prepared", "session", sid, "targetEpoch", prepared.TargetVRFEpoch, "elapsed", time.Since(started))
	return nil
}
