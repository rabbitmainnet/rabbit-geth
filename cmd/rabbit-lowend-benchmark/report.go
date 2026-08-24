package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func printSummary(report benchmarkReport) {
	fmt.Println("Rabbit Chain — low-end PC accessibility benchmark")
	fmt.Println("Execution:", report.ExecutionStatus)
	fmt.Println("Measurement stability:", report.MeasurementStabilityStatus)
	fmt.Println("Continuous stability:", report.ContinuousStabilityStatus)
	fmt.Println("Isolated stability:", report.IsolatedStabilityStatus)
	fmt.Println("Diagnostics:", report.DiagnosticConclusion)
	fmt.Println("Prototype algorithm:", report.PrototypeAlgorithm)
	fmt.Println("Perfil local encontrado:", report.CandidateProfileStatus)
	if report.SelectedMemoryMiB > 0 {
		fmt.Printf("Perfil local selecionado: %d MiB\n", report.SelectedMemoryMiB)
	}
	fmt.Println("Overall accessibility:", report.LowEndAccessibilityStatus)
	fmt.Println("Implementation:", report.ImplementationStatus)
	fmt.Println("Mainnet gate:", report.MainnetGate)
	for _, profile := range report.Profiles {
		fmt.Printf("%d MiB: continuous p95 %.3f ms/var %.2f%% | isolated p95 %.3f ms/var %.2f%% | low-end PC %.3f ms | %d verifications/block | qualifies=%t\n",
			profile.MemoryMiB,
			profile.P95OperationMs,
			profile.RoundVariabilityPercent,
			profile.IsolatedP95OperationMs,
			profile.IsolatedVariabilityPercent,
			profile.EstimatedWeakOperationMs,
			profile.MaxVerificationsWithinBudget,
			profile.ProvisionalQualification,
		)
	}
	for _, anomaly := range report.MeasurementAnomalies {
		fmt.Println("ANOMALIA:", anomaly)
	}
	fmt.Println("RESULT: measurement completed; no algorithm was enabled in consensus.")
}

func writeReports(directory string, report benchmarkReport) error {
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	blob, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(directory, "relatorio.json"), append(blob, '\n'), 0o644); err != nil {
		return err
	}
	var text strings.Builder
	fmt.Fprintln(&text, "# Accessibility benchmark — Rabbit Chain")
	fmt.Fprintln(&text)
	fmt.Fprintf(&text, "- Environment: **%s/%s, %d logical CPUs, %s**\n", report.RuntimeOS, report.RuntimeArchitecture, report.RuntimeLogicalCPUs, report.GoVersion)
	fmt.Fprintf(&text, "- Execution: **%s**\n", report.ExecutionStatus)
	fmt.Fprintf(&text, "- Measurement stability: **%s**\n", report.MeasurementStabilityStatus)
	fmt.Fprintf(&text, "- Continuous stability: **%s**\n", report.ContinuousStabilityStatus)
	fmt.Fprintf(&text, "- Isolated stability: **%s**\n", report.IsolatedStabilityStatus)
	fmt.Fprintf(&text, "- Diagnostics: **%s**\n", report.DiagnosticConclusion)
	fmt.Fprintf(&text, "- Prototype algorithm: **%s**\n", report.PrototypeAlgorithm)
	fmt.Fprintf(&text, "- Perfil local: **%s**\n", report.CandidateProfileStatus)
	if report.SelectedMemoryMiB > 0 {
		fmt.Fprintf(&text, "- Perfil local selecionado: **%d MiB**\n", report.SelectedMemoryMiB)
	}
	fmt.Fprintf(&text, "- Accessibility for all low-end PCs: **%s**\n", report.LowEndAccessibilityStatus)
	fmt.Fprintf(&text, "- Implementation: **%s**\n", report.ImplementationStatus)
	fmt.Fprintf(&text, "- Mainnet: **%s**\n", report.MainnetGate)
	fmt.Fprintln(&text)
	fmt.Fprintln(&text, "| Memory | Continuous p95 | Continuous var. | Isolated p95 | Isolated var. | low-end PC ms | Verifications | Difficulty | Ticket low-end PC | 1000 tickets | Qualifies |")
	fmt.Fprintln(&text, "|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|:---:|")
	for _, profile := range report.Profiles {
		fmt.Fprintf(&text, "| %d MiB | %.3f | %.2f%% | %.3f | %.2f%% | %.3f | %d | %d | %.2f s | %.2f h | %t |\n",
			profile.MemoryMiB,
			profile.P95OperationMs,
			profile.RoundVariabilityPercent,
			profile.IsolatedP95OperationMs,
			profile.IsolatedVariabilityPercent,
			profile.EstimatedWeakOperationMs,
			profile.MaxVerificationsWithinBudget,
			profile.DerivedDifficulty,
			profile.EstimatedWeakTicketSeconds,
			profile.EstimatedWeak1000TicketsHours,
			profile.ProvisionalQualification,
		)
	}
	if len(report.MeasurementAnomalies) > 0 {
		fmt.Fprintln(&text)
		fmt.Fprintln(&text, "## Measurement anomalies")
		fmt.Fprintln(&text)
		for _, anomaly := range report.MeasurementAnomalies {
			fmt.Fprintf(&text, "- %s\n", anomaly)
		}
	}
	fmt.Fprintln(&text)
	fmt.Fprintln(&text, "This result measures only the local machine and projects performance for a PC four times slower. Additional physical benchmarks are required before selecting the algorithm.")
	fmt.Fprintln(&text)
	fmt.Fprintln(&text, "**Consensus and genesis remain unchanged; mainnet remains blocked.**")
	return os.WriteFile(filepath.Join(directory, "resumo.md"), []byte(text.String()), 0o644)
}
