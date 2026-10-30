package core

import (
	"math/big"
	"testing"
)

func TestRabbitVRFRewardViewV2Amounts(t *testing.T) {
	cases := []struct {
		name     string
		fee      int64
		members  int
		producer int64
		each     int64
	}{
		{"four-participants", 2000, 4, 400, 150},
		{"integer-rounding", 2001, 4, 400, 150},
		{"dust-does-not-create-reward", 1, 4, 0, 0},
		{"zero-fee", 0, 4, 0, 0},
		{"observed-block-share", 4910552758193070, 4, 982110551638614, 368291456864480},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fee := big.NewInt(tc.fee)
			producer, each, err := RabbitVRFRewardAmounts(fee, tc.members, true)
			if err != nil {
				t.Fatal(err)
			}
			if producer.Cmp(big.NewInt(tc.producer)) != 0 ||
				each.Cmp(big.NewInt(tc.each)) != 0 {
				t.Fatalf("producer=%s each=%s; want producer=%d each=%d",
					producer, each, tc.producer, tc.each)
			}
			if fee.Int64() != tc.fee {
				t.Fatal("reporting mutated the input fee")
			}
			total := new(big.Int).Mul(each, big.NewInt(int64(tc.members)))
			total.Add(total, producer)
			if total.Cmp(fee) > 0 {
				t.Fatal("reported rewards exceed the paid fee")
			}
		})
	}
}

func TestRabbitVRFRewardViewRejectsInvalidInput(t *testing.T) {
	for _, v2 := range []bool{false, true} {
		for _, fee := range []*big.Int{nil, big.NewInt(-1)} {
			if _, _, err := RabbitVRFRewardAmounts(fee, 4, v2); err == nil {
				t.Fatal("invalid fee accepted")
			}
		}
		for _, members := range []int{0, -1} {
			if _, _, err := RabbitVRFRewardAmounts(big.NewInt(2000), members, v2); err == nil {
				t.Fatal("invalid participant count accepted")
			}
		}
	}
}
