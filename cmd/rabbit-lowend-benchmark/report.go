package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func printSummary(report benchmarkReport) {
	fmt.Println("Rabbit Chain — benchmark de acessibilidade para PC fraco")
	fmt.Println("Execução:", report.ExecutionStatus)
	fmt.Println("Estabilidade das medições:", report.MeasurementStabilityStatus)
	fmt.Println("Estabilidade contínua:", report.ContinuousStabilityStatus)
	fmt.Println("Estabilidade isolada:", report.IsolatedStabilityStatus)
	fmt.Println("Diagnóstico:", report.DiagnosticConclusion)
	fmt.Println("Algoritmo protótipo:", report.PrototypeAlgorithm)
	fmt.Println("Perfil local encontrado:", report.CandidateProfileStatus)
	if report.SelectedMemoryMiB > 0 {
		fmt.Printf("Perfil local selecionado: %d MiB\n", report.SelectedMemoryMiB)
	}
	fmt.Println("Acessibilidade geral:", report.LowEndAccessibilityStatus)
	fmt.Println("Implementação:", report.ImplementationStatus)
	fmt.Println("Gate da mainnet:", report.MainnetGate)
	for _, profile := range report.Profiles {
		fmt.Printf("%d MiB: contínuo p95 %.3f ms/var %.2f%% | isolado p95 %.3f ms/var %.2f%% | PC fraco %.3f ms | %d verificações/bloco | qualifica=%t\n",
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
	fmt.Println("RESULTADO: medição concluída; nenhum algoritmo foi ativado no consenso.")
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
	fmt.Fprintln(&text, "# Benchmark de acessibilidade — Rabbit Chain")
	fmt.Fprintln(&text)
	fmt.Fprintf(&text, "- Ambiente: **%s/%s, %d CPUs lógicas, %s**\n", report.RuntimeOS, report.RuntimeArchitecture, report.RuntimeLogicalCPUs, report.GoVersion)
	fmt.Fprintf(&text, "- Execução: **%s**\n", report.ExecutionStatus)
	fmt.Fprintf(&text, "- Estabilidade das medições: **%s**\n", report.MeasurementStabilityStatus)
	fmt.Fprintf(&text, "- Estabilidade contínua: **%s**\n", report.ContinuousStabilityStatus)
	fmt.Fprintf(&text, "- Estabilidade isolada: **%s**\n", report.IsolatedStabilityStatus)
	fmt.Fprintf(&text, "- Diagnóstico: **%s**\n", report.DiagnosticConclusion)
	fmt.Fprintf(&text, "- Algoritmo protótipo: **%s**\n", report.PrototypeAlgorithm)
	fmt.Fprintf(&text, "- Perfil local: **%s**\n", report.CandidateProfileStatus)
	if report.SelectedMemoryMiB > 0 {
		fmt.Fprintf(&text, "- Perfil local selecionado: **%d MiB**\n", report.SelectedMemoryMiB)
	}
	fmt.Fprintf(&text, "- Acessibilidade para todos os PCs fracos: **%s**\n", report.LowEndAccessibilityStatus)
	fmt.Fprintf(&text, "- Implementação: **%s**\n", report.ImplementationStatus)
	fmt.Fprintf(&text, "- Mainnet: **%s**\n", report.MainnetGate)
	fmt.Fprintln(&text)
	fmt.Fprintln(&text, "| Memória | Contínuo p95 | Var. contínua | Isolado p95 | Var. isolada | PC fraco ms | Verificações | Dificuldade | Ticket PC fraco | 1000 tickets | Qualifica |")
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
		fmt.Fprintln(&text, "## Anomalias de medição")
		fmt.Fprintln(&text)
		for _, anomaly := range report.MeasurementAnomalies {
			fmt.Fprintf(&text, "- %s\n", anomaly)
		}
	}
	fmt.Fprintln(&text)
	fmt.Fprintln(&text, "Este resultado mede somente a máquina local e projeta um PC quatro vezes mais lento. São necessários benchmarks físicos adicionais antes de escolher o algoritmo.")
	fmt.Fprintln(&text)
	fmt.Fprintln(&text, "**O consenso e o genesis permanecem inalterados; a mainnet continua bloqueada.**")
	return os.WriteFile(filepath.Join(directory, "resumo.md"), []byte(text.String()), 0o644)
}
