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
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/argon2"
)

const benchmarkVersion = "rabbit-argon2-batch-benchmark/1.1.0"

type options struct {
	memoryMiB          uint64
	batchSizes         string
	rounds             uint
	warmupOperations   uint64
	workers            uint64
	weakSlowdown       float64
	targetBlockTimeMs  float64
	verificationBudget float64
	outputDir          string
}

type batchProfile struct {
	BatchSize                  uint64    `json:"batchSize"`
	RoundDurationsMs           []float64 `json:"roundDurationsMs"`
	MedianMs                   float64   `json:"medianMs"`
	MADMs                      float64   `json:"madMs"`
	RobustP95Ms                float64   `json:"robustP95Ms"`
	RobustVariabilityPercent   float64   `json:"robustVariabilityPercent"`
	OutlierRounds              []uint    `json:"outlierRounds"`
	WorstRoundMs               float64   `json:"worstRoundMs"`
	EstimatedWeakRobustP95Ms   float64   `json:"estimatedWeakRobustP95Ms"`
	EstimatedWeakWorstRoundMs  float64   `json:"estimatedWeakWorstRoundMs"`
	VerificationBudgetShareBps uint64    `json:"verificationBudgetShareBps"`
	TargetBlockWorstShareBps   uint64    `json:"targetBlockWorstShareBps"`
	StableCoreStatus           string    `json:"stableCoreStatus"`
	VerificationBudgetStatus   string    `json:"verificationBudgetStatus"`
	WorstRoundBlockTimeStatus  string    `json:"worstRoundBlockTimeStatus"`
	ProfileStatus              string    `json:"profileStatus"`
}

type report struct {
	Version                  string         `json:"version"`
	GeneratedAt              string         `json:"generatedAt"`
	Runtime                  string         `json:"runtime"`
	MemoryMiB                uint64         `json:"memoryMiB"`
	BatchSizes               []uint64       `json:"batchSizes"`
	Rounds                   uint           `json:"rounds"`
	WarmupOperations         uint64         `json:"warmupOperations"`
	WorkerCount              uint64         `json:"workerCount"`
	WeakSlowdownFactor       float64        `json:"weakSlowdownFactor"`
	TargetBlockTimeMs        float64        `json:"targetBlockTimeMs"`
	VerificationBudgetMs     float64        `json:"verificationBudgetMs"`
	OfficialOutputMatches    bool           `json:"officialOutputMatches"`
	RepeatedOutputMatches    bool           `json:"repeatedOutputMatches"`
	WorkspaceBytes           uint64         `json:"workspaceBytes"`
	WorkspaceAllocationCount uint64         `json:"workspaceAllocationCount"`
	Profiles                 []batchProfile `json:"profiles"`
	OverallStatus            string         `json:"overallStatus"`
	ImplementationStatus     string         `json:"implementationStatus"`
	MainnetGate              string         `json:"mainnetGate"`
	ConsensusChanged         bool           `json:"consensusChanged"`
	GenesisChanged           bool           `json:"genesisChanged"`
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
	flag.StringVar(&opts.batchSizes, "batch-sizes", "1,8,16,32,64", "comma-separated batch sizes")
	flag.UintVar(&opts.rounds, "rounds", 21, "rounds per batch")
	flag.Uint64Var(&opts.warmupOperations, "warmup-operations", 64, "identical warm-up operations before each profile")
	flag.Uint64Var(&opts.workers, "workers", 2, "independent verification workers")
	flag.Float64Var(&opts.weakSlowdown, "weak-slowdown", 4, "conservative slowdown factor for a low-end PC")
	flag.Float64Var(&opts.targetBlockTimeMs, "target-block-time-ms", 10000, "frozen target block time")
	flag.Float64Var(&opts.verificationBudget, "verification-budget-ms", 5000, "budget reserved for verifications")
	flag.StringVar(&opts.outputDir, "output", "", "report directory")
	flag.Parse()
	return opts
}

