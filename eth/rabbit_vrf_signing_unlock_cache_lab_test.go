//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestRabbitVRFSigningUnlockCacheLifecycleV1(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "encrypted")
	password := filepath.Join(dir, "password")
	if err := os.WriteFile(path, []byte("authenticated ciphertext"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(password, []byte("password"), 0600); err != nil {
		t.Fatal(err)
	}
	var cache rabbitVRFSigningUnlockCacheV1
	var loads atomic.Int32
	var owned []byte
	loader := func() (rabbitVRFUnlockedSigningMaterialV1, error) {
		loads.Add(1)
		owned = bytes.Repeat([]byte{7}, 32)
		return rabbitVRFUnlockedSigningMaterialV1{secret: owned}, nil
	}
	session := common.HexToHash("0x01")
	load := func() (rabbitVRFUnlockedSigningMaterialV1, error) {
		return cache.load(session, "member", path, password, loader)
	}
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, err := load()
			if err != nil {
				t.Error(err)
				return
			}
			if v.secret[0] != 7 {
				t.Error("wrong copied key")
			}
			v.secret[0] = 9
		}()
	}
	wg.Wait()
	if loads.Load() != 1 {
		t.Fatalf("concurrent signing unlocked %d times", loads.Load())
	}
	if owned[0] != 7 {
		t.Fatal("caller mutated cached signing key")
	}
	old := owned
	replacement := filepath.Join(dir, "replacement")
	if err := os.WriteFile(replacement, []byte("new password"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, password); err != nil {
		t.Fatal(err)
	}
	v, err := load()
	if err != nil {
		t.Fatal(err)
	}
	rabbitVRFClearSigningBytesV1(v.secret)
	if loads.Load() != 2 || !bytes.Equal(old, make([]byte, 32)) {
		t.Fatal("password rotation reused or retained old key")
	}
	old = owned
	if err := os.WriteFile(path, []byte("changed ciphertext"), 0600); err != nil {
		t.Fatal(err)
	}
	v, err = load()
	if err != nil {
		t.Fatal(err)
	}
	rabbitVRFClearSigningBytesV1(v.secret)
	if loads.Load() != 3 || !bytes.Equal(old, make([]byte, 32)) {
		t.Fatal("ciphertext change did not invalidate key")
	}
	old = owned
	session = common.HexToHash("0x02")
	v, err = load()
	if err != nil {
		t.Fatal(err)
	}
	rabbitVRFClearSigningBytesV1(v.secret)
	if loads.Load() != 4 || !bytes.Equal(old, make([]byte, 32)) {
		t.Fatal("session change retained stale key")
	}
	old = owned
	cache.clear()
	if !bytes.Equal(old, make([]byte, 32)) || cache.entries != nil || cache.session != (common.Hash{}) {
		t.Fatal("clear retained secret material")
	}
}
func TestRabbitVRFSigningUnlockCacheFailuresV1(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "encrypted")
	password := filepath.Join(dir, "password")
	for _, p := range []string{path, password} {
		if err := os.WriteFile(p, []byte("file"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	var cache rabbitVRFSigningUnlockCacheV1
	attempts := 0
	loader := func() (rabbitVRFUnlockedSigningMaterialV1, error) {
		attempts++
		return rabbitVRFUnlockedSigningMaterialV1{}, errors.New("authentication failed")
	}
	for i := 0; i < 2; i++ {
		if _, err := cache.load(common.HexToHash("0x01"), "key", path, password, loader); err == nil {
			t.Fatal("failed decrypt accepted")
		}
	}
	if attempts != 2 || len(cache.entries) != 0 {
		t.Fatal("authentication failure cached")
	}
	good := func() (rabbitVRFUnlockedSigningMaterialV1, error) {
		return rabbitVRFUnlockedSigningMaterialV1{secret: bytes.Repeat([]byte{1}, 32)}, nil
	}
	if _, err := cache.load(common.HexToHash("0x01"), "key", path, password, good); err != nil {
		t.Fatal(err)
	}
	owned := cache.entries["key"].material.secret
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.load(common.HexToHash("0x01"), "key", path, password, good); err == nil {
		t.Fatal("deleted key reused")
	}
	if !bytes.Equal(owned, make([]byte, 32)) {
		t.Fatal("deleted file key was retained")
	}
}

func TestRabbitVRFSigningUnlockSyncClearsV1(t *testing.T) {
	runtime := &rabbitVRFDKGRuntime{secretReady: true}
	owned := bytes.Repeat([]byte{9}, 32)
	runtime.signingUnlock.session = common.HexToHash("0x01")
	runtime.signingUnlock.entries = map[string]rabbitVRFSigningUnlockEntryV1{"test": {material: rabbitVRFUnlockedSigningMaterialV1{secret: owned}}}
	runtime.setSyncingV1(true)
	if !bytes.Equal(owned, make([]byte, 32)) || len(runtime.signingUnlock.entries) != 0 {
		t.Fatal("sync retained signing keys")
	}
	if runtime.beginSecretOperationV1() {
		runtime.endSecretOperationV1()
		t.Fatal("sync allowed signing")
	}
}
