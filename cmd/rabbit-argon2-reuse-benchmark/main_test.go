//go:build linux && cgo

package main

import (
	"encoding/binary"
	"math"
	"testing"

	"golang.org/x/crypto/argon2"
)

func TestReusableOutputMatchesOfficialArgon2ID(t *testing.T) {
	resetReusableWorkspace()
	t.Cleanup(resetReusableWorkspace)
	input := make([]byte, 40)
	salt := []byte("RABBIT-LQC-WORK")
	for nonce := uint64(0); nonce < 16; nonce++ {
		binary.BigEndian.PutUint64(input[32:], nonce)
		var actual [32]byte
		if err := reusableArgon2IDInto(input, salt, 8*1024, &actual); err != nil {
			t.Fatal(err)
		}
		expected := argon2.IDKey(input, salt, 1, 8*1024, 1, 32)
		if string(actual[:]) != string(expected) {
			t.Fatalf("nonce %d diverged from official Argon2id", nonce)
		}
	}
}

func TestRobustStabilityClassifiesSingleHostOutlier(t *testing.T) {
	values := []float64{13.643, 15.901, 14.263, 218.441, 17.074, 18.025, 28.574}
	analysis := analyzeRoundStability(values)
	if len(analysis.OutlierRounds) != 1 || analysis.OutlierRounds[0] != 4 {
		t.Fatalf("outlier rounds = %v, want [4]", analysis.OutlierRounds)
	}
	if len(analysis.FasterOutlierRounds) != 0 || len(analysis.SlowerOutlierRounds) != 1 || analysis.SlowerOutlierRounds[0] != 4 {
		t.Fatalf("faster/slower outliers = %v/%v, want []/[4]", analysis.FasterOutlierRounds, analysis.SlowerOutlierRounds)
	}
	if math.Abs(analysis.RobustVariabilityPercent-30.32) > 0.1 {
		t.Fatalf("robust variability = %.2f%%, want about 30.32%%", analysis.RobustVariabilityPercent)
	}
}

func TestObservedHostVarianceHasStableCoreAndSafeTail(t *testing.T) {
	values := []float64{14.563, 11.818, 28.731, 15.031, 14.603, 15.048, 14.095, 33.734, 14.67, 16.009, 14.87, 13.018, 15.198, 14.152, 13.951}
	analysis := analyzeRoundStability(values)
	if len(analysis.FasterOutlierRounds) != 1 || analysis.FasterOutlierRounds[0] != 2 {
		t.Fatalf("faster outliers = %v, want [2]", analysis.FasterOutlierRounds)
	}
	if len(analysis.SlowerOutlierRounds) != 2 || analysis.SlowerOutlierRounds[0] != 3 || analysis.SlowerOutlierRounds[1] != 8 {
		t.Fatalf("slower outliers = %v, want [3 8]", analysis.SlowerOutlierRounds)
	}
	if math.Abs(analysis.RobustVariabilityPercent-4.91) > 0.1 {
		t.Fatalf("robust variability = %.2f%%, want about 4.91%%", analysis.RobustVariabilityPercent)
	}
	worst, weakWorst, safe := evaluateTailSafety(values, 4, 1000)
	if !safe || math.Abs(worst-33.734) > 0.001 || math.Abs(weakWorst-134.936) > 0.001 {
		t.Fatalf("tail worst/weak/safe = %.3f/%.3f/%t", worst, weakWorst, safe)
	}
}

func TestTailSafetyRejectsWorstRoundOutsideWeakBudget(t *testing.T) {
	_, weakWorst, safe := evaluateTailSafety([]float64{14, 15, 300}, 4, 1000)
	if safe || weakWorst != 1200 {
		t.Fatalf("weak tail = %.3f safe=%t, want 1200 and false", weakWorst, safe)
	}
}

func TestWorkspaceIsReused(t *testing.T) {
	resetReusableWorkspace()
	t.Cleanup(resetReusableWorkspace)
	input := make([]byte, 40)
	salt := []byte("RABBIT-LQC-WORK")
	var output [32]byte
	for nonce := uint64(0); nonce < 10; nonce++ {
		binary.BigEndian.PutUint64(input[32:], nonce)
		if err := reusableArgon2IDInto(input, salt, 8*1024, &output); err != nil {
			t.Fatal(err)
		}
	}
	bytes, allocations := reusableWorkspaceStats()
	if bytes != 8*1024*1024 || allocations != 1 {
		t.Fatalf("workspace bytes=%d allocations=%d, want 8388608 and 1", bytes, allocations)
	}
}
