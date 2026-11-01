package eth

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

func TestLQCDeferredExtensionV2420(t *testing.T) {
	old := &types.Header{Number: big.NewInt(101), Extra: []byte("rejected")}
	newer := &types.Header{Number: big.NewInt(102), Extra: []byte("new")}
	if !lqcMayRetryExtensionV2420(common.Hash{}, 80, old, 80) {
		t.Fatal("initial extension was incorrectly deferred")
	}
	if lqcMayRetryExtensionV2420(old.Hash(), 80, old, 80) {
		t.Fatal("same rejected head retried without progress")
	}
	if !lqcMayRetryExtensionV2420(old.Hash(), 80, old, 81) {
		t.Fatal("chain progress did not allow retry")
	}
	if !lqcMayRetryExtensionV2420(old.Hash(), 80, newer, 80) {
		t.Fatal("different target was blocked")
	}
	if lqcMayRetryExtensionV2420(common.Hash{}, 0, nil, 0) {
		t.Fatal("nil head accepted")
	}
}
