package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func printSummary(report simulationReport) {
	fmt.Println("Rabbit Chain — sequential ticket simulation")
	fmt.Println("Execution:", report.ExecutionStatus)
	fmt.Println("Identities without additional work:", report.IdentityAmplificationStatus)
	fmt.Println("Resource-majority risk:", report.ResourceMajorityRisk)
	fmt.Println("Cryptographic VDF:", report.CryptographicVDFStatus)
	fmt.Println("Implementation:", report.ImplementationStatus)
	fmt.Println("Mainnet gate:", report.MainnetGate)
	fmt.Println()
	fmt.Println("Fixed work with varying identities:")
	for _, scenario := range report.FixedWorkIdentityScenarios {
		fmt.Printf("%d identities: producer %.2f%% | committee %.2f%% | majority %.4f%%\n",
			scenario.Identities,
			scenario.AttackerProducerPercent,
			scenario.AttackerCommitteePercent,
			scenario.AttackerCommitteeMajorityPercent,
		)
	}
	fmt.Println()
	fmt.Println("Actual hardware increase against the reference network:")
	for _, scenario := range report.AttackerHardwareScenarios {
		fmt.Printf("%d attacker lanes against %d honest lanes: producer %.2f%% | committee majority %.4f%% | risk %s\n",
			scenario.AttackerLanes,
			scenario.HonestLanes,
			scenario.AttackerProducerPercent,
			scenario.AttackerCommitteeMajorityPercent,
			scenario.Risk,
		)
	}
	fmt.Println("RESULT: free identities are neutralized, but resource majority remains a fundamental risk.")
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
	fmt.Fprintln(&text, "# Sequential ticket simulation — Rabbit Chain")
	fmt.Fprintln(&text)
	fmt.Fprintf(&text, "- Execution: **%s**\n", report.ExecutionStatus)
	fmt.Fprintf(&text, "- Resistance to identity multiplication: **%s**\n", report.IdentityAmplificationStatus)
	fmt.Fprintf(&text, "- Resource-majority risk: **%s**\n", report.ResourceMajorityRisk)
	fmt.Fprintf(&text, "- Cryptographic VDF: **%s**\n", report.CryptographicVDFStatus)
	fmt.Fprintf(&text, "- Implementation: **%s**\n", report.ImplementationStatus)
	fmt.Fprintf(&text, "- Mainnet: **%s**\n", report.MainnetGate)
	fmt.Fprintln(&text)
	fmt.Fprintln(&text, "## Fixed work")
	fmt.Fprintln(&text)
	fmt.Fprintln(&text, "| Identities | Actual lanes | Attacker producer | Attacker committee | Committee majority |")
	fmt.Fprintln(&text, "|---:|---:|---:|---:|---:|")
	for _, scenario := range report.FixedWorkIdentityScenarios {
		fmt.Fprintf(&text, "| %d | %d | %.4f%% | %.4f%% | %.6f%% |\n",
			scenario.Identities, scenario.AttackerLanes, scenario.AttackerProducerPercent,
			scenario.AttackerCommitteePercent, scenario.AttackerCommitteeMajorityPercent)
	}
	fmt.Fprintln(&text)
	fmt.Fprintln(&text, "## Attacker hardware")
	fmt.Fprintln(&text)
	fmt.Fprintln(&text, "| Honest | Attacker | Attacker producer | Committee majority | Risk |")
	fmt.Fprintln(&text, "|---:|---:|---:|---:|:---:|")
	for _, scenario := range report.AttackerHardwareScenarios {
		fmt.Fprintf(&text, "| %d | %d | %.4f%% | %.6f%% | %s |\n",
			scenario.HonestLanes, scenario.AttackerLanes, scenario.AttackerProducerPercent,
			scenario.AttackerCommitteeMajorityPercent, scenario.Risk)
	}
	fmt.Fprintln(&text)
	fmt.Fprintln(&text, "## Network growth")
	fmt.Fprintln(&text)
	fmt.Fprintln(&text, "| Honest | Attacker | Attacker producer | Committee majority | Risk |")
	fmt.Fprintln(&text, "|---:|---:|---:|---:|:---:|")
	for _, scenario := range report.NetworkScaleScenarios {
		fmt.Fprintf(&text, "| %d | %d | %.4f%% | %.6f%% | %s |\n",
			scenario.HonestLanes, scenario.AttackerLanes, scenario.AttackerProducerPercent,
			scenario.AttackerCommitteeMajorityPercent, scenario.Risk)
	}
	fmt.Fprintln(&text)
	fmt.Fprintln(&text, "The model represents probability per work lane, not one identity per person. A cryptographic implementation and independent review remain mandatory.")
	fmt.Fprintln(&text)
	fmt.Fprintln(&text, "**Consensus and genesis remain unchanged; mainnet remains blocked.**")
	return os.WriteFile(filepath.Join(directory, "resumo.md"), []byte(text.String()), 0o644)
}
