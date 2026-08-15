package main

import (
	"math"
	"testing"
	"time"
)

func testBenchmarkOptions() options {
	return options{
		memoryProfiles:  "1,2",
		duration:        10 * time.Millisecond,
		rounds:          3,
		warmups:         1,
		isolatedSamples: 5,
		iterations:      1,
		parallelism:     1,
		weakSlowdown:    4,
		epochSeconds:    1280,
		targetSuccess:   0.80,
		verifyBudgetMs:  1000,
		maxTicketsBlock: 64,
	}
}

func TestDerivedDifficultyTargetsWeakPCProbability(t *testing.T) {
	opts := testBenchmarkOptions()
	profile := deriveProfile(opts, 16, []roundMeasurement{{samples: 10, averageMs: 100}, {samples: 10, averageMs: 100}, {samples: 10, averageMs: 100}}, []float64{100, 100, 100, 100, 100})
	weakAttempts := float64(opts.epochSeconds) / (profile.EstimatedWeakOperationMs / 1000)
	probability := 1 - math.Exp(-weakAttempts/float64(profile.DerivedDifficulty))
	if math.Abs(probability-opts.targetSuccess) > 0.01 {
		t.Fatalf("probability %.4f, want %.4f", probability, opts.targetSuccess)
	}
	if profile.EstimatedWeak1000TicketsHours < 100 {
		t.Fatalf("1000 tickets are unexpectedly cheap: %+v", profile)
	}
}

func TestMeasurementAnomaliesRejectNoisyOrInvertedProfiles(t *testing.T) {
	profiles := []profileResult{
		{MemoryMiB: 8, MedianOperationMs: 20, RoundVariabilityPercent: 10},
		{MemoryMiB: 16, MedianOperationMs: 100, RoundVariabilityPercent: 40},
		{MemoryMiB: 32, MedianOperationMs: 50, RoundVariabilityPercent: 10},
	}
	if anomalies := continuousMeasurementAnomalies(profiles); len(anomalies) != 2 {
		t.Fatalf("anomalies = %v, want variability and inversion", anomalies)
	}
}

func TestStableIsolatedMeasurementIdentifiesContinuousBottleneck(t *testing.T) {
	profiles := []profileResult{
		{MemoryMiB: 8, MedianOperationMs: 20, RoundVariabilityPercent: 80, IsolatedMedianOperationMs: 18, IsolatedVariabilityPercent: 5},
		{MemoryMiB: 16, MedianOperationMs: 40, RoundVariabilityPercent: 90, IsolatedMedianOperationMs: 35, IsolatedVariabilityPercent: 6},
	}
	if len(continuousMeasurementAnomalies(profiles)) == 0 {
		t.Fatal("expected continuous instability")
	}
	if anomalies := isolatedMeasurementAnomalies(profiles); len(anomalies) != 0 {
		t.Fatalf("unexpected isolated anomalies: %v", anomalies)
	}
}

func TestCandidateSelectionKeepsStableAccessibleProfile(t *testing.T) {
	profiles := []profileResult{
		{MemoryMiB: 8, ProvisionalQualification: true, RoundVariabilityPercent: 15.46, IsolatedVariabilityPercent: 22.20},
		{MemoryMiB: 16, ProvisionalQualification: false, RoundVariabilityPercent: 68.21, IsolatedVariabilityPercent: 62.82},
		{MemoryMiB: 32, ProvisionalQualification: false, RoundVariabilityPercent: 115.71, IsolatedVariabilityPercent: 42.54},
	}
	if selected := selectStableCandidateProfile(profiles); selected != 8 {
		t.Fatalf("selected = %d MiB, want 8 MiB", selected)
	}
}

func TestCandidateSelectionRejectsUnstableQualifiedProfile(t *testing.T) {
	profiles := []profileResult{
		{MemoryMiB: 8, ProvisionalQualification: true, RoundVariabilityPercent: 40, IsolatedVariabilityPercent: 10},
	}
	if selected := selectStableCandidateProfile(profiles); selected != 0 {
		t.Fatalf("selected = %d MiB, want none", selected)
	}
}

func TestBenchmarkRemainsProvisionalAndBlocked(t *testing.T) {
	report, err := runBenchmark(testBenchmarkOptions())
	if err != nil {
		t.Fatal(err)
	}
	if report.ExecutionStatus != "PASS" || report.LowEndAccessibilityStatus != "PROVISIONAL" {
		t.Fatalf("unsafe status: %+v", report)
	}
	if report.ImplementationStatus != "BENCHMARK_ONLY" || report.MainnetGate != "BLOCKED" {
		t.Fatalf("unsafe launch status: %+v", report)
	}
	if report.ConsensusChanged || report.GenesisChanged {
		t.Fatalf("benchmark must not mutate consensus: %+v", report)
	}
}

func TestMemoryProfilesRejectUnsafeInput(t *testing.T) {
	if _, err := parseMemoryProfiles("16,0,64"); err == nil {
		t.Fatal("expected invalid zero memory")
	}
}
