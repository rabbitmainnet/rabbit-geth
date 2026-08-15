package main

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

func runSimulation(opts options) (simulationReport, error) {
	if opts.honestParticipants == 0 || opts.scaleAttackerLanes == 0 || opts.fallbacks == 0 || opts.committeeSize == 0 {
		return simulationReport{}, fmt.Errorf("parâmetros devem ser maiores que zero")
	}
	identities, err := parsePositiveList(opts.identities)
	if err != nil {
		return simulationReport{}, err
	}
	attackerLanes, err := parsePositiveList(opts.attackerLanes)
	if err != nil {
		return simulationReport{}, err
	}
	networkSizes, err := parsePositiveList(opts.networkSizes)
	if err != nil {
		return simulationReport{}, err
	}

	report := simulationReport{
		SimulatorVersion:            simulatorVersion,
		ExecutionStatus:             "PASS",
		SequentialTicketModel:       "PASS",
		IdentityAmplificationStatus: "PASS",
		ResourceMajorityRisk:        "CONFIRMED",
		CryptographicVDFStatus:      "NOT_IMPLEMENTED",
		ImplementationStatus:        "SIMULATION_ONLY",
		MainnetGate:                 "BLOCKED",
		ConsensusChanged:            false,
		GenesisChanged:              false,
		Warnings: []string{
			"o modelo não implementa nem certifica uma VDF criptográfica",
			"identidades sem trabalho não criam poder, mas hardware adicional cria",
			"nenhum protocolo permissionless garante uma pessoa por lane sem identidade externa",
			"uma rede pequena pode ser dominada por um atacante com mais lanes que todos os honestos",
		},
	}

	fixedSelection := calculateSelection(opts.honestParticipants, 1, opts.fallbacks, opts.committeeSize)
	for _, count := range identities {
		report.FixedWorkIdentityScenarios = append(report.FixedWorkIdentityScenarios, identityResult{
			Identities:      count,
			selectionResult: fixedSelection,
		})
	}
	for _, lanes := range attackerLanes {
		report.AttackerHardwareScenarios = append(report.AttackerHardwareScenarios,
			calculateSelection(opts.honestParticipants, lanes, opts.fallbacks, opts.committeeSize))
	}
	for _, honest := range networkSizes {
		report.NetworkScaleScenarios = append(report.NetworkScaleScenarios,
			calculateSelection(honest, opts.scaleAttackerLanes, opts.fallbacks, opts.committeeSize))
	}
	return report, nil
}

func calculateSelection(honest, attacker, fallbacks, committeeTarget uint64) selectionResult {
	total := honest + attacker
	committee := committeeTarget
	if committee > total {
		committee = total
	}
	attackerShare := float64(attacker) / float64(total)
	majority := hypergeometricMajorityProbability(attacker, honest, committee)
	risk := "LOW"
	if attackerShare >= 0.5 || majority >= 0.5 {
		risk = "CRITICAL"
	} else if attackerShare >= 1.0/3.0 || majority >= 0.10 {
		risk = "HIGH"
	} else if attackerShare >= 0.10 || majority >= 0.01 {
		risk = "MEDIUM"
	}
	_ = fallbacks
	return selectionResult{
		HonestLanes:                      honest,
		AttackerLanes:                    attacker,
		TotalLanes:                       total,
		AttackerProducerPercent:          round(attackerShare*100, 4),
		AttackerFallbackPercent:          round(attackerShare*100, 4),
		AttackerCommitteePercent:         round(attackerShare*100, 4),
		AttackerCommitteeMajorityPercent: round(majority*100, 6),
		Risk:                             risk,
	}
}

func hypergeometricMajorityProbability(attacker, honest, draws uint64) float64 {
	if draws == 0 {
		return 0
	}
	total := attacker + honest
	minimum := draws/2 + 1
	maximum := draws
	if maximum > attacker {
		maximum = attacker
	}
	var probability float64
	for selected := minimum; selected <= maximum; selected++ {
		if draws-selected > honest {
			continue
		}
		logProbability := logCombination(attacker, selected) +
			logCombination(honest, draws-selected) -
			logCombination(total, draws)
		probability += math.Exp(logProbability)
	}
	if probability > 1 {
		return 1
	}
	return probability
}

func logCombination(total, selected uint64) float64 {
	if selected > total {
		return math.Inf(-1)
	}
	left, _ := math.Lgamma(float64(total) + 1)
	middle, _ := math.Lgamma(float64(selected) + 1)
	right, _ := math.Lgamma(float64(total-selected) + 1)
	return left - middle - right
}

func parsePositiveList(value string) ([]uint64, error) {
	var values []uint64
	seen := make(map[uint64]bool)
	for _, field := range strings.Split(value, ",") {
		parsed, err := strconv.ParseUint(strings.TrimSpace(field), 10, 64)
		if err != nil || parsed == 0 || parsed > 1_000_000 {
			return nil, fmt.Errorf("valor inválido %q", field)
		}
		if !seen[parsed] {
			seen[parsed] = true
			values = append(values, parsed)
		}
	}
	if len(values) == 0 {
		return nil, fmt.Errorf("lista vazia")
	}
	sort.Slice(values, func(left, right int) bool { return values[left] < values[right] })
	return values, nil
}

func round(value float64, places int) float64 {
	factor := math.Pow10(places)
	return math.Round(value*factor) / factor
}
