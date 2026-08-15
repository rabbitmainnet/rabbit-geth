package lqc

import (
	"math/big"
	"testing"
)

func TestWorkDifficultyV1ProfileCapacity(t *testing.T) {
	if err := ValidateWorkProtocolProfileV1(); err != nil {
		t.Fatal(err)
	}
	if WorkProtocolEpochLengthV1 != 128 {
		t.Fatalf(
			"epoch length = %d, want 128",
			WorkProtocolEpochLengthV1,
		)
	}
	if MaxWorkTicketsPerBlockV1 != 8 {
		t.Fatalf(
			"max tickets/block = %d, want 8",
			MaxWorkTicketsPerBlockV1,
		)
	}
	if TargetWorkTicketsPerEpochV1 != 256 {
		t.Fatalf(
			"target tickets/epoch = %d, want 256",
			TargetWorkTicketsPerEpochV1,
		)
	}
	if WorkTicketCommitCapacityPerEpochV1 != 1024 {
		t.Fatalf(
			"commit capacity = %d, want 1024",
			WorkTicketCommitCapacityPerEpochV1,
		)
	}
}

func TestWorkDifficultyV1StableAtTarget(t *testing.T) {
	current := big.NewInt(1000)
	next, err := NextWorkDifficultyV1(
		current,
		TargetWorkTicketsPerEpochV1,
	)
	if err != nil {
		t.Fatal(err)
	}
	if next.Cmp(current) != 0 {
		t.Fatalf("next = %s, want %s", next, current)
	}
}

func TestWorkDifficultyV1MaximumDownwardStep(t *testing.T) {
	for _, observed := range []uint64{0, 1, 64, 127, 128} {
		next, err := NextWorkDifficultyV1(
			big.NewInt(1000),
			observed,
		)
		if err != nil {
			t.Fatal(err)
		}
		if next.Cmp(big.NewInt(500)) != 0 {
			t.Fatalf(
				"observed=%d next=%s, want 500",
				observed,
				next,
			)
		}
	}
}

func TestWorkDifficultyV1ProportionalMiddleRange(t *testing.T) {
	tests := []struct {
		observed uint64
		want     int64
	}{
		{192, 750},
		{256, 1000},
		{384, 1500},
		{512, 2000},
		{768, 3000},
	}

	for _, test := range tests {
		next, err := NextWorkDifficultyV1(
			big.NewInt(1000),
			test.observed,
		)
		if err != nil {
			t.Fatal(err)
		}
		if next.Cmp(big.NewInt(test.want)) != 0 {
			t.Fatalf(
				"observed=%d next=%s, want %d",
				test.observed,
				next,
				test.want,
			)
		}
	}
}

func TestWorkDifficultyV1MaximumUpwardStep(t *testing.T) {
	for _, observed := range []uint64{1024, 1025, 5000, ^uint64(0)} {
		next, err := NextWorkDifficultyV1(
			big.NewInt(1000),
			observed,
		)
		if err != nil {
			t.Fatal(err)
		}
		if next.Cmp(big.NewInt(4000)) != 0 {
			t.Fatalf(
				"observed=%d next=%s, want 4000",
				observed,
				next,
			)
		}
	}
}

func TestWorkDifficultyV1NeverFallsBelowOne(t *testing.T) {
	next, err := NextWorkDifficultyV1(
		big.NewInt(1),
		0,
	)
	if err != nil {
		t.Fatal(err)
	}
	if next.Cmp(big.NewInt(1)) != 0 {
		t.Fatalf("next = %s, want 1", next)
	}
}

func TestWorkDifficultyV1RejectsInvalidCurrent(t *testing.T) {
	if _, err := NextWorkDifficultyV1(nil, 256); err != ErrInvalidWorkDifficultyV1 {
		t.Fatalf("nil error = %v", err)
	}
	if _, err := NextWorkDifficultyV1(big.NewInt(0), 256); err != ErrInvalidWorkDifficultyV1 {
		t.Fatalf("zero error = %v", err)
	}
}
