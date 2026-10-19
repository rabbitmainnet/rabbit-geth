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

func TestRecoveryPreservesExistingBlockchainState(t *testing.T) {
	dataDir := t.TempDir()

	chainDir := filepath.Join(dataDir, "rabbit", "chaindata")
	trieDir := filepath.Join(dataDir, "rabbit", "triedb")
	keyDir := filepath.Join(dataDir, "keystore")

	for _, dir := range []string{chainDir, trieDir, keyDir} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}

	chainFile := filepath.Join(chainDir, "existing-chain")
	trieFile := filepath.Join(trieDir, "existing-trie")
	keyFile := filepath.Join(keyDir, "UTC--wallet")

	if err := os.WriteFile(chainFile, []byte("chain"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(trieFile, []byte("trie"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, []byte("wallet"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := verifyRecoverableLocalChainState(dataDir); err != nil {
		t.Fatal(err)
	}

	for _, file := range []string{chainFile, trieFile, keyFile} {
		if _, err := os.Stat(file); err != nil {
			t.Fatalf("recovery removed %s: %v", file, err)
		}
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
