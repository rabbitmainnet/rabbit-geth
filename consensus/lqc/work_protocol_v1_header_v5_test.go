package lqc

import (
	"errors"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	rabbitvrf "github.com/ethereum/go-ethereum/crypto/rabbitvrf"
)

func TestLQCHeaderExtraV5RabbitVRFFinalizations(t *testing.T) {
	finalA := RabbitVRFFinalizationV1{
		Version:    RabbitVRFFinalizationVersionV1,
		KeysetRoot: common.HexToHash("0x1001"),
		RequestID:  common.HexToHash("0x01"),
		Epoch:      11,
		Round:      2,
		Randomness: common.HexToHash("0x11"),
		Signature:  rabbitvrf.Signature{1},
		ProofHash:  common.HexToHash("0x21"),
	}
	finalB := RabbitVRFFinalizationV1{
		Version:    RabbitVRFFinalizationVersionV1,
		KeysetRoot: common.HexToHash("0x1001"),
		RequestID:  common.HexToHash("0x02"),
		Epoch:      11,
		Round:      3,
		Randomness: common.HexToHash("0x12"),
		Signature:  rabbitvrf.Signature{1},
		ProofHash:  common.HexToHash("0x22"),
	}
	extra, err := EncodeLQCHeaderExtraV5(
		100,
		common.HexToHash("0x31"),
		common.HexToHash("0x32"),
		common.HexToHash("0x33"),
		nil,
		nil,
		nil,
		[]RabbitVRFFinalizationV1{finalB, finalA},
		16,
	)
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := ValidateLQCHeaderExtraV5(100, 16, extra)
	if err != nil {
		t.Fatal(err)
	}
	if len(envelope.RabbitVRFFinalizations) != 2 ||
		envelope.RabbitVRFFinalizations[0] != finalA ||
		envelope.RabbitVRFFinalizations[1] != finalB {
		t.Fatal("rabbit vrf finalizations were not encoded canonically")
	}
	if _, err := EncodeLQCHeaderExtraV5(
		100,
		common.HexToHash("0x31"),
		common.HexToHash("0x32"),
		common.HexToHash("0x33"),
		nil,
		nil,
		nil,
		[]RabbitVRFFinalizationV1{finalA, finalA},
		16,
	); !errors.Is(err, ErrDuplicateRabbitVRFFinalizationV1) {
		t.Fatalf("duplicate finalization error=%v", err)
	}
}

func TestLQCHeaderExtraV5SizeLimit(t *testing.T) {
	extra, err := EncodeLQCHeaderExtraV5(
		100,
		common.HexToHash("0x31"),
		common.HexToHash("0x32"),
		common.HexToHash("0x33"),
		nil,
		nil,
		nil,
		nil,
		16,
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(extra) > MaxRegistryHeaderExtraSize {
		t.Fatalf(
			"valid V5 header exceeds limit: %d > %d",
			len(extra),
			MaxRegistryHeaderExtraSize,
		)
	}

	oversized := append([]byte(nil), extra...)
	oversized = append(
		oversized,
		make([]byte, MaxRegistryHeaderExtraSize-len(oversized)+1)...,
	)

	if len(oversized) != MaxRegistryHeaderExtraSize+1 {
		t.Fatalf(
			"oversized header size=%d want=%d",
			len(oversized),
			MaxRegistryHeaderExtraSize+1,
		)
	}

	if _, err := DecodeLQCHeaderExtraV5(
		oversized,
		16,
	); !errors.Is(err, ErrInvalidLQCHeaderExtraV5) {
		t.Fatalf("oversized V5 header error=%v", err)
	}
}
