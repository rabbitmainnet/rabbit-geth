package rabbitvrf

import "sync"

type dkgCommitmentMemoKeyV1 struct {
	commitment    DKGCoefficientCommitmentV1
	allowInfinity bool
}

type dkgCommitmentMemoV1 struct {
	mu       sync.Mutex
	known    map[dkgCommitmentMemoKeyV1]struct{}
	order    []dkgCommitmentMemoKeyV1
	capacity int
}

var dkgCommitmentValidationCacheV1 = dkgCommitmentMemoV1{capacity: 512}

func ValidateDKGCoefficientCommitmentV1(
	commitment DKGCoefficientCommitmentV1,
	allowInfinity bool,
) error {
	return dkgCommitmentValidationCacheV1.validate(commitment, allowInfinity)
}

func (m *dkgCommitmentMemoV1) validate(
	commitment DKGCoefficientCommitmentV1,
	allowInfinity bool,
) error {
	key := dkgCommitmentMemoKeyV1{commitment, allowInfinity}
	m.mu.Lock()
	_, exists := m.known[key]
	m.mu.Unlock()
	if exists {
		return nil
	}

	// Full original validation remains mandatory on a cache miss.
	if err := validateDKGCoefficientCommitmentUncachedV1(
		commitment, allowInfinity,
	); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.capacity <= 0 {
		return nil
	}
	if m.known == nil {
		m.known = make(map[dkgCommitmentMemoKeyV1]struct{})
	}
	if _, exists := m.known[key]; !exists {
		if len(m.order) >= m.capacity {
			delete(m.known, m.order[0])
			m.order = m.order[1:]
		}
		m.known[key] = struct{}{}
		m.order = append(m.order, key)
	}
	return nil
}