func run(opts options) (report, error) {
	batchSizes, err := parseBatchSizes(opts.batchSizes)
	if err != nil {
		return report{}, err
	}
	if opts.memoryMiB == 0 || opts.memoryMiB > 128 || opts.rounds < 3 || opts.rounds > 41 || opts.warmupOperations == 0 || opts.warmupOperations > 4096 || opts.workers == 0 || opts.workers > 8 || opts.weakSlowdown < 1 || opts.targetBlockTimeMs <= 0 || opts.verificationBudget <= 0 || opts.verificationBudget > opts.targetBlockTimeMs {
		return report{}, fmt.Errorf("invalid parameters")
	}
	resetReusableWorkspace()
	input := make([]byte, 40)
	salt := []byte("RABBIT-LQC-WORK")
	var native, repeated [32]byte
	binary.BigEndian.PutUint64(input[32:], 42)
	for worker := uint64(0); worker < opts.workers; worker++ {
		var candidate [32]byte
		if err := reusableArgon2IDInto(uint32(worker), input, salt, uint32(opts.memoryMiB*1024), &candidate); err != nil {
			return report{}, err
		}
		if worker == 0 {
			native = candidate
		}
	}
	if err := reusableArgon2IDInto(0, input, salt, uint32(opts.memoryMiB*1024), &repeated); err != nil {
		return report{}, err
	}
	official := argon2.IDKey(input, salt, 1, uint32(opts.memoryMiB*1024), 1, 32)
	officialMatches := string(native[:]) == string(official)
	repeatedMatches := native == repeated
	if !officialMatches || !repeatedMatches {
		return report{}, fmt.Errorf("reusable output diverged from official Argon2id")
	}

	nonce := uint64(100)
	profiles := make([]batchProfile, 0, len(batchSizes))
	overallPass := true
	for _, batchSize := range batchSizes {
		if err := executeBatch(input, salt, uint32(opts.memoryMiB*1024), opts.warmupOperations, opts.workers, &nonce); err != nil {
			return report{}, err
		}
		durations := make([]float64, 0, opts.rounds)
		for roundIndex := uint(0); roundIndex < opts.rounds; roundIndex++ {
			started := time.Now()
			if err := executeBatch(input, salt, uint32(opts.memoryMiB*1024), batchSize, opts.workers, &nonce); err != nil {
				return report{}, err
			}
			durations = append(durations, float64(time.Since(started).Microseconds())/1000)
		}
		profile := analyzeBatchProfile(batchSize, durations, opts)
		if profile.ProfileStatus != "PASS" {
			overallPass = false
		}
		profiles = append(profiles, profile)
	}
	workspaceBytes, allocations := reusableWorkspaceStats()
	if workspaceBytes != opts.workers*opts.memoryMiB*1024*1024 || allocations != opts.workers {
		overallPass = false
	}
	overallStatus := "FAIL"
	if overallPass {
		overallStatus = "PASS"
	}
	return report{
		Version:                  benchmarkVersion,
		GeneratedAt:              time.Now().UTC().Format(time.RFC3339),
		Runtime:                  runtime.GOOS + "/" + runtime.GOARCH + " " + runtime.Version(),
		MemoryMiB:                opts.memoryMiB,
		BatchSizes:               batchSizes,
		Rounds:                   opts.rounds,
		WarmupOperations:         opts.warmupOperations,
		WorkerCount:              opts.workers,
		WeakSlowdownFactor:       opts.weakSlowdown,
		TargetBlockTimeMs:        opts.targetBlockTimeMs,
		VerificationBudgetMs:     opts.verificationBudget,
		OfficialOutputMatches:    officialMatches,
		RepeatedOutputMatches:    repeatedMatches,
		WorkspaceBytes:           workspaceBytes,
		WorkspaceAllocationCount: allocations,
		Profiles:                 profiles,
		OverallStatus:            overallStatus,
		ImplementationStatus:     "BENCHMARK_ONLY",
		MainnetGate:              "BLOCKED",
		ConsensusChanged:         false,
		GenesisChanged:           false,
	}, nil
}

