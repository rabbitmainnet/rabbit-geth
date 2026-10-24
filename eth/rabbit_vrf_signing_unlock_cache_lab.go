//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	gethcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/rabbitvrf"
	"github.com/ethereum/go-ethereum/internal/rabbitvrfstate"
)

// Local unlock state only. It never changes the encrypted stores or consensus.
// The cache owns its byte slices; callers receive copies, cleared after use.
type rabbitVRFUnlockedSigningMaterialV1 struct {
	secret  []byte
	binding lqc.RabbitVRFDKGTransportKeyBindingV1
}
type rabbitVRFSigningUnlockEntryV1 struct {
	material     rabbitVRFUnlockedSigningMaterialV1
	ciphertext   [32]byte
	passwordInfo os.FileInfo
}
type rabbitVRFSigningUnlockCacheV1 struct {
	mu                  sync.Mutex
	session             common.Hash
	entries             map[string]rabbitVRFSigningUnlockEntryV1
	preparedSession     common.Hash
	preparedBindingRoot common.Hash
	preparedEntries     map[string]rabbitVRFSigningUnlockEntryV1
}

func rabbitVRFClearSigningBytesV1(value []byte) {
	for i := range value {
		value[i] = 0
	}
}
func (cache *rabbitVRFSigningUnlockCacheV1) clearLocked() {
	for _, entry := range cache.entries {
		rabbitVRFClearSigningBytesV1(entry.material.secret)
	}
	for _, entry := range cache.preparedEntries {
		rabbitVRFClearSigningBytesV1(entry.material.secret)
	}
	cache.preparedEntries = nil
	cache.preparedSession = common.Hash{}
	cache.preparedBindingRoot = common.Hash{}
	cache.entries = nil
	cache.session = common.Hash{}
}
func (cache *rabbitVRFSigningUnlockCacheV1) selectSession(session common.Hash) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if cache.session != session {
		cache.selectSessionLockedV2(session)
	}
}

func (cache *rabbitVRFSigningUnlockCacheV1) clear() {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	cache.clearLocked()
}
func rabbitVRFSigningFileV1(path string) (os.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 1048576 {
		return nil, errors.New("invalid private signing file")
	}
	return info, nil
}
func rabbitVRFSameSigningFileV1(a, b os.FileInfo) bool {
	return a != nil && b != nil && os.SameFile(a, b) && a.Size() == b.Size() && a.ModTime() == b.ModTime() && a.Mode() == b.Mode()
}
func rabbitVRFSigningFingerprintV1(path, passwordPath string) ([32]byte, os.FileInfo, error) {
	var zero [32]byte
	if _, err := rabbitVRFSigningFileV1(path); err != nil {
		return zero, nil, err
	}
	passwordInfo, err := rabbitVRFSigningFileV1(passwordPath)
	if err != nil {
		return zero, nil, err
	}
	encoded, err := os.ReadFile(path)
	if err != nil {
		return zero, nil, err
	}
	return sha256.Sum256(encoded), passwordInfo, nil
}
func (cache *rabbitVRFSigningUnlockCacheV1) load(session common.Hash, key, path, passwordPath string, load func() (rabbitVRFUnlockedSigningMaterialV1, error)) (rabbitVRFUnlockedSigningMaterialV1, error) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	var zero rabbitVRFUnlockedSigningMaterialV1
	if session == (common.Hash{}) {
		cache.clearLocked()
		return zero, errors.New("empty signing session")
	}
	if cache.session != session {
		cache.selectSessionLockedV2(session)
	}
	fingerprint, passwordInfo, err := rabbitVRFSigningFingerprintV1(path, passwordPath)
	if err != nil {
		if entry, ok := cache.entries[key]; ok {
			rabbitVRFClearSigningBytesV1(entry.material.secret)
			delete(cache.entries, key)
		}
		return zero, err
	}
	if entry, ok := cache.entries[key]; ok {
		if entry.ciphertext == fingerprint && rabbitVRFSameSigningFileV1(entry.passwordInfo, passwordInfo) {
			value := entry.material
			value.secret = append([]byte(nil), value.secret...)
			return value, nil
		}
		rabbitVRFClearSigningBytesV1(entry.material.secret)
		delete(cache.entries, key)
	}
	value, err := load()
	if err != nil {
		rabbitVRFClearSigningBytesV1(value.secret)
		return zero, err
	}
	if len(value.secret) != 32 {
		rabbitVRFClearSigningBytesV1(value.secret)
		return zero, errors.New("invalid unlocked signing material")
	}
	// Do not retain material read across an atomic replacement or password edit.
	after, afterPassword, err := rabbitVRFSigningFingerprintV1(path, passwordPath)
	if err != nil || after != fingerprint || !rabbitVRFSameSigningFileV1(passwordInfo, afterPassword) {
		rabbitVRFClearSigningBytesV1(value.secret)
		return zero, errors.New("signing files changed while unlocking")
	}
	if len(cache.entries) >= 256 {
		cache.clearLocked()
		cache.session = session
	}
	if cache.entries == nil {
		cache.entries = make(map[string]rabbitVRFSigningUnlockEntryV1)
	}
	cache.entries[key] = rabbitVRFSigningUnlockEntryV1{material: value, ciphertext: fingerprint, passwordInfo: passwordInfo}
	value.secret = append([]byte(nil), value.secret...)
	return value, nil
}
func (runtime *rabbitVRFDKGRuntime) unlockedThresholdShareWithCacheV2(cache *rabbitVRFSigningUnlockCacheV1, context lqc.RabbitVRFDKGSessionContextV1, member lqc.RabbitVRFCommitteeMemberV1) (*rabbitvrf.SecretShare, error) {
	store, err := rabbitvrfstate.NewStandardDKGSecretShareStoreV1(filepath.Join(runtime.backend.vrfDKGInstanceDir, "rabbit-vrf", "dkg-secret-shares"))
	if err != nil {
		return nil, err
	}
	path, err := store.Path(context, member)
	if err != nil {
		return nil, err
	}
	session, err := lqc.RabbitVRFDKGSessionIDV1(context)
	if err != nil {
		return nil, err
	}
	passwordPath := runtime.backend.config.RabbitVRFDKGPasswordFile
	value, err := cache.load(session, fmt.Sprintf("bls:%d:%s", member.ShareID, member.Participant), path, passwordPath, func() (rabbitVRFUnlockedSigningMaterialV1, error) {
		var zero rabbitVRFUnlockedSigningMaterialV1
		password, err := readRabbitVRFDKGPasswordFileV1(passwordPath)
		if err != nil {
			return zero, err
		}
		share, err := store.Load(context, member, password)
		if err != nil {
			return zero, err
		}
		defer share.Destroy()
		encoded, err := share.Bytes()
		if err != nil {
			return zero, err
		}
		defer rabbitVRFClearSigningBytesV1(encoded[:])
		return rabbitVRFUnlockedSigningMaterialV1{secret: append([]byte(nil), encoded[:]...)}, nil
	})
	if err != nil {
		return nil, err
	}
	defer rabbitVRFClearSigningBytesV1(value.secret)
	return rabbitvrf.SecretShareFromBytes(member.ShareID, value.secret)
}
func (runtime *rabbitVRFDKGRuntime) unlockedThresholdTransportWithCacheV2(cache *rabbitVRFSigningUnlockCacheV1, context lqc.RabbitVRFDKGSessionContextV1, member lqc.RabbitVRFCommitteeMemberV1) (*ecdsa.PrivateKey, lqc.RabbitVRFDKGTransportKeyBindingV1, error) {
	var binding lqc.RabbitVRFDKGTransportKeyBindingV1
	store, err := rabbitvrfstate.NewStandardDKGTransportKeyStoreV1(filepath.Join(runtime.backend.vrfDKGInstanceDir, "rabbit-vrf", "dkg-transport"))
	if err != nil {
		return nil, binding, err
	}
	path, err := store.Path(context, member)
	if err != nil {
		return nil, binding, err
	}
	session, err := lqc.RabbitVRFDKGSessionIDV1(context)
	if err != nil {
		return nil, binding, err
	}
	passwordPath := runtime.backend.config.RabbitVRFDKGPasswordFile
	value, err := cache.load(session, fmt.Sprintf("transport:%d:%s", member.ShareID, member.Participant), path, passwordPath, func() (rabbitVRFUnlockedSigningMaterialV1, error) {
		var zero rabbitVRFUnlockedSigningMaterialV1
		password, err := readRabbitVRFDKGPasswordFileV1(passwordPath)
		if err != nil {
			return zero, err
		}
		key, persisted, err := store.Load(context, member, password)
		if err != nil {
			return zero, err
		}
		if key == nil {
			return zero, errors.New("nil signing transport key")
		}
		defer zeroRabbitVRFDKGPrivateKeyV1(key)
		return rabbitVRFUnlockedSigningMaterialV1{secret: gethcrypto.FromECDSA(key), binding: persisted}, nil
	})
	if err != nil {
		return nil, binding, err
	}
	defer rabbitVRFClearSigningBytesV1(value.secret)
	key, err := gethcrypto.ToECDSA(value.secret)
	return key, value.binding, err
}

