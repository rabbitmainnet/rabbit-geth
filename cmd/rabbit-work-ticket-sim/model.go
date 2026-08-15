package main

type options struct {
	honestMiners      int
	identityScenarios string
	workScenarios     string
	slots             uint64
	ticketsPerWork    uint64
	fallbacks         uint64
	committeeMin      uint64
	committeeMax      uint64
	outputDir         string
}

type roleMetrics struct {
	ProducerPercent             float64 `json:"producerPercent"`
	FallbackPercent             float64 `json:"fallbackPercent"`
	CommitteePercent            float64 `json:"committeePercent"`
	CommitteeMajorityPercent    float64 `json:"committeeMajorityPercent"`
	ProducerFallbackFullPercent float64 `json:"producerFallbackFullPercent"`
}

type identityScenarioResult struct {
	AttackerIdentities       int         `json:"attackerIdentities"`
	AttackerWorkUnits        uint64      `json:"attackerWorkUnits"`
	CurrentAddressRule       roleMetrics `json:"currentAddressRule"`
	ContinuousWorkTicketRule roleMetrics `json:"continuousWorkTicketRule"`
}

type workScenarioResult struct {
	AttackerIdentities       int         `json:"attackerIdentities"`
	AttackerWorkUnits        uint64      `json:"attackerWorkUnits"`
	TheoreticalWorkShare     float64     `json:"theoreticalWorkSharePercent"`
	ContinuousWorkTicketRule roleMetrics `json:"continuousWorkTicketRule"`
}

type simulationReport struct {
	SimulatorVersion     string                   `json:"simulatorVersion"`
	GeneratedAt          string                   `json:"generatedAt"`
	SimulationExecution  string                   `json:"simulationExecution"`
	CurrentAddressRule   string                   `json:"currentAddressRule"`
	CandidateTicketRule  string                   `json:"candidateTicketRule"`
	ImplementationStatus string                   `json:"implementationStatus"`
	MainnetGate          string                   `json:"mainnetGate"`
	ConsensusChanged     bool                     `json:"consensusChanged"`
	GenesisChanged       bool                     `json:"genesisChanged"`
	HonestMiners         int                      `json:"honestMiners"`
	SlotsPerScenario     uint64                   `json:"slotsPerScenario"`
	TicketsPerWorkUnit   uint64                   `json:"ticketsPerWorkUnit"`
	IdentityScenarios    []identityScenarioResult `json:"identityScenarios"`
	WorkScenarios        []workScenarioResult     `json:"workScenarios"`
	SecurityProperties   []string                 `json:"securityProperties"`
	RemainingRisks       []string                 `json:"remainingRisks"`
}

type metricCounter struct {
	slots                  uint64
	producer               uint64
	fallbackSeats          uint64
	attackerFallbackSeats  uint64
	committeeSeats         uint64
	attackerCommitteeSeats uint64
	committeeMajority      uint64
	producerFallbackFull   uint64
}

func (counter metricCounter) metrics() roleMetrics {
	return roleMetrics{
		ProducerPercent:             percentage(counter.producer, counter.slots),
		FallbackPercent:             percentage(counter.attackerFallbackSeats, counter.fallbackSeats),
		CommitteePercent:            percentage(counter.attackerCommitteeSeats, counter.committeeSeats),
		CommitteeMajorityPercent:    percentage(counter.committeeMajority, counter.slots),
		ProducerFallbackFullPercent: percentage(counter.producerFallbackFull, counter.slots),
	}
}
