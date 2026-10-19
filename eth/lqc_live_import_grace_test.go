package eth

import (
	"math/big"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

func TestLQCLiveImportGrace(t *testing.T) {
	parent := &types.Header{Number: big.NewInt(10)}
	target := &types.Header{Number: big.NewInt(11), ParentHash: parent.Hash()}
	t.Run("announcement races with successful live import", func(t *testing.T) {
		var head atomic.Pointer[types.Header]
		head.Store(parent)
		observed := make(chan struct{})
		result := make(chan bool, 1)
		go func() {
			first := true
			result <- waitForLQCLiveImport(func() *types.Header {
				snapshot := head.Load()
				if first {
					first = false
					close(observed)
				}
				return snapshot
			}, target, nil, time.Second)
		}()
		<-observed
		head.Store(target)
		select {
		case recover := <-result:
			if recover {
				t.Fatal("normal live import unnecessarily requested recovery")
			}
		case <-time.After(2 * time.Second):
			t.Fatal("live import waiter did not finish")
		}
	})
	t.Run("missing live block still recovers", func(t *testing.T) {
		if !waitForLQCLiveImport(func() *types.Header { return parent }, target, nil, 20*time.Millisecond) {
			t.Fatal("missing live block suppressed recovery")
		}
	})
	t.Run("shutdown cancels grace", func(t *testing.T) {
		quit := make(chan struct{})
		close(quit)
		if waitForLQCLiveImport(func() *types.Header { return parent }, target, quit, time.Second) {
			t.Fatal("shutdown requested recovery")
		}
	})
	for name, candidate := range map[string]*types.Header{
		"gap":              {Number: big.NewInt(12), ParentHash: target.Hash()},
		"competing parent": {Number: big.NewInt(11), ParentHash: common.HexToHash("0x01")},
	} {
		t.Run(name, func(t *testing.T) {
			reads := 0
			if !waitForLQCLiveImport(func() *types.Header { reads++; return parent }, candidate, nil, time.Hour) {
				t.Fatal("real recovery target suppressed")
			}
			if reads != 1 {
				t.Fatal("real recovery target entered grace")
			}
		})
	}
	t.Run("chain changes during grace", func(t *testing.T) {
		reads := 0
		conflicting := &types.Header{Number: big.NewInt(10), Extra: []byte("other fork")}
		if !waitForLQCLiveImport(func() *types.Header {
			reads++
			if reads == 1 {
				return parent
			}
			return conflicting
		}, target, nil, time.Second) {
			t.Fatal("changed parent suppressed recovery")
		}
	})
	t.Run("already canonical", func(t *testing.T) {
		if waitForLQCLiveImport(func() *types.Header { return target }, target, nil, time.Hour) {
			t.Fatal("canonical target requested recovery")
		}
	})
}
