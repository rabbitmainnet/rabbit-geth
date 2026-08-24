package main

import (
	"encoding/binary"
	"fmt"
	"math"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"
)

var benchmarkSink byte

func runBenchmark(opts options) (benchmarkReport, error) {
	if err := validateOptions(opts); err != nil {
		return benchmarkReport{}, err
	}
	memories, err := parseMemoryProfiles(opts.memoryProfiles)
	if err != nil {
		return benchmarkReport{}, err
	}

	report := benchmarkReport{
		BenchmarkVersion:           benchmarkVersion,
		GeneratedAt:                time.Now().UTC().Format(time.RFC3339),
		RuntimeOS:                  runtime.GOOS,
		RuntimeArchitecture:        runtime.GOARCH,
		RuntimeLogicalCPUs:         runtime.NumCPU(),
		GoVersion:                  runtime.Version(),
		ExecutionStatus:            "PASS",
		MeasurementStabilityStatus: "PASS",
		ContinuousStabilityStatus:  "PASS",
		IsolatedStabilityStatus:    "PASS",
		DiagnosticConclusion:       "STABLE_LOCAL_MEASUREMENT",
		PrototypeAlgorithm:         "Argon2id v1.3 (benchmark only)",
		CandidateProfileStatus:     "FAIL",
		LowEndAccessibilityStatus:  "PROVISIONAL",
		ImplementationStatus:       "BENCHMARK_ONLY",
		MainnetGate:                "BLOCKED",
		ConsensusChanged:           false,
		GenesisChanged:             false,
		WeakSlowdownFactor:         opts.weakSlowdown,
		EpochSeconds:               opts.epochSeconds,
		VerificationBudgetMs:       opts.verifyBudgetMs,
		Warnings: []string{
			"a measurement on one machine does not certify all low-end PCs",
			"Argon2id is only a measurable prototype and has not yet been selected for consensus",
			"GPU, ASIC, power consumption, and denial-of-service attacks require separate lab testing",
			"per-person equality remains impossible without an external identity source",
		},
	}

	roundMeasurements := make(map[uint64][]roundMeasurement, len(memories))
	for roundIndex := uint(0); roundIndex < opts.rounds; roundIndex++ {
		order := append([]uint64(nil), memories...)
		if roundIndex%2 == 1 {
			reverse(order)
		}
		for _, memory := range order {
			roundMeasurements[memory] = append(roundMeasurements[memory], benchmarkProfileRound(opts, memory))
		}
	}
	for _, memory := range memories {
		isolated := benchmarkIsolated(opts, memory)
		report.Profiles = append(report.Profiles, deriveProfile(opts, memory, roundMeasurements[memory], isolated))
	}
	continuousAnomalies := continuousMeasurementAnomalies(report.Profiles)
	isolatedAnomalies := isolatedMeasurementAnomalies(report.Profiles)
	for _, anomaly := range continuousAnomalies {
		report.MeasurementAnomalies = append(report.MeasurementAnomalies, "continuous: "+anomaly)
	}
	for _, anomaly := range isolatedAnomalies {
		report.MeasurementAnomalies = append(report.MeasurementAnomalies, "isolated: "+anomaly)
	}
	if len(continuousAnomalies) > 0 {
		report.ContinuousStabilityStatus = "FAIL"
	}
	if len(isolatedAnomalies) > 0 {
		report.IsolatedStabilityStatus = "FAIL"
	}
	if len(report.MeasurementAnomalies) > 0 {
		report.MeasurementStabilityStatus = "FAIL"
	}
	report.SelectedMemoryMiB = selectStableCandidateProfile(report.Profiles)
	if report.SelectedMemoryMiB > 0 {
		report.CandidateProfileStatus = "PASS"
		if len(report.MeasurementAnomalies) > 0 {
			report.MeasurementStabilityStatus = "PARTIAL"
			if report.ContinuousStabilityStatus == "FAIL" {
				report.ContinuousStabilityStatus = "PARTIAL"
			}
			if report.IsolatedStabilityStatus == "FAIL" {
				report.IsolatedStabilityStatus = "PARTIAL"
			}
			report.DiagnosticConclusion = "STABLE_ACCESSIBLE_PROFILE_WITH_REJECTED_LARGER_PROFILES"
		}
	} else if len(report.MeasurementAnomalies) > 0 {
		report.CandidateProfileStatus = "INCONCLUSIVE"
	}
	switch {
	case report.SelectedMemoryMiB > 0 && len(report.MeasurementAnomalies) > 0:
		// The selected profile is stable. Anomalies belong to rejected profiles.
	case len(continuousAnomalies) > 0 && len(isolatedAnomalies) == 0:
		report.DiagnosticConclusion = "GO_ALLOCATION_OR_GC_BOTTLENECK"
	case len(continuousAnomalies) > 0 && len(isolatedAnomalies) > 0:
		report.DiagnosticConclusion = "HOST_OR_PROTOTYPE_INSTABILITY"
	case len(continuousAnomalies) == 0 && len(isolatedAnomalies) > 0:
		report.DiagnosticConclusion = "ISOLATED_VERIFICATION_INSTABILITY"
	}
	return report, nil
}

