package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRabbitNodeLogDamageDetectionOnlyReadsCurrentAttempt(t *testing.T) {
	dataDir := t.TempDir()
	logDir := filepath.Join(dataDir, "logs")
	if err := os.MkdirAll(logDir, 0700); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(logDir, "rabbit-node.log")
	if err := os.WriteFile(logPath, []byte("CRIT Failed to recover state err=\"unexpected state history\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	offset := rabbitNodeLogSize(dataDir)
	if err := os.WriteFile(logPath, append(mustReadFile(t, logPath), []byte("INFO clean new attempt\n")...), 0600); err != nil {
		t.Fatal(err)
	}
	if rabbitNodeLogHasRecoverableChainDamageSince(dataDir, offset) {
		t.Fatal("old corruption marker must not trigger a new recovery")
	}
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("CRIT Failed to recover state err=\"unexpected state history\"\n"); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if !rabbitNodeLogHasRecoverableChainDamageSince(dataDir, offset) {
		t.Fatal("current-attempt corruption marker was not detected")
	}
}

func TestResetRecoverableLocalChainStatePreservesWalletAndNodeIdentity(t *testing.T) {
	dataDir := t.TempDir()

	for _, dir := range []string{
		filepath.Join(dataDir, "rabbit", "chaindata"),
		filepath.Join(dataDir, "rabbit", "triedb"),
		filepath.Join(dataDir, "keystore"),
	} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}

	key := filepath.Join(dataDir, "keystore", "UTC--test-wallet")
	nodeKey := filepath.Join(dataDir, "rabbit", "nodekey")
	if err := os.WriteFile(key, []byte("encrypted-wallet"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(nodeKey, []byte("node-identity"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "rabbit", "chaindata", "broken"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "rabbit", "triedb", "broken"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := resetRecoverableLocalChainState(dataDir); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dataDir, "rabbit", "chaindata")); !os.IsNotExist(err) {
		t.Fatalf("chaindata still exists: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "rabbit", "triedb")); !os.IsNotExist(err) {
		t.Fatalf("triedb still exists: %v", err)
	}
	if got, err := os.ReadFile(key); err != nil || string(got) != "encrypted-wallet" {
		t.Fatalf("wallet was not preserved: got=%q err=%v", got, err)
	}
	if got, err := os.ReadFile(nodeKey); err != nil || string(got) != "node-identity" {
		t.Fatalf("node identity was not preserved: got=%q err=%v", got, err)
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
