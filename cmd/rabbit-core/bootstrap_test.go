package main

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
)

func testArchive(t *testing.T) []byte {
	t.Helper()
	var compressed bytes.Buffer
	zw, e := zstd.NewWriter(&compressed)
	if e != nil {
		t.Fatal(e)
	}
	tw := tar.NewWriter(zw)
	for _, name := range []string{"rabbit/", "rabbit/chaindata/", "rabbit/triedb/"} {
		if e := tw.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeDir, Mode: 0700}); e != nil {
			t.Fatal(e)
		}
	}
	for _, name := range []string{"rabbit/chaindata/CURRENT", "rabbit/triedb/CURRENT"} {
		b := []byte("sample database contents")
		if e := tw.WriteHeader(&tar.Header{Name: name, Size: int64(len(b)), Mode: 0600}); e != nil {
			t.Fatal(e)
		}
		if _, e := tw.Write(b); e != nil {
			t.Fatal(e)
		}
	}
	if e := tw.Close(); e != nil {
		t.Fatal(e)
	}
	if e := zw.Close(); e != nil {
		t.Fatal(e)
	}
	return compressed.Bytes()
}
func digest(data []byte) string { h := sha256.Sum256(data); return hex.EncodeToString(h[:]) }
func testServer(t *testing.T, data []byte) *httptest.Server {
	t.Helper()
	return httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if v := r.Header.Get("Range"); v != "" {
			var off int
			if _, e := fmt.Sscanf(v, "bytes=%d-", &off); e != nil || off >= len(data) {
				w.WriteHeader(416)
				return
			}
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", off, len(data)-1, len(data)))
			w.Header().Set("Content-Length", strconv.Itoa(len(data)-off))
			w.WriteHeader(206)
			_, _ = w.Write(data[off:])
			return
		}
		_, _ = w.Write(data)
	}))
}
func TestBootstrapNewInstallation(t *testing.T) {
	archive := testArchive(t)
	server := testServer(t, archive)
	defer server.Close()
	dir := t.TempDir()
	if e := bootstrapFrom(context.Background(), dir, server.URL, digest(archive), server.Client()); e != nil {
		t.Fatal(e)
	}
	for _, name := range []string{"chaindata", "triedb"} {
		if _, e := os.Stat(filepath.Join(dir, "rabbit", name, "CURRENT")); e != nil {
			t.Fatal(e)
		}
	}
}
func TestBootstrapResumesInterruptedDownload(t *testing.T) {
	archive := testArchive(t)
	server := testServer(t, archive)
	defer server.Close()
	dir := t.TempDir()
	sum := digest(archive)
	if e := os.WriteFile(filepath.Join(dir, ".rabbit-bootstrap-"+sum+".part"), archive[:len(archive)/2], 0600); e != nil {
		t.Fatal(e)
	}
	if e := bootstrapFrom(context.Background(), dir, server.URL, sum, server.Client()); e != nil {
		t.Fatal(e)
	}
}
func TestBootstrapRejectsChecksumMismatch(t *testing.T) {
	archive := testArchive(t)
	server := testServer(t, archive)
	defer server.Close()
	dir := t.TempDir()
	incorrect := strings.Repeat("0", 64)
	if e := bootstrapFrom(context.Background(), dir, server.URL, incorrect, server.Client()); e == nil {
		t.Fatal("accepted wrong checksum")
	}
	if _, e := os.Stat(filepath.Join(dir, "rabbit")); !os.IsNotExist(e) {
		t.Fatalf("installed corrupt chain: %v", e)
	}
}
func TestBootstrapDoesNotTouchExistingChain(t *testing.T) {
	dir := t.TempDir()
	chain := filepath.Join(dir, "rabbit", "chaindata")
	if e := os.MkdirAll(chain, 0700); e != nil {
		t.Fatal(e)
	}
	sentinel := filepath.Join(chain, "sentinel")
	if e := os.WriteFile(sentinel, []byte("preserve"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := bootstrapFrom(context.Background(), dir, "invalid-url", "bad-checksum", nil); e != nil {
		t.Fatal(e)
	}
	if b, e := os.ReadFile(sentinel); e != nil || string(b) != "preserve" {
		t.Fatal("existing chain modified")
	}
}
func TestBootstrapRejectsExistingRabbitWithoutChain(t *testing.T) {
	dir := t.TempDir()
	if e := os.Mkdir(filepath.Join(dir, "rabbit"), 0700); e != nil {
		t.Fatal(e)
	}
	sentinel := filepath.Join(dir, "rabbit", "keystore")
	if e := os.WriteFile(sentinel, []byte("wallet"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := bootstrapFrom(context.Background(), dir, "https://example.com/snapshot", strings.Repeat("0", 64), nil); e == nil {
		t.Fatal("overwrote existing rabbit directory")
	}
	if b, e := os.ReadFile(sentinel); e != nil || string(b) != "wallet" {
		t.Fatal("wallet changed")
	}
}
func TestBootstrapRejectsNonHTTPS(t *testing.T) {
	if e := bootstrapFrom(context.Background(), t.TempDir(), "http://example.com/snapshot", strings.Repeat("0", 64), nil); e == nil {
		t.Fatal("insecure URL accepted")
	}
}
