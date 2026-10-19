package lqc

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestRabbitVRFThresholdPolicyV1(
	t *testing.T,
) {
	tests := []struct {
		committeeSize uint64
		threshold     uint64
		maxFaults     uint64
	}{
		{1, 1, 0},
		{2, 2, 0},
		{3, 3, 0},
		{4, 3, 1},
		{5, 4, 1},
		{31, 21, 10},
		{32, 22, 10},
		{33, 23, 10},
		{34, 23, 11},
		{64, 43, 21},
		{100, 67, 33},
		{128, 86, 42},
	}

	for _, test := range tests {
		threshold, maxFaults, err :=
			RabbitVRFThresholdPolicyV1(
				test.committeeSize,
			)
		if err != nil {
			t.Fatalf(
				"committee=%d err=%v",
				test.committeeSize,
				err,
			)
		}

		if threshold != test.threshold ||
			maxFaults != test.maxFaults {
			t.Fatalf(
				"committee=%d threshold=%d faults=%d want=%d,%d",
				test.committeeSize,
				threshold,
				maxFaults,
				test.threshold,
				test.maxFaults,
			)
		}

		if threshold != test.committeeSize-maxFaults {
			t.Fatalf(
				"committee=%d liveness boundary mismatch",
				test.committeeSize,
			)
		}

		if threshold <= maxFaults {
			t.Fatalf(
				"committee=%d adversarial threshold invalid",
				test.committeeSize,
			)
		}
	}

	if _, _, err := RabbitVRFThresholdPolicyV1(0); err == nil {
		t.Fatal("zero committee size accepted")
	}
}

func TestRabbitVRFDKGSessionV1Deterministic(
	t *testing.T,
) {
	chainID := big.NewInt(9280)
	committeeRoot := common.HexToHash("0xaaaa")

	first, err := NewRabbitVRFDKGSessionContextV1(
		chainID,
		11,
		committeeRoot,
		32,
	)
	if err != nil {
		t.Fatal(err)
	}

	second, err := NewRabbitVRFDKGSessionContextV1(
		chainID,
		11,
		committeeRoot,
		32,
	)
	if err != nil {
		t.Fatal(err)
	}

	if first.Threshold != 22 ||
		first.MaxFaults != 10 {
		t.Fatalf(
			"threshold=%d faults=%d want=22,10",
			first.Threshold,
			first.MaxFaults,
		)
	}

	firstID, err := RabbitVRFDKGSessionIDV1(first)
	if err != nil {
		t.Fatal(err)
	}

	secondID, err := RabbitVRFDKGSessionIDV1(second)
	if err != nil {
		t.Fatal(err)
	}

	if firstID != secondID {
		t.Fatal("identical DKG context produced different session ID")
	}
}

func TestRabbitVRFDKGSessionV1BindsContext(
	t *testing.T,
) {
	baseContext, err := NewRabbitVRFDKGSessionContextV1(
		big.NewInt(9280),
		11,
		common.HexToHash("0xaaaa"),
		32,
	)
	if err != nil {
		t.Fatal(err)
	}

	baseID, err := RabbitVRFDKGSessionIDV1(
		baseContext,
	)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		make func() RabbitVRFDKGSessionContextV1
	}{
		{
			name: "chain",
			make: func() RabbitVRFDKGSessionContextV1 {
				context, err :=
					NewRabbitVRFDKGSessionContextV1(
						big.NewInt(928),
						11,
						common.HexToHash("0xaaaa"),
						32,
					)
				if err != nil {
					t.Fatal(err)
				}
				return context
			},
		},
		{
			name: "epoch",
			make: func() RabbitVRFDKGSessionContextV1 {
				context, err :=
					NewRabbitVRFDKGSessionContextV1(
						big.NewInt(9280),
						12,
						common.HexToHash("0xaaaa"),
						32,
					)
				if err != nil {
					t.Fatal(err)
				}
				return context
			},
		},
		{
			name: "committee root",
			make: func() RabbitVRFDKGSessionContextV1 {
				context, err :=
					NewRabbitVRFDKGSessionContextV1(
						big.NewInt(9280),
						11,
						common.HexToHash("0xaaab"),
						32,
					)
				if err != nil {
					t.Fatal(err)
				}
				return context
			},
		},
		{
			name: "committee size",
			make: func() RabbitVRFDKGSessionContextV1 {
				context, err :=
					NewRabbitVRFDKGSessionContextV1(
						big.NewInt(9280),
						11,
						common.HexToHash("0xaaaa"),
						33,
					)
				if err != nil {
					t.Fatal(err)
				}
				return context
			},
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			candidate := test.make()

			candidateID, err :=
				RabbitVRFDKGSessionIDV1(candidate)
			if err != nil {
				t.Fatal(err)
			}

			if candidateID == baseID {
				t.Fatal("changed DKG context did not change session ID")
			}
		})
	}
}

func TestRabbitVRFDKGSessionV1RejectsPolicyTampering(
	t *testing.T,
) {
	context, err := NewRabbitVRFDKGSessionContextV1(
		big.NewInt(9280),
		11,
		common.HexToHash("0xaaaa"),
		32,
	)
	if err != nil {
		t.Fatal(err)
	}

	wrongThreshold := context
	wrongThreshold.Threshold--

	if err := ValidateRabbitVRFDKGSessionContextV1(
		wrongThreshold,
	); err == nil {
		t.Fatal("tampered threshold accepted")
	}

	wrongFaultBudget := context
	wrongFaultBudget.MaxFaults++

	if err := ValidateRabbitVRFDKGSessionContextV1(
		wrongFaultBudget,
	); err == nil {
		t.Fatal("tampered fault budget accepted")
	}
}

func TestRabbitVRFDKGSessionV1RejectsInvalidContext(
	t *testing.T,
) {
	valid, err := NewRabbitVRFDKGSessionContextV1(
		big.NewInt(9280),
		11,
		common.HexToHash("0xaaaa"),
		32,
	)
	if err != nil {
		t.Fatal(err)
	}

	tests := []RabbitVRFDKGSessionContextV1{
		{},
		{
			Version:        RabbitVRFDKGSessionVersionV1,
			ChainID:        nil,
			TargetVRFEpoch: valid.TargetVRFEpoch,
			CommitteeRoot:  valid.CommitteeRoot,
			CommitteeSize:  valid.CommitteeSize,
			Threshold:      valid.Threshold,
			MaxFaults:      valid.MaxFaults,
		},
		{
			Version:        RabbitVRFDKGSessionVersionV1,
			ChainID:        big.NewInt(9280),
			TargetVRFEpoch: 0,
			CommitteeRoot:  valid.CommitteeRoot,
			CommitteeSize:  valid.CommitteeSize,
			Threshold:      valid.Threshold,
			MaxFaults:      valid.MaxFaults,
		},
		{
			Version:        RabbitVRFDKGSessionVersionV1,
			ChainID:        big.NewInt(9280),
			TargetVRFEpoch: valid.TargetVRFEpoch,
			CommitteeRoot:  common.Hash{},
			CommitteeSize:  valid.CommitteeSize,
			Threshold:      valid.Threshold,
			MaxFaults:      valid.MaxFaults,
		},
	}

	for index, context := range tests {
		if _, err := RabbitVRFDKGSessionIDV1(
			context,
		); err == nil {
			t.Fatalf(
				"invalid context accepted at index %d",
				index,
			)
		}
	}
}
