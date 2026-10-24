//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"bytes"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestRabbitVRFEpochPrewarmPromotionV2(t *testing.T) {
	dir := t.TempDir()
	path, password := filepath.Join(dir, "key"), filepath.Join(dir, "password")
	if err := os.WriteFile(path, []byte("ciphertext"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(password, []byte("password"), 0600); err != nil {
		t.Fatal(err)
	}
	fingerprint, info, err := rabbitVRFSigningFingerprintV1(path, password)
	if err != nil {
		t.Fatal(err)
	}
	active, future, root := common.HexToHash("0x01"), common.HexToHash("0x02"), common.HexToHash("0x03")
	var cache rabbitVRFSigningUnlockCacheV1
	cache.selectSession(active)
	old := bytes.Repeat([]byte{8}, 32)
	cache.entries = map[string]rabbitVRFSigningUnlockEntryV1{"old": {material: rabbitVRFUnlockedSigningMaterialV1{secret: old}}}
	next := bytes.Repeat([]byte{7}, 32)
	entries := map[string]rabbitVRFSigningUnlockEntryV1{"member": {
		material: rabbitVRFUnlockedSigningMaterialV1{secret: next}, ciphertext: fingerprint, passwordInfo: info,
	}}
	if err := cache.installPreparedV2(active, future, root, entries); err != nil {
		t.Fatal(err)
	}
	if cache.session != active || cache.entries["member"].material.secret != nil {
		t.Fatal("preparation became active prematurely")
	}
	if !cache.preparedReadyV2(active, future, root) || cache.preparedReadyV2(active, future, common.HexToHash("0x04")) {
		t.Fatal("prepared root isolation failed")
	}
	cache.selectSession(future)
	if !bytes.Equal(old, make([]byte, 32)) {
		t.Fatal("old active secret retained")
	}
	calls := 0
	loader := func() (rabbitVRFUnlockedSigningMaterialV1, error) {
		calls++
		return rabbitVRFUnlockedSigningMaterialV1{secret: bytes.Repeat([]byte{9}, 32)}, nil
	}
	result, err := cache.load(future, "member", path, password, loader)
	if err != nil || calls != 0 || !bytes.Equal(result.secret, next) {
		t.Fatal("promotion reloaded or corrupted material", err)
	}
	result.secret[0] = 99
	if next[0] != 7 {
		t.Fatal("caller mutated prepared entry")
	}
	rabbitVRFClearSigningBytesV1(result.secret)
	if err := os.WriteFile(path, []byte("changed ciphertext"), 0600); err != nil {
		t.Fatal(err)
	}
	result, err = cache.load(future, "member", path, password, loader)
	if err != nil || calls != 1 || !bytes.Equal(next, make([]byte, 32)) {
		t.Fatal("promoted fingerprint check bypassed", err)
	}
	rabbitVRFClearSigningBytesV1(result.secret)
	cache.clear()
}

func TestRabbitVRFEpochPrewarmClearAndWrongSessionV2(t *testing.T) {
	active, future, other, root := common.HexToHash("0x01"), common.HexToHash("0x02"), common.HexToHash("0x03"), common.HexToHash("0x04")
	for _, syncClear := range []bool{false, true} {
		var cache rabbitVRFSigningUnlockCacheV1
		cache.selectSession(active)
		secret := bytes.Repeat([]byte{7}, 32)
		if err := cache.installPreparedV2(active, future, root, map[string]rabbitVRFSigningUnlockEntryV1{
			"member": {material: rabbitVRFUnlockedSigningMaterialV1{secret: secret}},
		}); err != nil {
			t.Fatal(err)
		}
		if syncClear {
			cache.clear()
		} else {
			cache.selectSession(other)
		}
		if !bytes.Equal(secret, make([]byte, 32)) || cache.preparedEntries != nil || cache.preparedSession != (common.Hash{}) {
			t.Fatal("stale prepared secret retained")
		}
	}
	var cache rabbitVRFSigningUnlockCacheV1
	cache.selectSession(other)
	rejected := bytes.Repeat([]byte{6}, 32)
	if err := cache.installPreparedV2(active, future, root, map[string]rabbitVRFSigningUnlockEntryV1{
		"member": {material: rabbitVRFUnlockedSigningMaterialV1{secret: rejected}},
	}); err == nil || !bytes.Equal(rejected, make([]byte, 32)) {
		t.Fatal("stale preparation installed or leaked")
	}
}

func TestRabbitVRFEpochPrewarmSyncClearsBothV2(t *testing.T) {
	runtime := &rabbitVRFDKGRuntime{}
	active, future, root := common.HexToHash("0x01"), common.HexToHash("0x02"), common.HexToHash("0x03")
	runtime.signingUnlock.selectSession(active)
	secret := bytes.Repeat([]byte{7}, 32)
	if err := runtime.signingUnlock.installPreparedV2(active, future, root, map[string]rabbitVRFSigningUnlockEntryV1{
		"member": {material: rabbitVRFUnlockedSigningMaterialV1{secret: secret}},
	}); err != nil {
		t.Fatal(err)
	}
	runtime.setSyncingV1(true)
	if !bytes.Equal(secret, make([]byte, 32)) || runtime.signingUnlock.preparedEntries != nil {
		t.Fatal("sync retained prepared material")
	}
}

func TestRabbitVRFEpochPrewarmConcurrentClearV2(t *testing.T) {
	dir := t.TempDir()
	path, password := filepath.Join(dir, "key"), filepath.Join(dir, "password")
	if err := os.WriteFile(path, []byte("ciphertext"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(password, []byte("password"), 0600); err != nil {
		t.Fatal(err)
	}
	active, future, root := common.HexToHash("0x01"), common.HexToHash("0x02"), common.HexToHash("0x03")
	var cache rabbitVRFSigningUnlockCacheV1
	var wg sync.WaitGroup
	for worker := 0; worker < 3; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 20; i++ {
				result, err := cache.load(active, "member", path, password, func() (rabbitVRFUnlockedSigningMaterialV1, error) {
					return rabbitVRFUnlockedSigningMaterialV1{secret: bytes.Repeat([]byte{8}, 32)}, nil
				})
				if err != nil {
					t.Error(err)
					return
				}
				rabbitVRFClearSigningBytesV1(result.secret)
				_ = cache.installPreparedV2(active, future, root, map[string]rabbitVRFSigningUnlockEntryV1{
					"member": {material: rabbitVRFUnlockedSigningMaterialV1{secret: bytes.Repeat([]byte{7}, 32)}},
				})
				cache.clear()
			}
		}()
	}
	wg.Wait()
	cache.clear()
	if cache.preparedEntries != nil || cache.entries != nil {
		t.Fatal("clear retained entries")
	}
}
