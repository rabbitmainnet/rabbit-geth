package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func printSummary(report simulationReport) {
	fmt.Println("Rabbit Chain — continuous work-ticket defense simulation")
	fmt.Println("Execution:", report.SimulationExecution)
	fmt.Println("Current per-address rule:", report.CurrentAddressRule)
	fmt.Println("Candidate per-work rule:", report.CandidateTicketRule)
	fmt.Println("Implementation:", report.ImplementationStatus)
	fmt.Println("Mainnet gate:", report.MainnetGate)
	for _, scenario := range report.IdentityScenarios {
		fmt.Printf("%d identities, same work: current %.2f%% | candidate %.2f%% producer share\n",
			scenario.AttackerIdentities,
			scenario.CurrentAddressRule.ProducerPercent,
			scenario.ContinuousWorkTicketRule.ProducerPercent,
		)
	}
	fmt.Println("RESULT: the candidate rule removes the model’s free gain from address count; it is not yet implemented in consensus.")
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
	fmt.Fprintln(&text, "# Work-ticket defense simulation — Rabbit Chain")
	fmt.Fprintln(&text)
	fmt.Fprintf(&text, "- Execution: **%s**\n", report.SimulationExecution)
	fmt.Fprintf(&text, "- Current per-address rule: **%s**\n", report.CurrentAddressRule)
	fmt.Fprintf(&text, "- Candidate rule: **%s**\n", report.CandidateTicketRule)
	fmt.Fprintf(&text, "- Status: **%s**\n", report.ImplementationStatus)
	fmt.Fprintf(&text, "- Mainnet: **%s**\n", report.MainnetGate)
	fmt.Fprintln(&text)
	fmt.Fprintln(&text, "## Identity multiplication with constant total work")
	fmt.Fprintln(&text)
	fmt.Fprintln(&text, "| Identities | Current rule: producer | Tickets: producer | Tickets: fallbacks | Tickets: committee | Committee majority |")
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
	fmt.Fprintln(&text, "## Actual work increase with 5,000 identities")
	fmt.Fprintln(&text)
	fmt.Fprintln(&text, "| Attacker work | Theoretical share | Observed producer | Observed committee |")
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
	fmt.Fprintln(&text, "## Interpretation")
	fmt.Fprintln(&text)
	fmt.Fprintln(&text, "The candidate rule makes empty addresses useless: only valid proofs generate tickets. Participation tracks computational work rather than wallet count.")
	fmt.Fprintln(&text)
	fmt.Fprintln(&text, "**This PASS approves only the mathematical model. Consensus remains unchanged and mainnet remains blocked.**")
	return os.WriteFile(filepath.Join(directory, "resumo.md"), []byte(text.String()), 0o644)
}
