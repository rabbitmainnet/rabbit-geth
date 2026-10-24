package rabbitvrfstate

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
)

func TestThresholdPartialAtomicV1(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "partials")
	path := filepath.Join(dir, "packet.json")
	content := []byte("verified-public-partial\n")
	if err := atomicWriteThresholdPartialFileV1(path, content); err != nil {
		t.Fatal(err)
	}
	got, err := readPrivateFileV1(path)
	if err != nil || !bytes.Equal(got, content) {
		t.Fatal("persisted content mismatch", err)
	}
	if err := atomicWriteThresholdPartialFileV1(path, []byte("conflict")); err == nil {
		t.Fatal("existing packet overwritten")
	}
	got, err = os.ReadFile(path)
	if err != nil || !bytes.Equal(got, content) {
		t.Fatal("original packet changed", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatal("unexpected temporary files", err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatal("unsafe file permissions", err)
		}
	}
}

func TestThresholdPartialAtomicConcurrentV1(t *testing.T) {
	path := filepath.Join(t.TempDir(), "partials", "packet.json")
	var winners atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(value byte) {
			defer wg.Done()
			content := bytes.Repeat([]byte{value}, 4096)
			if atomicWriteThresholdPartialFileV1(path, content) == nil {
				winners.Add(1)
			}
		}(byte(i + 1))
	}
	wg.Wait()
	if winners.Load() != 1 {
		t.Fatalf("expected one winner, got %d", winners.Load())
	}
	got, err := os.ReadFile(path)
	if err != nil || len(got) != 4096 {
		t.Fatal("incomplete persisted packet", err)
	}
	if !bytes.Equal(got, bytes.Repeat(got[:1], len(got))) {
		t.Fatal("mixed concurrent writes")
	}
}
