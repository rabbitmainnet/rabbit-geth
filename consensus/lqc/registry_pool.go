package lqc

import (
	"errors"
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum/common"
)

const MaxRegistryPoolOperations = 4096

var (
	ErrRegistryPoolDisabled     = errors.New("lqc registry operation pool disabled")
	ErrRegistryPoolFull         = errors.New("lqc registry operation pool full")
	ErrRegistryOperationKnown   = errors.New("lqc registry operation already known")
	ErrRegistryOperationPending = errors.New("lqc registry address already has a pending operation")
)

// RegistryOperationPool is a bounded, in-memory relay pool. It is not a source
// of consensus truth: every operation is revalidated against the canonical
// parent snapshot before a producer places it in a header.
type RegistryOperationPool struct {
	mu        sync.RWMutex
	byHash    map[common.Hash]RegistryOperation
	byAddress map[common.Address]common.Hash
}

type RegistryPoolStatus struct {
	Pending  int `json:"pending"`
	Capacity int `json:"capacity"`
}

func NewRegistryOperationPool() *RegistryOperationPool {
	return &RegistryOperationPool{
		byHash:    make(map[common.Hash]RegistryOperation),
		byAddress: make(map[common.Address]common.Hash),
	}
}

func RegistryOperationHash(chainID *big.Int, operation RegistryOperation) common.Hash {
	return RegistryOperationSigningHash(chainID, operation)
}

func cloneRegistryOperation(operation RegistryOperation) RegistryOperation {
	clone := operation
	clone.Signature = append([]byte(nil), operation.Signature...)
	return clone
}

func (p *RegistryOperationPool) Add(chainID *big.Int, operation RegistryOperation) (common.Hash, error) {
	if p == nil {
		return common.Hash{}, ErrRegistryPoolDisabled
	}
	hash := RegistryOperationHash(chainID, operation)
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, exists := p.byHash[hash]; exists {
		return hash, ErrRegistryOperationKnown
	}
	if previousHash, exists := p.byAddress[operation.Address]; exists {
		previous := p.byHash[previousHash]
		if operation.Sequence <= previous.Sequence {
			return common.Hash{}, ErrRegistryOperationPending
		}
		delete(p.byHash, previousHash)
	} else if len(p.byHash) >= MaxRegistryPoolOperations {
		return common.Hash{}, ErrRegistryPoolFull
	}
	p.byHash[hash] = cloneRegistryOperation(operation)
	p.byAddress[operation.Address] = hash
	return hash, nil
}

func (p *RegistryOperationPool) Has(hash common.Hash) bool {
	if p == nil {
		return false
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	_, exists := p.byHash[hash]
	return exists
}

func (p *RegistryOperationPool) Pending(blockNumber uint64) []RegistryOperation {
	if p == nil {
		return nil
	}
	p.mu.RLock()
	operations := make([]RegistryOperation, 0, len(p.byHash))
	for _, operation := range p.byHash {
		if operation.ValidUntil >= blockNumber {
			operations = append(operations, cloneRegistryOperation(operation))
		}
	}
	p.mu.RUnlock()
	return CanonicalRegistryOperations(operations)
}

func (p *RegistryOperationPool) PruneExpired(blockNumber uint64) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for hash, operation := range p.byHash {
		if operation.ValidUntil < blockNumber {
			delete(p.byHash, hash)
			delete(p.byAddress, operation.Address)
		}
	}
}

func (p *RegistryOperationPool) Status() RegistryPoolStatus {
	status := RegistryPoolStatus{Capacity: MaxRegistryPoolOperations}
	if p == nil {
		return status
	}
	p.mu.RLock()
	status.Pending = len(p.byHash)
	p.mu.RUnlock()
	return status
}