func selectStableCandidateProfile(profiles []profileResult) uint64 {
	for _, profile := range profiles {
		if profile.ProvisionalQualification &&
			profile.RoundVariabilityPercent <= 35 &&
			profile.IsolatedVariabilityPercent <= 35 {
			// Profiles are sorted by memory. Prefer the smallest stable profile so
			// the local gate does not silently exclude low-end computers.
			return profile.MemoryMiB
		}
	}
	return 0
}

type roundMeasurement struct {
	samples   uint64
	averageMs float64
}

func benchmarkProfileRound(opts options, memoryMiB uint64) roundMeasurement {
	runtime.GC()
	for warmup := uint(0); warmup < opts.warmups; warmup++ {
		benchmarkAttempt(opts, memoryMiB, uint64(warmup))
	}
	started := time.Now()
	var samples uint64
	for time.Since(started) < opts.duration || samples < 3 {
		benchmarkAttempt(opts, memoryMiB, samples)
		samples++
	}
	elapsed := time.Since(started)
	averageMs := float64(elapsed.Microseconds()) / 1000 / float64(samples)
	return roundMeasurement{samples: samples, averageMs: averageMs}
}

func benchmarkAttempt(opts options, memoryMiB, nonce uint64) {
	var input [40]byte
	binary.BigEndian.PutUint64(input[32:], nonce)
	output := argon2.IDKey(
		input[:],
		[]byte("RABBIT-LQC-WORK"),
		uint32(opts.iterations),
		uint32(memoryMiB*1024),
		uint8(opts.parallelism),
		32,
	)
	benchmarkSink ^= output[0]
}

func benchmarkIsolated(opts options, memoryMiB uint64) []float64 {
	benchmarkAttempt(opts, memoryMiB, 0)
	measurements := make([]float64, 0, opts.isolatedSamples)
	for sample := uint(0); sample < opts.isolatedSamples; sample++ {
		runtime.GC()
		started := time.Now()
		benchmarkAttempt(opts, memoryMiB, uint64(sample+1))
		elapsed := time.Since(started)
		measurements = append(measurements, float64(elapsed.Microseconds())/1000)
	}
	return measurements
}

func deriveProfile(opts options, memoryMiB uint64, rounds []roundMeasurement, isolated []float64) profileResult {
	var samples uint64
	var weightedMs float64
	roundAverages := make([]float64, 0, len(rounds))
	for _, round := range rounds {
		samples += round.samples
		weightedMs += round.averageMs * float64(round.samples)
		roundAverages = append(roundAverages, round.averageMs)
	}
	averageMs := weightedMs / float64(samples)
	medianMs := percentile(roundAverages, 0.50)
	p95Ms := percentile(roundAverages, 0.95)
	variability := 100 * standardDeviation(roundAverages) / medianMs
	isolatedMedianMs := percentile(isolated, 0.50)
	isolatedP95Ms := percentile(isolated, 0.95)
	isolatedVariability := 100 * standardDeviation(isolated) / isolatedMedianMs
	weakOperationMs := p95Ms * opts.weakSlowdown
	weakAttemptsPerEpoch := float64(opts.epochSeconds) / (weakOperationMs / 1000)
	difficulty := uint64(math.Floor(weakAttemptsPerEpoch / -math.Log(1-opts.targetSuccess)))
	if difficulty == 0 {
		difficulty = 1
	}
	maxVerifications := uint64(math.Floor(opts.verifyBudgetMs / p95Ms))
	if maxVerifications > opts.maxTicketsBlock {
		maxVerifications = opts.maxTicketsBlock
	}
	localTicketSeconds := float64(difficulty) * p95Ms / 1000
	weakTicketSeconds := float64(difficulty) * weakOperationMs / 1000
	qualified := memoryMiB <= 128 && weakOperationMs <= 2000 && maxVerifications >= 8 && weakTicketSeconds <= float64(opts.epochSeconds)
	return profileResult{
		MemoryMiB:                     memoryMiB,
		Iterations:                    uint32(opts.iterations),
		Parallelism:                   uint8(opts.parallelism),
		Samples:                       samples,
		Rounds:                        uint(len(rounds)),
		RoundAverageOperationMs:       roundedValues(roundAverages, 3),
		AverageOperationMs:            round(averageMs, 3),
		MedianOperationMs:             round(medianMs, 3),
		P95OperationMs:                round(p95Ms, 3),
		RoundVariabilityPercent:       round(variability, 2),
		IsolatedSamples:               uint64(len(isolated)),
		IsolatedMedianOperationMs:     round(isolatedMedianMs, 3),
		IsolatedP95OperationMs:        round(isolatedP95Ms, 3),
		IsolatedVariabilityPercent:    round(isolatedVariability, 2),
		OperationsPerSecond:           round(1000/p95Ms, 2),
		EstimatedWeakOperationMs:      round(weakOperationMs, 3),
		MaxVerificationsWithinBudget:  maxVerifications,
		DerivedDifficulty:             difficulty,
		TargetSuccessPercent:          round(opts.targetSuccess*100, 2),
		EstimatedLocalTicketSeconds:   round(localTicketSeconds, 2),
		EstimatedWeakTicketSeconds:    round(weakTicketSeconds, 2),
		EstimatedWeak1000TicketsHours: round(weakTicketSeconds*1000/3600, 2),
		ProvisionalQualification:      qualified,
	}
}

