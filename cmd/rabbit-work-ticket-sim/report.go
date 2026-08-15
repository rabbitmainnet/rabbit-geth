package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func printSummary(report simulationReport) {
	fmt.Println("Rabbit Chain — simulação da defesa por tickets de trabalho contínuos")
	fmt.Println("Execução:", report.SimulationExecution)
	fmt.Println("Regra atual por endereço:", report.CurrentAddressRule)
	fmt.Println("Regra candidata por trabalho:", report.CandidateTicketRule)
	fmt.Println("Implementação:", report.ImplementationStatus)
	fmt.Println("Gate da mainnet:", report.MainnetGate)
	for _, scenario := range report.IdentityScenarios {
		fmt.Printf("%d identidades, mesmo trabalho: atual %.2f%% | candidata %.2f%% do produtor\n",
			scenario.AttackerIdentities,
			scenario.CurrentAddressRule.ProducerPercent,
			scenario.ContinuousWorkTicketRule.ProducerPercent,
		)
	}
	fmt.Println("RESULTADO: a regra candidata remove o ganho gratuito por quantidade de endereços no modelo; ainda não está implementada no consenso.")
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
	fmt.Fprintln(&text, "# Simulação da defesa por tickets de trabalho — Rabbit Chain")
	fmt.Fprintln(&text)
	fmt.Fprintf(&text, "- Execução: **%s**\n", report.SimulationExecution)
	fmt.Fprintf(&text, "- Regra atual por endereço: **%s**\n", report.CurrentAddressRule)
	fmt.Fprintf(&text, "- Regra candidata: **%s**\n", report.CandidateTicketRule)
	fmt.Fprintf(&text, "- Estado: **%s**\n", report.ImplementationStatus)
	fmt.Fprintf(&text, "- Mainnet: **%s**\n", report.MainnetGate)
	fmt.Fprintln(&text)
	fmt.Fprintln(&text, "## Multiplicação de identidades com trabalho total constante")
	fmt.Fprintln(&text)
	fmt.Fprintln(&text, "| Identidades | Regra atual: produtor | Tickets: produtor | Tickets: fallbacks | Tickets: committee | Maioria committee |")
	fmt.Fprintln(&text, "|---:|---:|---:|---:|---:|---:|")
	for _, scenario := range report.IdentityScenarios {
		fmt.Fprintf(&text, "| %d | %.2f%% | %.2f%% | %.2f%% | %.2f%% | %.2f%% |\n",
			scenario.AttackerIdentities,
			scenario.CurrentAddressRule.ProducerPercent,
			scenario.ContinuousWorkTicketRule.ProducerPercent,
			scenario.ContinuousWorkTicketRule.FallbackPercent,
			scenario.ContinuousWorkTicketRule.CommitteePercent,
			scenario.ContinuousWorkTicketRule.CommitteeMajorityPercent,
		)
	}
	fmt.Fprintln(&text)
	fmt.Fprintln(&text, "## Aumento real de trabalho com 5.000 identidades")
	fmt.Fprintln(&text)
	fmt.Fprintln(&text, "| Trabalho atacante | Participação teórica | Produtor observado | Committee observado |")
	fmt.Fprintln(&text, "|---:|---:|---:|---:|")
	for _, scenario := range report.WorkScenarios {
		fmt.Fprintf(&text, "| %d | %.2f%% | %.2f%% | %.2f%% |\n",
			scenario.AttackerWorkUnits,
			scenario.TheoreticalWorkShare,
			scenario.ContinuousWorkTicketRule.ProducerPercent,
			scenario.ContinuousWorkTicketRule.CommitteePercent,
		)
	}
	fmt.Fprintln(&text)
	fmt.Fprintln(&text, "## Interpretação")
	fmt.Fprintln(&text)
	fmt.Fprintln(&text, "A regra candidata torna endereços vazios inúteis: somente provas válidas geram tickets. A participação passa a acompanhar o trabalho computacional, não a quantidade de carteiras.")
	fmt.Fprintln(&text)
	fmt.Fprintln(&text, "**Este PASS aprova somente o modelo matemático. O consenso continua inalterado e a mainnet continua bloqueada.**")
	return os.WriteFile(filepath.Join(directory, "resumo.md"), []byte(text.String()), 0o644)
}
