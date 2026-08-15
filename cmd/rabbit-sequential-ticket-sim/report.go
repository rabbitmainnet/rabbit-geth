package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func printSummary(report simulationReport) {
	fmt.Println("Rabbit Chain — simulação de tickets sequenciais")
	fmt.Println("Execução:", report.ExecutionStatus)
	fmt.Println("Identidades sem trabalho adicional:", report.IdentityAmplificationStatus)
	fmt.Println("Risco de maioria de recursos:", report.ResourceMajorityRisk)
	fmt.Println("VDF criptográfica:", report.CryptographicVDFStatus)
	fmt.Println("Implementação:", report.ImplementationStatus)
	fmt.Println("Gate da mainnet:", report.MainnetGate)
	fmt.Println()
	fmt.Println("Trabalho fixo, variando identidades:")
	for _, scenario := range report.FixedWorkIdentityScenarios {
		fmt.Printf("%d identidades: produtor %.2f%% | committee %.2f%% | maioria %.4f%%\n",
			scenario.Identities,
			scenario.AttackerProducerPercent,
			scenario.AttackerCommitteePercent,
			scenario.AttackerCommitteeMajorityPercent,
		)
	}
	fmt.Println()
	fmt.Println("Aumento real de hardware contra a rede de referência:")
	for _, scenario := range report.AttackerHardwareScenarios {
		fmt.Printf("%d lanes atacantes contra %d honestas: produtor %.2f%% | maioria committee %.4f%% | risco %s\n",
			scenario.AttackerLanes,
			scenario.HonestLanes,
			scenario.AttackerProducerPercent,
			scenario.AttackerCommitteeMajorityPercent,
			scenario.Risk,
		)
	}
	fmt.Println("RESULTADO: identidades gratuitas são neutralizadas, mas maioria de recursos continua sendo um risco fundamental.")
}

func writeReports(directory string, report simulationReport) error {
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
	fmt.Fprintln(&text, "# Simulação de tickets sequenciais — Rabbit Chain")
	fmt.Fprintln(&text)
	fmt.Fprintf(&text, "- Execução: **%s**\n", report.ExecutionStatus)
	fmt.Fprintf(&text, "- Resistência à multiplicação de identidades: **%s**\n", report.IdentityAmplificationStatus)
	fmt.Fprintf(&text, "- Risco de maioria de recursos: **%s**\n", report.ResourceMajorityRisk)
	fmt.Fprintf(&text, "- VDF criptográfica: **%s**\n", report.CryptographicVDFStatus)
	fmt.Fprintf(&text, "- Implementação: **%s**\n", report.ImplementationStatus)
	fmt.Fprintf(&text, "- Mainnet: **%s**\n", report.MainnetGate)
	fmt.Fprintln(&text)
	fmt.Fprintln(&text, "## Trabalho fixo")
	fmt.Fprintln(&text)
	fmt.Fprintln(&text, "| Identidades | Lanes reais | Producer atacante | Committee atacante | Maioria committee |")
	fmt.Fprintln(&text, "|---:|---:|---:|---:|---:|")
	for _, scenario := range report.FixedWorkIdentityScenarios {
		fmt.Fprintf(&text, "| %d | %d | %.4f%% | %.4f%% | %.6f%% |\n",
			scenario.Identities, scenario.AttackerLanes, scenario.AttackerProducerPercent,
			scenario.AttackerCommitteePercent, scenario.AttackerCommitteeMajorityPercent)
	}
	fmt.Fprintln(&text)
	fmt.Fprintln(&text, "## Hardware do atacante")
	fmt.Fprintln(&text)
	fmt.Fprintln(&text, "| Honestas | Atacante | Producer atacante | Maioria committee | Risco |")
	fmt.Fprintln(&text, "|---:|---:|---:|---:|:---:|")
	for _, scenario := range report.AttackerHardwareScenarios {
		fmt.Fprintf(&text, "| %d | %d | %.4f%% | %.6f%% | %s |\n",
			scenario.HonestLanes, scenario.AttackerLanes, scenario.AttackerProducerPercent,
			scenario.AttackerCommitteeMajorityPercent, scenario.Risk)
	}
	fmt.Fprintln(&text)
	fmt.Fprintln(&text, "## Crescimento da rede")
	fmt.Fprintln(&text)
	fmt.Fprintln(&text, "| Honestas | Atacante | Producer atacante | Maioria committee | Risco |")
	fmt.Fprintln(&text, "|---:|---:|---:|---:|:---:|")
	for _, scenario := range report.NetworkScaleScenarios {
		fmt.Fprintf(&text, "| %d | %d | %.4f%% | %.6f%% | %s |\n",
			scenario.HonestLanes, scenario.AttackerLanes, scenario.AttackerProducerPercent,
			scenario.AttackerCommitteeMajorityPercent, scenario.Risk)
	}
	fmt.Fprintln(&text)
	fmt.Fprintln(&text, "O modelo mostra chance por lane de trabalho, não uma identidade por pessoa. Uma implementação criptográfica e revisão independente continuam obrigatórias.")
	fmt.Fprintln(&text)
	fmt.Fprintln(&text, "**O consenso e o genesis permanecem inalterados; a mainnet continua bloqueada.**")
	return os.WriteFile(filepath.Join(directory, "resumo.md"), []byte(text.String()), 0o644)
}