func continuousMeasurementAnomalies(profiles []profileResult) []string {
	var anomalies []string
	for _, profile := range profiles {
		if profile.RoundVariabilityPercent > 35 {
			anomalies = append(anomalies, fmt.Sprintf("%d MiB varied by %.2f%% across rounds", profile.MemoryMiB, profile.RoundVariabilityPercent))
		}
	}
	for index := 1; index < len(profiles); index++ {
		previous := profiles[index-1]
		current := profiles[index]
		if current.MedianOperationMs < previous.MedianOperationMs*0.85 {
			anomalies = append(anomalies, fmt.Sprintf("%d MiB was unexpectedly faster than %d MiB", current.MemoryMiB, previous.MemoryMiB))
		}
	}
	return anomalies
}

func isolatedMeasurementAnomalies(profiles []profileResult) []string {
	var anomalies []string
	for _, profile := range profiles {
		if profile.IsolatedVariabilityPercent > 35 {
			anomalies = append(anomalies, fmt.Sprintf("%d MiB varied by %.2f%% across samples", profile.MemoryMiB, profile.IsolatedVariabilityPercent))
		}
	}
	for index := 1; index < len(profiles); index++ {
		previous := profiles[index-1]
		current := profiles[index]
		if current.IsolatedMedianOperationMs < previous.IsolatedMedianOperationMs*0.85 {
			anomalies = append(anomalies, fmt.Sprintf("%d MiB was unexpectedly faster than %d MiB", current.MemoryMiB, previous.MemoryMiB))
		}
	}
	return anomalies
}

func percentile(values []float64, quantile float64) float64 {
	ordered := append([]float64(nil), values...)
	sort.Float64s(ordered)
	index := int(math.Ceil(quantile*float64(len(ordered)))) - 1
	if index < 0 {
		index = 0
	}
	return ordered[index]
}

func standardDeviation(values []float64) float64 {
	var total float64
	for _, value := range values {
		total += value
	}
	mean := total / float64(len(values))
	var squares float64
	for _, value := range values {
		difference := value - mean
		squares += difference * difference
	}
	return math.Sqrt(squares / float64(len(values)))
}

func roundedValues(values []float64, places int) []float64 {
	result := make([]float64, len(values))
	for index, value := range values {
		result[index] = round(value, places)
	}
	return result
}

func reverse(values []uint64) {
	for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
		values[left], values[right] = values[right], values[left]
	}
}

func parseMemoryProfiles(value string) ([]uint64, error) {
	var values []uint64
	seen := make(map[uint64]bool)
	for _, field := range strings.Split(value, ",") {
		parsed, err := strconv.ParseUint(strings.TrimSpace(field), 10, 64)
		if err != nil || parsed == 0 || parsed > 1024 {
			return nil, fmt.Errorf("invalid memory profile %q", field)
		}
		if !seen[parsed] {
			seen[parsed] = true
			values = append(values, parsed)
		}
	}
	if len(values) == 0 {
		return nil, fmt.Errorf("empty memory list")
	}
	sort.Slice(values, func(left, right int) bool { return values[left] < values[right] })
	return values, nil
}

func round(value float64, places int) float64 {
	factor := math.Pow10(places)
	return math.Round(value*factor) / factor
}