func executeBatch(input, salt []byte, memoryKiB uint32, size, workers uint64, nonce *uint64) error {
	baseNonce := *nonce
	*nonce += size
	errCh := make(chan error, workers)
	var wait sync.WaitGroup
	for worker := uint64(0); worker < workers; worker++ {
		wait.Add(1)
		go func(worker uint64) {
			defer wait.Done()
			localInput := append([]byte(nil), input...)
			var output [32]byte
			for operation := worker; operation < size; operation += workers {
				binary.BigEndian.PutUint64(localInput[32:], baseNonce+operation+1)
				if err := reusableArgon2IDInto(uint32(worker), localInput, salt, memoryKiB, &output); err != nil {
					errCh <- err
					return
				}
			}
		}(worker)
	}
	wait.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			return err
		}
	}
	return nil
}

func parseBatchSizes(raw string) ([]uint64, error) {
	parts := strings.Split(raw, ",")
	seen := make(map[uint64]struct{})
	result := make([]uint64, 0, len(parts))
	for _, part := range parts {
		value, err := strconv.ParseUint(strings.TrimSpace(part), 10, 64)
		if err != nil || value == 0 || value > 64 {
			return nil, fmt.Errorf("invalid batch size %q", part)
		}
		if _, exists := seen[value]; exists {
			return nil, fmt.Errorf("duplicate batch size %d", value)
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("at least one batch size is required")
	}
	return result, nil
}

func analyzeBatchProfile(batchSize uint64, durations []float64, opts options) batchProfile {
	medianValue := median(durations)
	deviations := make([]float64, len(durations))
	for index, value := range durations {
		deviations[index] = math.Abs(value - medianValue)
	}
	mad := median(deviations)
	var outliers []uint
	var inliers []float64
	for index, value := range durations {
		modifiedZ := 0.0
		if mad > 0 {
			modifiedZ = 0.6745 * (value - medianValue) / mad
		}
		if math.Abs(modifiedZ) > 3.5 {
			outliers = append(outliers, uint(index+1))
			continue
		}
		inliers = append(inliers, value)
	}
	robustP95 := percentile(inliers, 0.95)
	robustVariability := coefficientOfVariation(inliers, median(inliers))
	worst := maximum(durations)
	weakRobustP95 := robustP95 * opts.weakSlowdown
	weakWorst := worst * opts.weakSlowdown
	stableCore := robustVariability <= 35
	budgetSafe := weakRobustP95 <= opts.verificationBudget
	worstSafe := weakWorst <= opts.targetBlockTimeMs
	status := "FAIL"
	if stableCore && budgetSafe && worstSafe {
		status = "PASS"
	}
	return batchProfile{
		BatchSize:                  batchSize,
		RoundDurationsMs:           rounded(durations),
		MedianMs:                   round(medianValue, 3),
		MADMs:                      round(mad, 3),
		RobustP95Ms:                round(robustP95, 3),
		RobustVariabilityPercent:   round(robustVariability, 2),
		OutlierRounds:              outliers,
		WorstRoundMs:               round(worst, 3),
		EstimatedWeakRobustP95Ms:   round(weakRobustP95, 3),
		EstimatedWeakWorstRoundMs:  round(weakWorst, 3),
		VerificationBudgetShareBps: ratioBps(weakRobustP95, opts.verificationBudget),
		TargetBlockWorstShareBps:   ratioBps(weakWorst, opts.targetBlockTimeMs),
		StableCoreStatus:           passFail(stableCore),
		VerificationBudgetStatus:   passFail(budgetSafe),
		WorstRoundBlockTimeStatus:  passFail(worstSafe),
		ProfileStatus:              status,
	}
}

func passFail(ok bool) string {
	if ok {
		return "PASS"
	}
	return "FAIL"
}

func ratioBps(value, reference float64) uint64 {
	if reference <= 0 {
		return ^uint64(0)
	}
	return uint64(math.Round(10000 * value / reference))
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

func percentile(values []float64, quantile float64) float64 {
	ordered := append([]float64(nil), values...)
	sort.Float64s(ordered)
	index := int(math.Ceil(quantile*float64(len(ordered)))) - 1
	if index < 0 {
		index = 0
	}
	return ordered[index]
}

func coefficientOfVariation(values []float64, reference float64) float64 {
	if len(values) == 0 || reference <= 0 {
		return math.Inf(1)
	}
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
	return 100 * math.Sqrt(squared/float64(len(values))) / reference
}

func maximum(values []float64) float64 {
	result := 0.0
	for _, value := range values {
		if value > result {
			result = value
		}
	}
	return result
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
	fmt.Println("Rabbit Chain — reusable Argon2id batch capacity")
	fmt.Printf("Configuration: %d MiB/workspace | %d workers | warm-up %d operations/profile | low-end PC %.1fx | block %.0f ms | budget %.0f ms\n", result.MemoryMiB, result.WorkerCount, result.WarmupOperations, result.WeakSlowdownFactor, result.TargetBlockTimeMs, result.VerificationBudgetMs)
	for _, profile := range result.Profiles {
		fmt.Printf("Batch %2d: robust P95 %.3f ms | low-end PC %.3f ms | worst low-end PC %.3f ms | core %s | budget %s | block %s | %s\n",
			profile.BatchSize, profile.RobustP95Ms, profile.EstimatedWeakRobustP95Ms, profile.EstimatedWeakWorstRoundMs,
			profile.StableCoreStatus, profile.VerificationBudgetStatus, profile.WorstRoundBlockTimeStatus, profile.ProfileStatus)
	}
	fmt.Printf("Workspace: %d bytes | allocations: %d | official equivalence: %t\n", result.WorkspaceBytes, result.WorkspaceAllocationCount, result.OfficialOutputMatches)
	fmt.Println("Each proof remains sequential; only independent proofs are verified in parallel.")
	fmt.Println("Overall status:", result.OverallStatus)
	fmt.Println("Implementation:", result.ImplementationStatus)
	fmt.Println("Mainnet gate:", result.MainnetGate)
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
	fmt.Fprintln(&summary, "# Reusable Argon2id batch capacity — Rabbit Chain")
	fmt.Fprintln(&summary)
	fmt.Fprintf(&summary, "- Overall result: **%s**\n", result.OverallStatus)
	fmt.Fprintf(&summary, "- Memory/low-end factor: **%d MiB / %.1fx**\n", result.MemoryMiB, result.WeakSlowdownFactor)
	fmt.Fprintf(&summary, "- Uniform warm-up: **%d operations before each profile**\n", result.WarmupOperations)
	fmt.Fprintf(&summary, "- Parallel verification: **%d workers; %d MiB total**\n", result.WorkerCount, result.WorkerCount*result.MemoryMiB)
	fmt.Fprintf(&summary, "- Block/verification budget: **%.0f / %.0f ms**\n", result.TargetBlockTimeMs, result.VerificationBudgetMs)
	for _, profile := range result.Profiles {
		fmt.Fprintf(&summary, "- Batch %d: **%s** — low-end P95 %.3f ms; worst %.3f ms; outliers %v\n", profile.BatchSize, profile.ProfileStatus, profile.EstimatedWeakRobustP95Ms, profile.EstimatedWeakWorstRoundMs, profile.OutlierRounds)
	}
	fmt.Fprintln(&summary)
	fmt.Fprintln(&summary, "Each Argon2id proof remains sequential; only independent proofs are verified in parallel. The benchmark does not implement consensus tickets or unblock mainnet.")
	return os.WriteFile(filepath.Join(directory, "resumo.md"), []byte(summary.String()), 0o644)
}
