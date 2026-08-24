package main

import (
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"
)

const (
	benchmarkVersion          = "rabbit-argon2-reuse-benchmark/1.0.2"
	modifiedZOutlierThreshold = 3.5
)

type options struct {
	memoryMiB      uint64
	duration       time.Duration
	rounds         uint
	warmups        uint
	weakSlowdown   float64
	verifyBudgetMs float64
	outputDir      string
}

type report struct {
	Version                   string    `json:"version"`
	GeneratedAt               string    `json:"generatedAt"`
	Runtime                   string    `json:"runtime"`
	MemoryMiB                 uint64    `json:"memoryMiB"`
	RoundP95Ms                []float64 `json:"roundP95Ms"`
	OverallP95Ms              float64   `json:"overallP95Ms"`
	VariabilityPercent        float64   `json:"variabilityPercent"`
	RobustVariabilityPercent  float64   `json:"robustVariabilityPercent"`
	RoundMedianMs             float64   `json:"roundMedianMs"`
	RoundMADMs                float64   `json:"roundMADMs"`
	OutlierRounds             []uint    `json:"outlierRounds"`
	OutlierCount              uint      `json:"outlierCount"`
	FasterOutlierRounds       []uint    `json:"fasterOutlierRounds"`
	SlowerOutlierRounds       []uint    `json:"slowerOutlierRounds"`
	StabilityMethod           string    `json:"stabilityMethod"`
	EstimatedWeakOperationMs  float64   `json:"estimatedWeakOperationMs"`
	WeakSlowdownFactor        float64   `json:"weakSlowdownFactor"`
	VerifyBudgetMs            float64   `json:"verifyBudgetMs"`
	WorstRoundP95Ms           float64   `json:"worstRoundP95Ms"`
	EstimatedWeakWorstRoundMs float64   `json:"estimatedWeakWorstRoundMs"`
	TailSafetyStatus          string    `json:"tailSafetyStatus"`
	MaxVerificationsPerSecond uint64    `json:"maxVerificationsPerSecond"`
	WorkspaceBytes            uint64    `json:"workspaceBytes"`
	WorkspaceAllocationCount  uint64    `json:"workspaceAllocationCount"`
	OfficialOutputMatches     bool      `json:"officialOutputMatches"`
	RepeatedOutputMatches     bool      `json:"repeatedOutputMatches"`
	ContinuousStabilityStatus string    `json:"continuousStabilityStatus"`
	CandidateStatus           string    `json:"candidateStatus"`
	ImplementationStatus      string    `json:"implementationStatus"`
	MainnetGate               string    `json:"mainnetGate"`
	ConsensusChanged          bool      `json:"consensusChanged"`
	GenesisChanged            bool      `json:"genesisChanged"`
}

func main() {
	opts := parseFlags()
	result, err := run(opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
		os.Exit(1)
	}
	printReport(result)
	if opts.outputDir != "" {
		if err := writeReport(opts.outputDir, result); err != nil {
			fmt.Fprintln(os.Stderr, "ERROR:", err)
			os.Exit(1)
		}
	}
}

func parseFlags() options {
	var opts options
	flag.Uint64Var(&opts.memoryMiB, "memory-mib", 8, "reusable memory in MiB")
	flag.DurationVar(&opts.duration, "duration", 750*time.Millisecond, "minimum duration of each round")
	flag.UintVar(&opts.rounds, "rounds", 7, "measurement rounds")
	flag.UintVar(&opts.warmups, "warmups", 2, "warm-ups before measurement")
	flag.Float64Var(&opts.weakSlowdown, "weak-slowdown", 4, "conservative slowdown factor for a low-end PC")
	flag.Float64Var(&opts.verifyBudgetMs, "verify-budget-ms", 1000, "verification budget per block")
	flag.StringVar(&opts.outputDir, "output", "", "report directory")
	flag.Parse()
	return opts
}

