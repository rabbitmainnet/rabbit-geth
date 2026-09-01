//go:build (linux || darwin || windows) && cgo && rabbit_randomx

package rabbitx

import (
	"encoding/hex"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestRandomXLightHasherMatchesIndependentCVector(t *testing.T) {
	var key common.Hash
	for i := range key {
		key[i] = byte(i)
	}

	h, err := NewLightHasher()
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	got, err := h.Hash(key, []byte("rabbit-cgo-bridge-v1"))
	if err != nil {
		t.Fatal(err)
	}

	const want = "1d899231c94479f8ffc9f95c46b01d3a541b9a426ba4db69fb00b4f2b2c491c1"
	if hex.EncodeToString(got[:]) != want {
		t.Fatalf("hash=%x want=%s", got, want)
	}

	again, err := h.Hash(key, []byte("rabbit-cgo-bridge-v1"))
	if err != nil {
		t.Fatal(err)
	}
	if again != got {
		t.Fatal("same key/input produced different RandomX hash")
	}
}

func TestRandomXFullHasherMatchesLightHasher(t *testing.T) {
	var key common.Hash
	for i := range key {
		key[i] = byte(i)
	}
	input := []byte("rabbit-full-memory-equivalence-v1")

	light, err := NewLightHasher()
	if err != nil {
		t.Fatal(err)
	}
	defer light.Close()
	want, err := light.Hash(key, input)
	if err != nil {
		t.Fatal(err)
	}

	full, err := NewFullHasher()
	if err != nil {
		t.Fatal(err)
	}
	defer full.Close()
	got, err := full.Hash(key, input)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("full-memory hash=%x light hash=%x", got, want)
	}
}
