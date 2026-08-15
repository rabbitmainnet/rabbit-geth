//go:build linux && cgo

package main

/*
#cgo linux LDFLAGS: -l:libargon2.so.1
#include <stdint.h>
#include <stdlib.h>

typedef enum Argon2_type { Argon2_d = 0, Argon2_i = 1, Argon2_id = 2 } argon2_type;
typedef int (*allocate_fptr)(uint8_t **memory, size_t bytes_to_allocate);
typedef void (*deallocate_fptr)(uint8_t *memory, size_t bytes_to_allocate);

typedef struct Argon2_Context {
	uint8_t *out;
	uint32_t outlen;
	uint8_t *pwd;
	uint32_t pwdlen;
	uint8_t *salt;
	uint32_t saltlen;
	uint8_t *secret;
	uint32_t secretlen;
	uint8_t *ad;
	uint32_t adlen;
	uint32_t t_cost;
	uint32_t m_cost;
	uint32_t lanes;
	uint32_t threads;
	uint32_t version;
	allocate_fptr allocate_cbk;
	deallocate_fptr free_cbk;
	uint32_t flags;
} argon2_context;

extern int argon2_ctx(argon2_context *context, argon2_type type);
extern const char *argon2_error_message(int error_code);

static uint8_t *rabbit_workspace;
static size_t rabbit_workspace_size;
static uint64_t rabbit_allocation_count;

static int rabbit_reuse_allocate(uint8_t **memory, size_t bytes) {
	if (rabbit_workspace_size < bytes) {
		uint8_t *next = (uint8_t *)realloc(rabbit_workspace, bytes);
		if (next == NULL) return -1;
		rabbit_workspace = next;
		rabbit_workspace_size = bytes;
		rabbit_allocation_count++;
	}
	*memory = rabbit_workspace;
	return 0;
}

static void rabbit_reuse_free(uint8_t *memory, size_t bytes) {
	(void)memory;
	(void)bytes;
}

static int rabbit_argon2id_reuse(
	uint8_t *output,
	uint8_t *input,
	uint32_t input_len,
	uint8_t *salt,
	uint32_t salt_len,
	uint32_t memory_kib
) {
	argon2_context context = {
		.out = output,
		.outlen = 32,
		.pwd = input,
		.pwdlen = input_len,
		.salt = salt,
		.saltlen = salt_len,
		.secret = NULL,
		.secretlen = 0,
		.ad = NULL,
		.adlen = 0,
		.t_cost = 1,
		.m_cost = memory_kib,
		.lanes = 1,
		.threads = 1,
		.version = 0x13,
		.allocate_cbk = rabbit_reuse_allocate,
		.free_cbk = rabbit_reuse_free,
		.flags = 0,
	};
	return argon2_ctx(&context, Argon2_id);
}

static size_t rabbit_reuse_workspace_size(void) {
	return rabbit_workspace_size;
}

static uint64_t rabbit_reuse_allocation_count(void) {
	return rabbit_allocation_count;
}

static void rabbit_reuse_workspace_reset(void) {
	free(rabbit_workspace);
	rabbit_workspace = NULL;
	rabbit_workspace_size = 0;
	rabbit_allocation_count = 0;
}
*/
import "C"

import (
	"fmt"
	"sync"
	"unsafe"
)

var reusableArgon2Mutex sync.Mutex

func reusableArgon2IDInto(input, salt []byte, memoryKiB uint32, output *[32]byte) error {
	if len(input) == 0 || len(salt) == 0 || output == nil {
		return fmt.Errorf("input, salt and output are required")
	}
	reusableArgon2Mutex.Lock()
	defer reusableArgon2Mutex.Unlock()
	code := C.rabbit_argon2id_reuse(
		(*C.uint8_t)(unsafe.Pointer(&output[0])),
		(*C.uint8_t)(unsafe.Pointer(&input[0])),
		C.uint32_t(len(input)),
		(*C.uint8_t)(unsafe.Pointer(&salt[0])),
		C.uint32_t(len(salt)),
		C.uint32_t(memoryKiB),
	)
	if code != 0 {
		return fmt.Errorf("argon2id native error %d: %s", int(code), C.GoString(C.argon2_error_message(code)))
	}
	return nil
}

func reusableWorkspaceStats() (uint64, uint64) {
	reusableArgon2Mutex.Lock()
	defer reusableArgon2Mutex.Unlock()
	return uint64(C.rabbit_reuse_workspace_size()), uint64(C.rabbit_reuse_allocation_count())
}

func resetReusableWorkspace() {
	reusableArgon2Mutex.Lock()
	defer reusableArgon2Mutex.Unlock()
	C.rabbit_reuse_workspace_reset()
}