// Called by the independent request lane even when no request is pending.
// This lane activates only the canonical ACTIVE session. A separate prepared
// cache may be promoted by exact session ID; preparation never signs.
func (runtime *rabbitVRFDKGRuntime) warmThresholdSigningKeysV1() error {
	if runtime == nil {
		return nil
	}
	if runtime.backend == nil || runtime.backend.config == nil || runtime.backend.vrfDKGInstanceDir == "" {
		runtime.signingUnlock.clear()
		return nil
	}
	if !runtime.beginSecretOperationV1() {
		runtime.signingUnlock.clear()
		return nil
	}
	defer runtime.endSecretOperationV1()
	context := runtime.thresholdContextV1()
	if context.SessionID == (common.Hash{}) || len(context.Members) == 0 {
		runtime.signingUnlock.clear()
		return nil
	}
	runtime.signingUnlock.selectSession(context.SessionID)
	store, err := rabbitvrfstate.NewDKGFinalKeysetStoreV1(filepath.Join(runtime.backend.vrfDKGInstanceDir, "rabbit-vrf", "dkg-final-keysets"))
	if err != nil {
		return err
	}
	if _, err = store.Load(context.CanonicalSession); err != nil {
		return err
	}
	for _, member := range context.Members {
		share, err := runtime.unlockedThresholdShareV1(context.CanonicalSession, member)
		if err != nil {
			return err
		}
		share.Destroy()
		key, binding, err := runtime.unlockedThresholdTransportV1(context.CanonicalSession, member)
		if err != nil {
			return err
		}
		zeroRabbitVRFDKGPrivateKeyV1(key)
		index := member.ShareID - 1
		if index >= uint64(len(context.CanonicalTransportBindings)) || binding != context.CanonicalTransportBindings[index] {
			runtime.signingUnlock.clear()
			return errors.New("warm signing binding mismatch")
		}
	}
	if runtime.thresholdContextV1().SessionID != context.SessionID {
		runtime.signingUnlock.clear()
		return errRabbitVRFDKGArtifactSessionMismatch
	}
	return nil
}
