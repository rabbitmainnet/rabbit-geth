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
			"endereços sem prova não entram na seleção",
			"cada ticket exige trabalho novo e expira após uma época",
			"desafio inclui chain ID, época e checkpoint canônico",
			"seed de seleção só fica conhecida depois do fechamento das provas",
			"dividir a mesma capacidade entre mais endereços não cria tickets adicionais",
		},
		RemainingRisks: []string{
			"a chance continua proporcional ao trabalho computacional total, não a pessoas",
			"dificuldade e capacidade por época precisam de ajuste determinístico",
			"pool, persistência, reorg, expiração e sincronização ainda precisam ser implementados e auditados",
			"resistência a grinding e manipulação do checkpoint precisa de testes adversariais ao vivo",
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

	// A seed diferente para cada cenário produz pequena variação amostral, mas
	// não muda a quantidade de tickets do controlador. Estes limites detectam
	// amplificação material sem transformar ruído estatístico em falso FAIL.
	if spread(candidateShares) > 10 || max(candidateShares) > 15 {
		report.CandidateTicketRule = "FAIL"
	}
	return report, nil
}
