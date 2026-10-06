//go:build (linux || darwin || windows) && cgo && rabbit_randomx

package rabbitx

import (
	"errors"
	"sync"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestLightHasherEpochCacheAlternationAndEviction(t *testing.T) {
	a := common.HexToHash("0x01")
	b := common.HexToHash("0x02")
	c := common.HexToHash("0x03")
	input := []byte("rabbit-epoch-cache-equivalence-v1")
	// Separate fresh instances establish expected results before pooled use.
	expected := make(map[common.Hash]common.Hash)
	for _, key := range []common.Hash{a, b, c} {
		reference, err := NewLightHasher()
		if err != nil {
			t.Fatal(err)
		}
		result, err := reference.Hash(key, input)
		reference.Close()
		if err != nil {
			t.Fatal(err)
		}
		expected[key] = result
	}
	h, err := NewLightHasher()
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	check := func(key common.Hash) {
		t.Helper()
		got, err := h.Hash(key, input)
		if err != nil || got != expected[key] {
			t.Fatalf("key=%s got=%s expected=%s err=%v", key, got, expected[key], err)
		}
	}
	check(a)
	firstA := h.epochs[0]
	check(b)
	firstB := h.epochs[0]
	for i := 0; i < 4; i++ {
		check(a)
		check(b)
	}
	if h.epochs[0] != firstB || h.epochs[1] != firstA {
		t.Fatal("alternating two epochs rebuilt a retained cache")
	}
	check(a) // a is most recently used; b must be evicted next.
	check(c)
	if h.epochs[1] != firstA || h.epochs[0].key != c {
		t.Fatal("least recently used eviction lost the retained epoch")
	}
	if firstB.vm != nil || firstB.cache != nil {
		t.Fatal("evicted epoch still owns RandomX resources")
	}
	check(b) // reconstruct an evicted key and check the same hash again.
	h.Close()
	h.Close()
	for _, epoch := range h.epochs {
		if epoch != nil {
			t.Fatal("close retained an epoch")
		}
	}
	if _, err := h.Hash(a, input); !errors.Is(err, ErrClosed) {
		t.Fatalf("hash after close: %v", err)
	}
}

func TestLightHasherEpochCacheConcurrentAlternation(t *testing.T) {
	keys := []common.Hash{common.HexToHash("0x01"), common.HexToHash("0x02")}
	input := []byte("rabbit-concurrent-epoch-cache-v1")
	h, err := NewLightHasher()
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	want := make(map[common.Hash]common.Hash)
	for _, key := range keys {
		result, err := h.Hash(key, input)
		if err != nil {
			t.Fatal(err)
		}
		want[key] = result
	}
	retained := [2]*lightHasherEpoch{h.epochs[0], h.epochs[1]}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		key := keys[i%2]
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 3; j++ {
				got, err := h.Hash(key, input)
				if err != nil || got != want[key] {
					t.Errorf("concurrent hash key=%s got=%s err=%v", key, got, err)
					return
				}
			}
		}()
	}
	wg.Wait()
	for _, epoch := range h.epochs {
		if epoch != retained[0] && epoch != retained[1] {
			t.Fatal("concurrent alternating calls rebuilt a cache")
		}
	}
}
