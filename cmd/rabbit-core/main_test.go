package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestKeyAddress(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "UTC--test")
	if err := os.WriteFile(path, []byte(`{"address":"da5bf4a009e63d6db4effaf5a2d6910f4d5bd2a0"}`), 0600); err != nil {
		t.Fatal(err)
	}
	address, err := keyAddress(path)
	if err != nil {
		t.Fatal(err)
	}
	if address.Hex() != "0xdA5bf4A009e63D6dB4EfFaF5a2D6910f4D5BD2a0" {
		t.Fatalf("address=%s", address)
	}
}

func TestSessionPasswordFile(t *testing.T) {
	path, cleanup, err := sessionPasswordFile(t.TempDir(), "secret")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	encoded, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != "secret\n" {
		t.Fatalf("password file contents=%q", encoded)
	}
}

func TestConfiguredBootnodesPrefersFlag(t *testing.T) {
	t.Setenv("RABBIT_BOOTNODES", "enode://environment")
	got := configuredBootnodes(options{bootnodes: "enode://flag"})
	if got != "enode://flag" {
		t.Fatalf("bootnodes=%q", got)
	}
}
