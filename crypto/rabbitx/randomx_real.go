//go:build (linux || darwin || windows) && cgo && rabbit_randomx

package rabbitx

/*
#cgo linux LDFLAGS: -lstdc++ -lpthread -ldl -lm
#cgo darwin LDFLAGS: -lc++ -lpthread -lm
#cgo windows LDFLAGS: -lstdc++
#include <stdint.h>
#include <stddef.h>

typedef struct randomx_cache randomx_cache;
typedef struct randomx_dataset randomx_dataset;
typedef struct randomx_vm randomx_vm;
typedef uint32_t randomx_flags;

extern randomx_flags randomx_get_flags(void);
extern randomx_cache* randomx_alloc_cache(randomx_flags flags);
extern void randomx_init_cache(randomx_cache* cache, const void* key, size_t keySize);
extern void randomx_release_cache(randomx_cache* cache);
extern randomx_dataset* randomx_alloc_dataset(randomx_flags flags);
extern unsigned long randomx_dataset_item_count(void);
extern void randomx_init_dataset(randomx_dataset* dataset, randomx_cache* cache, unsigned long startItem, unsigned long itemCount);
extern void randomx_release_dataset(randomx_dataset* dataset);
extern randomx_vm* randomx_create_vm(randomx_flags flags, randomx_cache* cache, randomx_dataset* dataset);
extern void randomx_destroy_vm(randomx_vm* machine);
extern void randomx_calculate_hash(randomx_vm* machine, const void* input, size_t inputSize, void* output);

static randomx_flags rabbit_randomx_light_flags(void) {
	return (randomx_flags)(randomx_get_flags() & ~((randomx_flags)4));
}

static randomx_flags rabbit_randomx_full_flags(void) {
	return (randomx_flags)(randomx_get_flags() | ((randomx_flags)4));
}
*/
import "C"

import (
	"errors"
	"sync"
	"unsafe"

	"github.com/ethereum/go-ethereum/common"
)

var (
	ErrUnavailable = errors.New("rabbit RandomX runtime unavailable")
	ErrAllocation  = errors.New("rabbit RandomX allocation failed")
	ErrClosed      = errors.New("rabbit RandomX hasher closed")
)

// Keep at most two epoch caches. Header verification and block execution may
// alternate between adjacent epochs; neither may reuse a VM with a different key.
const lightHasherCacheLimit = 2

type lightHasherEpoch struct {
	cache *C.randomx_cache
	vm    *C.randomx_vm
	key   common.Hash
}

type LightHasher struct {
	mu     sync.Mutex
	epochs [lightHasherCacheLimit]*lightHasherEpoch // most recently used first
	closed bool
}

// FullHasher uses the frozen Rabbit RandomX 1 GiB dataset for mining. Nodes
// should continue using LightHasher for bounded-memory ticket verification.
type FullHasher struct {
	mu          sync.Mutex
	dataset     *C.randomx_dataset
	vm          *C.randomx_vm
	key         common.Hash
	initialized bool
	closed      bool
}

func NewLightHasher() (*LightHasher, error) {
	return &LightHasher{}, nil
}

func NewFullHasher() (*FullHasher, error) {
	return &FullHasher{}, nil
}

func (h *FullHasher) resetLocked(key common.Hash) error {
	if h.vm != nil {
		C.randomx_destroy_vm(h.vm)
		h.vm = nil
	}
	if h.dataset != nil {
		C.randomx_release_dataset(h.dataset)
		h.dataset = nil
	}

	flags := C.rabbit_randomx_full_flags()
	cache := C.randomx_alloc_cache(flags)
	if cache == nil {
		return ErrAllocation
	}
	C.randomx_init_cache(cache, unsafe.Pointer(&key[0]), C.size_t(len(key)))

	dataset := C.randomx_alloc_dataset(flags)
	if dataset == nil {
		C.randomx_release_cache(cache)
		return ErrAllocation
	}
	C.randomx_init_dataset(dataset, cache, 0, C.randomx_dataset_item_count())
	C.randomx_release_cache(cache)

	vm := C.randomx_create_vm(flags, nil, dataset)
	if vm == nil {
		C.randomx_release_dataset(dataset)
		return ErrAllocation
	}

	h.dataset = dataset
	h.vm = vm
	h.key = key
	h.initialized = true
	return nil
}

func (h *FullHasher) Hash(epochKey common.Hash, input []byte) (common.Hash, error) {
	if h == nil || epochKey == (common.Hash{}) || len(input) == 0 {
		return common.Hash{}, ErrUnavailable
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return common.Hash{}, ErrClosed
	}
	if !h.initialized || h.key != epochKey {
		if err := h.resetLocked(epochKey); err != nil {
			return common.Hash{}, err
		}
	}
	var out common.Hash
	C.randomx_calculate_hash(
		h.vm,
		unsafe.Pointer(&input[0]),
		C.size_t(len(input)),
		unsafe.Pointer(&out[0]),
	)
	return out, nil
}

func (h *FullHasher) Close() {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.vm != nil {
		C.randomx_destroy_vm(h.vm)
		h.vm = nil
	}
	if h.dataset != nil {
		C.randomx_release_dataset(h.dataset)
		h.dataset = nil
	}
	h.initialized = false
	h.closed = true
}

func (epoch *lightHasherEpoch) release() {
	if epoch == nil {
		return
	}
	if epoch.vm != nil {
		C.randomx_destroy_vm(epoch.vm)
		epoch.vm = nil
	}
	if epoch.cache != nil {
		C.randomx_release_cache(epoch.cache)
		epoch.cache = nil
	}
}

func (h *LightHasher) epochLocked(key common.Hash) (*lightHasherEpoch, error) {
	for index, epoch := range h.epochs {
		if epoch != nil && epoch.key == key {
			if index == 1 {
				h.epochs[0], h.epochs[1] = h.epochs[1], h.epochs[0]
			}
			return epoch, nil
		}
	}
	// Release the oldest before allocating, so even transient use stays bounded.
	h.epochs[1].release()
	h.epochs[1] = nil
	flags := C.rabbit_randomx_light_flags()
	cache := C.randomx_alloc_cache(flags)
	if cache == nil {
		return nil, ErrAllocation
	}
	C.randomx_init_cache(cache, unsafe.Pointer(&key[0]), C.size_t(len(key)))
	vm := C.randomx_create_vm(flags, cache, nil)
	if vm == nil {
		C.randomx_release_cache(cache)
		return nil, ErrAllocation
	}
	epoch := &lightHasherEpoch{cache: cache, vm: vm, key: key}
	h.epochs[1] = h.epochs[0]
	h.epochs[0] = epoch
	return epoch, nil
}

func (h *LightHasher) Hash(epochKey common.Hash, input []byte) (common.Hash, error) {
	if h == nil || epochKey == (common.Hash{}) || len(input) == 0 {
		return common.Hash{}, ErrUnavailable
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return common.Hash{}, ErrClosed
	}
	epoch, err := h.epochLocked(epochKey)
	if err != nil {
		return common.Hash{}, err
	}
	var out common.Hash
	C.randomx_calculate_hash(
		epoch.vm, unsafe.Pointer(&input[0]), C.size_t(len(input)),
		unsafe.Pointer(&out[0]),
	)
	return out, nil
}

func (h *LightHasher) Close() {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for index, epoch := range h.epochs {
		epoch.release()
		h.epochs[index] = nil
	}
	h.closed = true
}
