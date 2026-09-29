package core

import (
	"errors"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core/types"
)

type rabbitVRFReplayReaderV1 struct {
	calls  int
	values []consensus.RabbitVRFValidatedFinalization
	hitAt  int
}

func (r *rabbitVRFReplayReaderV1) RabbitVRFValidatedFinalizations(
	common.Hash,
) ([]consensus.RabbitVRFValidatedFinalization, bool, error) {
	r.calls++
	if r.hitAt > 0 && r.calls >= r.hitAt {
		return r.values, true, nil
	}
	return nil, false, nil
}

func TestRabbitVRFValidatedFinalizationsReplayV1(t *testing.T) {
	header := &types.Header{}
	blockHash := common.HexToHash("0x1234")

	expected := []consensus.RabbitVRFValidatedFinalization{
		{
			RequestID:  common.HexToHash("0x01"),
			Epoch:      4,
			Round:      385,
			Randomness: common.HexToHash("0x02"),
			ProofHash:  common.HexToHash("0x03"),
		},
	}

	t.Run("cache_miss_verify_then_hit", func(t *testing.T) {
		reader := &rabbitVRFReplayReaderV1{
			values: expected,
			hitAt:  2,
		}
		verifyCalls := 0

		got, err := rabbitVRFValidatedFinalizationsForExecution(
			reader,
			func(consensus.ChainHeaderReader, *types.Header) error {
				verifyCalls++
				return nil
			},
			nil,
			header,
			blockHash,
		)
		if err != nil {
			t.Fatal(err)
		}
		if verifyCalls != 1 {
			t.Fatalf("VerifyHeader calls=%d want=1", verifyCalls)
		}
		if reader.calls != 2 {
			t.Fatalf("reader calls=%d want=2", reader.calls)
		}
		if len(got) != 1 || got[0] != expected[0] {
			t.Fatalf("finalizations=%+v want=%+v", got, expected)
		}
	})

	t.Run("cache_miss_verify_still_missing", func(t *testing.T) {
		reader := &rabbitVRFReplayReaderV1{}
		verifyCalls := 0

		_, err := rabbitVRFValidatedFinalizationsForExecution(
			reader,
			func(consensus.ChainHeaderReader, *types.Header) error {
				verifyCalls++
				return nil
			},
			nil,
			header,
			blockHash,
		)
		if err == nil || !strings.Contains(
			err.Error(),
			"Rabbit VRF validated finalizations unavailable",
		) {
			t.Fatalf("error=%v", err)
		}
		if verifyCalls != 1 {
			t.Fatalf("VerifyHeader calls=%d want=1", verifyCalls)
		}
		if reader.calls != 2 {
			t.Fatalf("reader calls=%d want=2", reader.calls)
		}
	})

	t.Run("verify_failure_is_fail_closed", func(t *testing.T) {
		reader := &rabbitVRFReplayReaderV1{}
		verifyErr := errors.New("invalid V5 header")

		_, err := rabbitVRFValidatedFinalizationsForExecution(
			reader,
			func(consensus.ChainHeaderReader, *types.Header) error {
				return verifyErr
			},
			nil,
			header,
			blockHash,
		)
		if !errors.Is(err, verifyErr) {
			t.Fatalf("error=%v want=%v", err, verifyErr)
		}
		if reader.calls != 1 {
			t.Fatalf("reader calls=%d want=1", reader.calls)
		}
	})
}