func run(opts options) (report, error) {
	if opts.memoryMiB == 0 || opts.memoryMiB > 128 || opts.duration <= 0 || opts.rounds < 3 || opts.rounds > 21 || opts.warmups == 0 {
		return report{}, fmt.Errorf("invalid parameters")
	}
	resetReusableWorkspace()
	input := make([]byte, 40)
	salt := []byte("RABBIT-LQC-WORK")
	var native, repeated [32]byte
	binary.BigEndian.PutUint64(input[32:], 42)
	if err := reusableArgon2IDInto(input, salt, uint32(opts.memoryMiB*1024), &native); err != nil {
		return report{}, err
	}
	if err := reusableArgon2IDInto(input, salt, uint32(opts.memoryMiB*1024), &repeated); err != nil {
		return report{}, err
	}
	official := argon2.IDKey(input, salt, 1, uint32(opts.memoryMiB*1024), 1, 32)
	officialMatches := string(native[:]) == string(official)
	repeatedMatches := native == repeated
	if !officialMatches || !repeatedMatches {
		return report{}, fmt.Errorf("reusable output diverged from official Argon2id")
	}

	var allSamples []float64
	var roundP95 []float64
	var nonce uint64
	for round := uint(0); round < opts.rounds; round++ {
		for warmup := uint(0); warmup < opts.warmups; warmup++ {
			nonce++
			binary.BigEndian.PutUint64(input[32:], nonce)
			if err := reusableArgon2IDInto(input, salt, uint32(opts.memoryMiB*1024), &native); err != nil {
				return report{}, err
			}
		}
		startedRound := time.Now()
		var samples []float64
		for time.Since(startedRound) < opts.duration || len(samples) < 5 {
			nonce++
			binary.BigEndian.PutUint64(input[32:], nonce)
			started := time.Now()
			if err := reusableArgon2IDInto(input, salt, uint32(opts.memoryMiB*1024), &native); err != nil {
				return report{}, err
			}
			samples = append(samples, float64(time.Since(started).Microseconds())/1000)
		}
		roundP95 = append(roundP95, percentile(samples, 0.95))
		allSamples = append(allSamples, samples...)
	}
	workspaceBytes, allocations := reusableWorkspaceStats()
	overallP95 := percentile(allSamples, 0.95)
	stability := analyzeRoundStability(roundP95)
	weakMs := overallP95 * opts.weakSlowdown
	worstRoundP95, weakWorstRoundMs, tailSafe := evaluateTailSafety(roundP95, opts.weakSlowdown, opts.verifyBudgetMs)
	maxVerifications := uint64(math.Floor(opts.verifyBudgetMs / overallP95))
	stable := stability.RobustVariabilityPercent <= 35 && tailSafe
	qualified := stable && officialMatches && repeatedMatches && allocations == 1 && workspaceBytes == opts.memoryMiB*1024*1024 && weakMs <= 2000 && maxVerifications >= 8
	status := "FAIL"
	if stable {
		status = "PASS"
	}
	tailStatus := "FAIL"
	if tailSafe {
		tailStatus = "PASS"
	}
	candidate := "INCONCLUSIVE"
	if qualified {
		candidate = "PASS"
	}
	return report{
		Version:                   benchmarkVersion,
		GeneratedAt:               time.Now().UTC().Format(time.RFC3339),
		Runtime:                   runtime.GOOS + "/" + runtime.GOARCH + " " + runtime.Version(),
		MemoryMiB:                 opts.memoryMiB,
		RoundP95Ms:                rounded(roundP95),
		OverallP95Ms:              round(overallP95, 3),
		VariabilityPercent:        round(stability.RawVariabilityPercent, 2),
		RobustVariabilityPercent:  round(stability.RobustVariabilityPercent, 2),
		RoundMedianMs:             round(stability.MedianMs, 3),
		RoundMADMs:                round(stability.MADMs, 3),
		OutlierRounds:             stability.OutlierRounds,
		OutlierCount:              uint(len(stability.OutlierRounds)),
		FasterOutlierRounds:       stability.FasterOutlierRounds,
		SlowerOutlierRounds:       stability.SlowerOutlierRounds,
		StabilityMethod:           "MEDIAN_MAD_WITH_ABSOLUTE_WEAK_TAIL_BUDGET",
		EstimatedWeakOperationMs:  round(weakMs, 3),
		WeakSlowdownFactor:        round(opts.weakSlowdown, 3),
		VerifyBudgetMs:            round(opts.verifyBudgetMs, 3),
		WorstRoundP95Ms:           round(worstRoundP95, 3),
		EstimatedWeakWorstRoundMs: round(weakWorstRoundMs, 3),
		TailSafetyStatus:          tailStatus,
		MaxVerificationsPerSecond: maxVerifications,
		WorkspaceBytes:            workspaceBytes,
		WorkspaceAllocationCount:  allocations,
		OfficialOutputMatches:     officialMatches,
		RepeatedOutputMatches:     repeatedMatches,
		ContinuousStabilityStatus: status,
		CandidateStatus:           candidate,
		ImplementationStatus:      "BENCHMARK_ONLY",
		MainnetGate:               "BLOCKED",
		ConsensusChanged:          false,
		GenesisChanged:            false,
	}, nil
}

