package main

import "time"

type options struct {
	memoryProfiles  string
	duration        time.Duration
	rounds          uint
	warmups         uint
	isolatedSamples uint
	iterations      uint
	parallelism     uint
	weakSlowdown    float64
	epochSeconds    uint64
	targetSuccess   float64
	verifyBudgetMs  float64
	maxTicketsBlock uint64
	outputDir       string
}

type profileResult struct {
	MemoryMiB                     uint64    `json:"memoryMiB"`
	Iterations                    uint32    `json:"iterations"`
	Parallelism                   uint8     `json:"parallelism"`
	Samples                       uint64    `json:"samples"`
	Rounds                        uint      `json:"rounds"`
	RoundAverageOperationMs       []float64 `json:"roundAverageOperationMs"`
	AverageOperationMs            float64   `json:"averageOperationMs"`
	MedianOperationMs             float64   `json:"medianOperationMs"`
	P95OperationMs                float64   `json:"p95OperationMs"`
	RoundVariabilityPercent       float64   `json:"roundVariabilityPercent"`
	IsolatedSamples               uint64    `json:"isolatedSamples"`
	IsolatedMedianOperationMs     float64   `json:"isolatedMedianOperationMs"`
	IsolatedP95OperationMs        float64   `json:"isolatedP95OperationMs"`
	IsolatedVariabilityPercent    float64   `json:"isolatedVariabilityPercent"`
	OperationsPerSecond           float64   `json:"operationsPerSecond"`
	EstimatedWeakOperationMs      float64   `json:"estimatedWeakOperationMs"`
	MaxVerificationsWithinBudget  uint64    `json:"maxVerificationsWithinBudget"`
	DerivedDifficulty             uint64    `json:"derivedDifficulty"`
	TargetSuccessPercent          float64   `json:"targetSuccessPercent"`
	EstimatedLocalTicketSeconds   float64   `json:"estimatedLocalTicketSeconds"`
	EstimatedWeakTicketSeconds    float64   `json:"estimatedWeakTicketSeconds"`
	EstimatedWeak1000TicketsHours float64   `json:"estimatedWeak1000TicketsHours"`
	ProvisionalQualification      bool      `json:"provisionalQualification"`
}

type benchmarkReport struct {
	BenchmarkVersion           string          `json:"benchmarkVersion"`
	GeneratedAt                string          `json:"generatedAt"`
	RuntimeOS                  string          `json:"runtimeOS"`
	RuntimeArchitecture        string          `json:"runtimeArchitecture"`
	RuntimeLogicalCPUs         int             `json:"runtimeLogicalCPUs"`
	GoVersion                  string          `json:"goVersion"`
	ExecutionStatus            string          `json:"executionStatus"`
	MeasurementStabilityStatus string          `json:"measurementStabilityStatus"`
	ContinuousStabilityStatus  string          `json:"continuousStabilityStatus"`
	IsolatedStabilityStatus    string          `json:"isolatedStabilityStatus"`
	DiagnosticConclusion       string          `json:"diagnosticConclusion"`
	PrototypeAlgorithm         string          `json:"prototypeAlgorithm"`
	CandidateProfileStatus     string          `json:"candidateProfileStatus"`
	LowEndAccessibilityStatus  string          `json:"lowEndAccessibilityStatus"`
	ImplementationStatus       string          `json:"implementationStatus"`
	MainnetGate                string          `json:"mainnetGate"`
	ConsensusChanged           bool            `json:"consensusChanged"`
	GenesisChanged             bool            `json:"genesisChanged"`
	WeakSlowdownFactor         float64         `json:"weakSlowdownFactor"`
	EpochSeconds               uint64          `json:"epochSeconds"`
	VerificationBudgetMs       float64         `json:"verificationBudgetMs"`
	SelectedMemoryMiB          uint64          `json:"selectedMemoryMiB"`
	Profiles                   []profileResult `json:"profiles"`
	MeasurementAnomalies       []string        `json:"measurementAnomalies"`
	Warnings                   []string        `json:"warnings"`
}
