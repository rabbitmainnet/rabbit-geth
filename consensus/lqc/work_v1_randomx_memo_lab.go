//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package lqc

import (
	"container/list"
	"errors"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

var errWorkV1RandomXMemoClosed = errors.New("closed local RandomX verification cache")

type workV1RandomXMemoKey struct {
	epoch common.Hash
	input common.Hash
}
type workV1RandomXMemoValue struct {
	key  workV1RandomXMemoKey
	hash common.Hash
}

// Only deterministic RandomX OUTPUTS are memoized. Every caller still checks
// its own difficulty, identity, signature, target, role and canonical context.
// The key contains the entire dataset anchor and a digest of the full input.
func newWorkV1MemoizedRandomXHasherV1(hash WorkRelayHasherV1, closeHasher func(), capacity int) (WorkRelayHasherV1, func()) {
	var mu sync.Mutex
	entries := make(map[workV1RandomXMemoKey]*list.Element)
	recent := list.New()
	closed := false
	compute := func(epoch common.Hash, input []byte) (common.Hash, error) {
		key := workV1RandomXMemoKey{epoch: epoch, input: crypto.Keccak256Hash(input)}
		mu.Lock()
		defer mu.Unlock()
		if closed {
			return common.Hash{}, errWorkV1RandomXMemoClosed
		}
		if entry := entries[key]; entry != nil {
			recent.MoveToFront(entry)
			return entry.Value.(workV1RandomXMemoValue).hash, nil
		}
		value, err := hash(epoch, input)
		if err != nil {
			return value, err
		}
		if capacity > 0 {
			entries[key] = recent.PushFront(workV1RandomXMemoValue{key: key, hash: value})
			if recent.Len() > capacity {
				old := recent.Back()
				delete(entries, old.Value.(workV1RandomXMemoValue).key)
				recent.Remove(old)
			}
		}
		return value, nil
	}
	close := func() {
		mu.Lock()
		defer mu.Unlock()
		if closed {
			return
		}
		closed = true
		entries = nil
		recent.Init()
		if closeHasher != nil {
			closeHasher()
		}
	}
	return compute, close
}
