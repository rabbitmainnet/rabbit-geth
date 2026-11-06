//go:build (linux || darwin || windows) && cgo && rabbit_randomx

package rabbitx

import (
	"fmt"
	"sync"

	"github.com/ethereum/go-ethereum/common"
)

var runtimeProfileCheck struct {
	sync.Once
	err error
}

// Check the linked implementation, not a build label or the source headers.
// RandomX's upstream 2 GiB profile also hashes in light mode, but produces
// different consensus entropy from Rabbit's published 1 GiB runtime.
func verifyRuntimeProfile() error {
	runtimeProfileCheck.Do(func() {
		var key common.Hash
		for i := range key {
			key[i] = byte(i)
		}
		// Bypass the constructor here to avoid recursively entering Once.
		h := &LightHasher{}
		defer h.Close()
		got, err := h.Hash(key, []byte("rabbit-cgo-bridge-v1"))
		if err != nil {
			runtimeProfileCheck.err = fmt.Errorf("Rabbit RandomX profile check: %w", err)
			return
		}
		want := common.HexToHash("0x1d899231c94479f8ffc9f95c46b01d3a541b9a426ba4db69fb00b4f2b2c491c1")
		if got != want {
			runtimeProfileCheck.err = fmt.Errorf("Rabbit RandomX profile mismatch: got %s, expected %s; rebuild RandomX with the pinned Rabbit 1 GiB profile", got, want)
		}
	})
	return runtimeProfileCheck.err
}
