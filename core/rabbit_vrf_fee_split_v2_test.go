package core

import (
	"math/big"
	"testing"
)

func TestRabbitVRFFeeSplitV2(t *testing.T) {
	fee := big.NewInt(1_000_000_000_000)
	p, each, treasury, err := rabbitVRFSplitProtocolFeeV2(fee, 3)
	if err != nil {
		t.Fatal(err)
	}
	if p.Int64() != 200_000_000_000 ||
		each.Int64() != 100_000_000_000 ||
		treasury.Int64() != 500_000_000_000 {
		t.Fatalf("unexpected split: %s %s %s", p, each, treasury)
	}
	if fee.Int64() != 1_000_000_000_000 {
		t.Fatal("input fee mutated")
	}
}

func TestRabbitVRFFeeSplitV2Conservation(t *testing.T) {
	for _, amount := range []int64{0, 1, 9, 101, 1000, 1000000000001} {
		for _, count := range []int{1, 3, 32, 128} {
			fee := big.NewInt(amount)
			p, each, treasury, err := rabbitVRFSplitProtocolFeeV2(fee, count)
			if err != nil {
				t.Fatal(err)
			}
			total := new(big.Int).Mul(each, big.NewInt(int64(count)))
			total.Add(total, p)
			total.Add(total, treasury)
			if total.Cmp(fee) != 0 || treasury.Sign() < 0 {
				t.Fatalf("conservation failed: fee=%d count=%d", amount, count)
			}
		}
	}
}

func TestRabbitVRFFeeSplitV2Invalid(t *testing.T) {
	for _, fee := range []*big.Int{nil, big.NewInt(-1)} {
		if _, _, _, err := rabbitVRFSplitProtocolFeeV2(fee, 3); err == nil {
			t.Fatal("invalid fee accepted")
		}
	}
	for _, count := range []int{0, -1} {
		if _, _, _, err := rabbitVRFSplitProtocolFeeV2(big.NewInt(100), count); err == nil {
			t.Fatal("invalid participant count accepted")
		}
	}
}