type roundStability struct {
	RawVariabilityPercent    float64
	RobustVariabilityPercent float64
	MedianMs                 float64
	MADMs                    float64
	OutlierRounds            []uint
	FasterOutlierRounds      []uint
	SlowerOutlierRounds      []uint
}

func analyzeRoundStability(values []float64) roundStability {
	roundMedian := median(values)
	rawVariability := coefficientOfVariation(values, roundMedian)
	deviations := make([]float64, len(values))
	for index, value := range values {
		deviations[index] = math.Abs(value - roundMedian)
	}
	mad := median(deviations)
	var outliers []uint
	var fasterOutliers []uint
	var slowerOutliers []uint
	var inliers []float64
	for index, value := range values {
		modifiedZ := 0.0
		if mad > 0 {
			modifiedZ = 0.6745 * (value - roundMedian) / mad
		}
		if math.Abs(modifiedZ) > modifiedZOutlierThreshold {
			outliers = append(outliers, uint(index+1))
			if modifiedZ < 0 {
				fasterOutliers = append(fasterOutliers, uint(index+1))
			} else {
				slowerOutliers = append(slowerOutliers, uint(index+1))
			}
			continue
		}
		inliers = append(inliers, value)
	}
	robustVariability := coefficientOfVariation(inliers, median(inliers))
	return roundStability{
		RawVariabilityPercent:    rawVariability,
		RobustVariabilityPercent: robustVariability,
		MedianMs:                 roundMedian,
		MADMs:                    mad,
		OutlierRounds:            outliers,
		FasterOutlierRounds:      fasterOutliers,
		SlowerOutlierRounds:      slowerOutliers,
	}
}

func evaluateTailSafety(values []float64, weakSlowdown, budgetMs float64) (float64, float64, bool) {
	worst := 0.0
	for _, value := range values {
		if value > worst {
			worst = value
		}
	}
	weakWorst := worst * weakSlowdown
	return worst, weakWorst, weakWorst <= budgetMs
}

func median(values []float64) float64 {
	ordered := append([]float64(nil), values...)
	sort.Float64s(ordered)
	middle := len(ordered) / 2
	if len(ordered)%2 == 0 {
		return (ordered[middle-1] + ordered[middle]) / 2
	}
	return ordered[middle]
}

func coefficientOfVariation(values []float64, reference float64) float64 {
	if len(values) == 0 || reference <= 0 {
		return math.Inf(1)
	}
	return 100 * standardDeviation(values) / reference
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
	var squared float64
	for _, value := range values {
		delta := value - mean
		squared += delta * delta
	}
	return math.Sqrt(squared / float64(len(values)))
}

func round(value float64, places int) float64 {
	factor := math.Pow10(places)
	return math.Round(value*factor) / factor
}

func rounded(values []float64) []float64 {
	result := make([]float64, len(values))
	for index, value := range values {
		result[index] = round(value, 3)
	}
	return result
}

