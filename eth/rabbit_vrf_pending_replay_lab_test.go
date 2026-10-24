//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"errors"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestRabbitVRFPendingReplayRetainsIncompleteV1(t *testing.T) {
	a := common.HexToHash("0xaa")
	b := common.HexToHash("0xbb")

	calls := 0
	remaining, err := rabbitVRFReplayPendingV1(
		[]common.Hash{a, b},
		func(id common.Hash) error {
			calls++
			if id == b {
				return errRabbitVRFRequestNotPendingV1
			}
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || len(remaining) != 1 || remaining[0] != a {
		t.Fatal("incomplete request was lost")
	}
}

func TestRabbitVRFPendingReplayFailsClosedV1(t *testing.T) {
	a := common.HexToHash("0xaa")
	failure := errors.New("transport unavailable")

	_, err := rabbitVRFReplayPendingV1(
		[]common.Hash{a},
		func(common.Hash) error { return failure },
	)
	if !errors.Is(err, failure) {
		t.Fatalf("error lost: %v", err)
	}
}
