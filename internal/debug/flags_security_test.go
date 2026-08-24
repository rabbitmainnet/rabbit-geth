// Copyright 2026 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package debug

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSecureRotatingLogFile(t *testing.T) {
	first, err := secureRotatingLogFile()
	if err != nil {
		t.Fatalf("create first rotating log file: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(first) })

	second, err := secureRotatingLogFile()
	if err != nil {
		t.Fatalf("create second rotating log file: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(second) })

	if first == second {
		t.Fatalf("rotating log paths are not unique: %q", first)
	}
	if filepath.Base(first) == "geth-lumberjack.log" {
		t.Fatalf("rotating log path is predictable: %q", first)
	}
	info, err := os.Stat(first)
	if err != nil {
		t.Fatalf("stat rotating log file: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("rotating log permissions = %o, want 600", info.Mode().Perm())
	}
}
