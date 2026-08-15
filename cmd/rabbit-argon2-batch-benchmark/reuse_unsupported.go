//go:build !linux || !cgo

package main

import "fmt"

func reusableArgon2IDInto(worker uint32, input, salt []byte, memoryKiB uint32, output *[32]byte) error {
	return fmt.Errorf("Argon2 batch benchmark requires Linux, cgo and libargon2.so.1")
}

func reusableWorkspaceStats() (uint64, uint64) { return 0, 0 }
func resetReusableWorkspace()                  {}