func printReport(result report) {
	fmt.Println("Rabbit Chain — reusable-memory Argon2id benchmark")
	fmt.Println("Output identical to official implementation:", result.OfficialOutputMatches)
	fmt.Println("Deterministic repetition:", result.RepeatedOutputMatches)
	fmt.Printf("Workspace: %d bytes | allocations: %d\n", result.WorkspaceBytes, result.WorkspaceAllocationCount)
	fmt.Printf("P95: %.3f ms | variability: %.2f%% | low-end PC: %.3f ms\n", result.OverallP95Ms, result.VariabilityPercent, result.EstimatedWeakOperationMs)
	fmt.Printf("Round median: %.3f ms | MAD: %.3f ms | robust variability: %.2f%%\n", result.RoundMedianMs, result.RoundMADMs, result.RobustVariabilityPercent)
	fmt.Printf("Auditable outliers: %v | faster: %v | slower: %v\n", result.OutlierRounds, result.FasterOutlierRounds, result.SlowerOutlierRounds)
	fmt.Printf("Worst-round P95: %.3f ms | low-end PC: %.3f ms | budget %.3f ms: %s\n", result.WorstRoundP95Ms, result.EstimatedWeakWorstRoundMs, result.VerifyBudgetMs, result.TailSafetyStatus)
	fmt.Println("Continuous stability:", result.ContinuousStabilityStatus)
	fmt.Println("Reusable profile:", result.CandidateStatus)
	fmt.Println("Implementation:", result.ImplementationStatus)
	fmt.Println("Mainnet gate:", result.MainnetGate)
	fmt.Println("RESULT: allocation diagnostics completed; consensus and genesis unchanged.")
}

func writeReport(directory string, result report) error {
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	blob, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(directory, "relatorio.json"), append(blob, '\n'), 0o644); err != nil {
		return err
	}
	var summary strings.Builder
	fmt.Fprintln(&summary, "# Argon2id with reusable memory — Rabbit Chain")
	fmt.Fprintln(&summary)
	fmt.Fprintf(&summary, "- Output identical to `golang.org/x/crypto/argon2`: **%t**\n", result.OfficialOutputMatches)
	fmt.Fprintf(&summary, "- Deterministic repetition: **%t**\n", result.RepeatedOutputMatches)
	fmt.Fprintf(&summary, "- Workspace: **%d bytes; %d allocation**\n", result.WorkspaceBytes, result.WorkspaceAllocationCount)
	fmt.Fprintf(&summary, "- P95: **%.3f ms**\n", result.OverallP95Ms)
	fmt.Fprintf(&summary, "- Variability: **%.2f%%**\n", result.VariabilityPercent)
	fmt.Fprintf(&summary, "- Robust variability after MAD classification: **%.2f%%**\n", result.RobustVariabilityPercent)
	fmt.Fprintf(&summary, "- Round median/MAD: **%.3f / %.3f ms**\n", result.RoundMedianMs, result.RoundMADMs)
	fmt.Fprintf(&summary, "- Auditable outliers: **%v**\n", result.OutlierRounds)
	fmt.Fprintf(&summary, "- Faster/slower outliers: **%v / %v**\n", result.FasterOutlierRounds, result.SlowerOutlierRounds)
	fmt.Fprintf(&summary, "- Worst-round P95: **%.3f ms; %.3f ms on the estimated low-end PC**\n", result.WorstRoundP95Ms, result.EstimatedWeakWorstRoundMs)
	fmt.Fprintf(&summary, "- Low-end PC factor/budget: **%.3fx / %.3f ms**\n", result.WeakSlowdownFactor, result.VerifyBudgetMs)
	fmt.Fprintf(&summary, "- Tail safety within budget: **%s**\n", result.TailSafetyStatus)
	fmt.Fprintf(&summary, "- Stability method: **%s**\n", result.StabilityMethod)
	fmt.Fprintf(&summary, "- Stability: **%s**\n", result.ContinuousStabilityStatus)
	fmt.Fprintf(&summary, "- Reusable profile: **%s**\n", result.CandidateStatus)
	fmt.Fprintln(&summary)
	fmt.Fprintln(&summary, "This code is only a Linux/cgo benchmark. It is not an approved consensus implementation and does not unblock mainnet.")
	return os.WriteFile(filepath.Join(directory, "resumo.md"), []byte(summary.String()), 0o644)
}
