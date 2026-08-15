//go:build linux && cgo && rabbit_randomx

package rabbitx

/*
#cgo LDFLAGS: -lstdc++ -lpthread -ldl -lm
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
extern randomx_vm* randomx_create_vm(randomx_flags flags, randomx_cache* cache, randomx_dataset* dataset);
extern void randomx_destroy_vm(randomx_vm* machine);
extern void randomx_calculate_hash(randomx_vm* machine, const void* input, size_t inputSize, void* output);

static randomx_flags rabbit_randomx_light_flags(void) {
	return (randomx_flags)(randomx_get_flags() & ~((randomx_flags)4));
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

type LightHasher struct {
	mu          sync.Mutex
	cache       *C.randomx_cache
	vm          *C.randomx_vm
	key         common.Hash
	initialized bool
	closed      bool
}

func NewLightHasher() (*LightHasher, error) {
	return &LightHasher{}, nil
}

func (h *LightHasher) resetLocked(key common.Hash) error {
	if h.vm != nil {
		C.randomx_destroy_vm(h.vm)
		h.vm = nil
	}
	if h.cache != nil {
		C.randomx_release_cache(h.cache)
		h.cache = nil
	}

	flags := C.rabbit_randomx_light_flags()
	cache := C.randomx_alloc_cache(flags)
	if cache == nil {
		return ErrAllocation
	}
	C.randomx_init_cache(cache, unsafe.Pointer(&key[0]), C.size_t(len(key)))

	vm := C.randomx_create_vm(flags, cache, nil)
	if vm == nil {
		C.randomx_release_cache(cache)
		return ErrAllocation
	}

	h.cache = cache
	h.vm = vm
	h.key = key
	h.initialized = true
	return nil
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

func (h *LightHasher) Close() {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.vm != nil {
		C.randomx_destroy_vm(h.vm)
		h.vm = nil
	}
	if h.cache != nil {
		C.randomx_release_cache(h.cache)
		h.cache = nil
	}
	h.initialized = false
	h.closed = true
}
