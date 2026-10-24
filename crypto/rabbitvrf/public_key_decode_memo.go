package rabbitvrf

import (
	bls12381 "github.com/consensys/gnark-crypto/ecc/bls12-381"
	"sync"
)

type publicKeyDecodeMemoV1 struct {
	mu       sync.Mutex
	entries  map[PublicKey]bls12381.G1Affine
	order    []PublicKey
	capacity int
}

var publicKeyDecodeCacheV1 = publicKeyDecodeMemoV1{capacity: 256}

func decodePublicKey(encoded PublicKey) (bls12381.G1Affine, error) {
	return publicKeyDecodeCacheV1.decode(encoded)
}

func (m *publicKeyDecodeMemoV1) decode(encoded PublicKey) (bls12381.G1Affine, error) {
	m.mu.Lock()
	point, found := m.entries[encoded]
	m.mu.Unlock()
	if found {
		return point, nil
	}

	// Preserve every original decoding, curve and subgroup check.
	point, err := decodePublicKeyUncachedV1(encoded)
	if err != nil {
		return bls12381.G1Affine{}, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.capacity <= 0 {
		return point, nil
	}
	if m.entries == nil {
		m.entries = make(map[PublicKey]bls12381.G1Affine)
	}
	if _, exists := m.entries[encoded]; !exists {
		if len(m.order) >= m.capacity {
			delete(m.entries, m.order[0])
			m.order = m.order[1:]
		}
		m.entries[encoded] = point
		m.order = append(m.order, encoded)
	}
	return point, nil
}
