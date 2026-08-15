//go:build !linux || !cgo

package main

import "fmt"

func reusableArgon2IDInto(input, salt []byte, memoryKiB uint32, output *[32]byte) error {
	return fmt.Errorf("reusable Argon2 benchmark requires Linux, cgo and libargon2.so.1")
}

func reusableWorkspaceStats() (uint64, uint64) { return 0, 0 }
func resetReusableWorkspace()                  {}
