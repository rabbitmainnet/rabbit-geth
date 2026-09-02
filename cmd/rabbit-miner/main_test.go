package main

import (
	"math/big"
	"os"
	"path/filepath"
	"testing"
)

func TestFormatRAB(t *testing.T) {
	tests := []struct {
		wei  string
		want string
	}{
		{"0", "0"},
		{"1", "0.000000000000000001"},
		{"1000000000000000000", "1"},
		{"1250000000000000000", "1.25"},
	}
	for _, test := range tests {
		wei, ok := new(big.Int).SetString(test.wei, 10)
		if !ok {
			t.Fatal("invalid test value")
		}
		if got := formatRAB(wei); got != test.want {
			t.Fatalf("formatRAB(%s)=%s want=%s", test.wei, got, test.want)
		}
	}
}

func TestReadPasswordTrimsOnlyLineEndings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "password.txt")
	if err := os.WriteFile(path, []byte("a password with spaces\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := readPassword(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != "a password with spaces" {
		t.Fatalf("password=%q", got)
	}
}

func TestResolveKeyFileRequiresOneFile(t *testing.T) {
	dir := t.TempDir()
	if _, err := resolveKeyFile(dir); err == nil {
		t.Fatal("empty directory accepted")
	}
	path := filepath.Join(dir, "UTC--key")
	if err := os.WriteFile(path, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := resolveKeyFile(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != path {
		t.Fatalf("path=%q want=%q", got, path)
	}
	if err := os.WriteFile(filepath.Join(dir, "second"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveKeyFile(dir); err == nil {
		t.Fatal("multiple files accepted")
	}
}
