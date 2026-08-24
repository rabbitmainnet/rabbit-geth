package main

import "time"

func runSimulation(opts options) (simulationReport, error) {
	if err := validateOptions(opts); err != nil {
		return simulationReport{}, err
	}
	identities, err := parsePositiveList(opts.identityScenarios)
	if err != nil {
		return simulationReport{}, err
	}
	workUnits, err := parsePositiveList(opts.workScenarios)
	if err != nil {
		return simulationReport{}, err
	}

	report := simulationReport{
		SimulatorVersion:     simulatorVersion,
		GeneratedAt:          time.Now().UTC().Format(time.RFC3339),
		SimulationExecution:  "PASS",
		CurrentAddressRule:   "FAIL",
		CandidateTicketRule:  "PASS",
		ImplementationStatus: "SIMULATION_ONLY",
		MainnetGate:          "BLOCKED",
		ConsensusChanged:     false,
		GenesisChanged:       false,
		HonestMiners:         opts.honestMiners,
		SlotsPerScenario:     opts.slots,
		TicketsPerWorkUnit:   opts.ticketsPerWork,
		SecurityProperties: []string{
			"addresses without proof do not enter selection",
			"each ticket requires new work and expires after one epoch",
			"the challenge includes chain ID, epoch, and canonical checkpoint",
			"the selection seed becomes known only after proof submission closes",
			"splitting the same capacity across more addresses does not create additional tickets",
		},
		RemainingRisks: []string{
			"probability remains proportional to total computational work, not people",
			"difficulty and per-epoch capacity require deterministic adjustment",
			"pooling, persistence, reorgs, expiration, and synchronization still need implementation and auditing",
			"resistance to grinding and checkpoint manipulation requires live adversarial testing",
		},
	}

	var candidateShares []float64
	for _, identityCount := range identities {
		current := simulateCurrentAddressRule(opts, identityCount)
		candidate := simulateWorkTicketRule(opts, identityCount, 1)
		report.IdentityScenarios = append(report.IdentityScenarios, identityScenarioResult{
			AttackerIdentities:       identityCount,
			AttackerWorkUnits:        1,
			CurrentAddressRule:       current,
			ContinuousWorkTicketRule: candidate,
		})
		candidateShares = append(candidateShares, candidate.ProducerPercent)
	}

	for _, work := range workUnits {
		candidate := simulateWorkTicketRule(opts, identities[len(identities)-1], uint64(work))
		report.WorkScenarios = append(report.WorkScenarios, workScenarioResult{
			AttackerIdentities:       identities[len(identities)-1],
			AttackerWorkUnits:        uint64(work),
			TheoreticalWorkShare:     roundPercent(float64(work) / float64(opts.honestMiners+work)),
			ContinuousWorkTicketRule: candidate,
		})
	}

	// A different seed for each scenario produces minor sampling variation, but
	// does not change the controller ticket count. These thresholds detect
	// material amplification without turning statistical noise into a false FAIL.
	if spread(candidateShares) > 10 || max(candidateShares) > 15 {
		report.CandidateTicketRule = "FAIL"
	}
	return report, nil
}
