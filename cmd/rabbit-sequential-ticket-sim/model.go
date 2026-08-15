package main

type options struct {
	honestParticipants uint64
	identities         string
	attackerLanes      string
	networkSizes       string
	scaleAttackerLanes uint64
	fallbacks          uint64
	committeeSize      uint64
	outputDir          string
}

type selectionResult struct {
	HonestLanes                      uint64  `json:"honestLanes"`
	AttackerLanes                    uint64  `json:"attackerLanes"`
	TotalLanes                       uint64  `json:"totalLanes"`
	AttackerProducerPercent          float64 `json:"attackerProducerPercent"`
	AttackerFallbackPercent          float64 `json:"attackerFallbackPercent"`
	AttackerCommitteePercent         float64 `json:"attackerCommitteePercent"`
	AttackerCommitteeMajorityPercent float64 `json:"attackerCommitteeMajorityPercent"`
	Risk                             string  `json:"risk"`
}

type identityResult struct {
	Identities uint64 `json:"identities"`
	selectionResult
}

type simulationReport struct {
	SimulatorVersion            string            `json:"simulatorVersion"`
	ExecutionStatus             string            `json:"executionStatus"`
	SequentialTicketModel       string            `json:"sequentialTicketModel"`
	IdentityAmplificationStatus string            `json:"identityAmplificationStatus"`
	ResourceMajorityRisk        string            `json:"resourceMajorityRisk"`
	CryptographicVDFStatus      string            `json:"cryptographicVDFStatus"`
	ImplementationStatus        string            `json:"implementationStatus"`
	MainnetGate                 string            `json:"mainnetGate"`
	ConsensusChanged            bool              `json:"consensusChanged"`
	GenesisChanged              bool              `json:"genesisChanged"`
	FixedWorkIdentityScenarios  []identityResult  `json:"fixedWorkIdentityScenarios"`
	AttackerHardwareScenarios   []selectionResult `json:"attackerHardwareScenarios"`
	NetworkScaleScenarios       []selectionResult `json:"networkScaleScenarios"`
	Warnings                    []string          `json:"warnings"`
}
