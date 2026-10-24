package rabbitvrf

import (
	bls12381 "github.com/consensys/gnark-crypto/ecc/bls12-381"
	"sync"
	"testing"
)

func TestPublicKeyDecodeMemoV1(t *testing.T) {
	m := publicKeyDecodeMemoV1{capacity: 4}
	var first PublicKey
	for i := byte(1); i <= 5; i++ {
		key, err := fixedSecretKey(t, i).PublicKey()
		if err != nil {
			t.Fatal(err)
		}
		if i == 1 {
			first = key
		}
		got, err := m.decode(key)
		if err != nil {
			t.Fatal(err)
		}
		want, err := decodePublicKeyUncachedV1(key)
		if err != nil || got != want {
			t.Fatal("decoder mismatch")
		}
	}
	if len(m.entries) != 4 || len(m.order) != 4 {
		t.Fatal("cache exceeded its bound")
	}
	if _, exists := m.entries[first]; exists {
		t.Fatal("oldest entry not evicted")
	}

	point, err := m.decode(first)
	if err != nil {
		t.Fatal(err)
	}
	point = bls12381.G1Affine{}
	again, err := m.decode(first)
	if err != nil || again == point {
		t.Fatal("returned value modified cached point")
	}

	before := len(m.entries)
	var invalid PublicKey
	if _, err := m.decode(invalid); err == nil {
		t.Fatal("invalid key accepted")
	}
	if len(m.entries) != before {
		t.Fatal("invalid key cached")
	}

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				if _, err := m.decode(first); err != nil {
					t.Error(err)
				}
			}
		}()
	}
	wg.Wait()
}
