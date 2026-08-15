package main

type finding struct {
	Severity    string `json:"severity"`
	Code        string `json:"code"`
	Title       string `json:"title"`
	Description string `json:"description"`
	FirstBlock  uint64 `json:"firstBlock,omitempty"`
}

type configReport struct {
	Engine                string   `json:"engine"`
	SelectionSizing       string   `json:"selectionSizing"`
	ChainID               string   `json:"chainId"`
	RegistryMode          string   `json:"registryMode"`
	BootstrapParticipants []string `json:"bootstrapParticipants"`
	EraLength             uint64   `json:"eraLength"`
	CommitteeRatioBPS     uint64   `json:"committeeRatioBps"`
	ProducerRatioBPS      uint64   `json:"producerRatioBps"`
	FallbackCount         uint64   `json:"fallbackCount"`
	CommitteeSize         uint64   `json:"committeeSize"`
	CommitteeMin          uint64   `json:"committeeMin"`
	CommitteeMax          uint64   `json:"committeeMax"`
	RewardMode            string   `json:"rewardMode"`
	RegistryProtocolBlock uint64   `json:"registryProtocolBlock"`
	LockedThroughBlock    uint64   `json:"lockedThroughBlock"`
	ReleaseStage1Block    uint64   `json:"releaseStage1Block"`
	ReleaseStage2Block    uint64   `json:"releaseStage2Block"`
	ReleaseStage3Block    uint64   `json:"releaseStage3Block"`
	ReleaseStage4Block    uint64   `json:"releaseStage4Block"`
}

type allocationResult struct {
	Address                    string `json:"address"`
	Role                       string `json:"role"`
	ExpectedWei                string `json:"expectedWei"`
	ExpectedRAB                string `json:"expectedRab"`
	ExpectedLiquidCreditWei    string `json:"expectedLiquidCreditWei"`
	ExpectedLockedCreditWei    string `json:"expectedLockedCreditWei"`
	ObservedEmissionWei        string `json:"observedEmissionWei"`
	ObservedEmissionRAB        string `json:"observedEmissionRab"`
	ObservedBalanceDeltaWei    string `json:"observedBalanceDeltaWei"`
	TransactionBalanceDeltaWei string `json:"transactionBalanceDeltaWei"`
	ConsensusLiquidDeltaWei    string `json:"consensusLiquidDeltaWei"`
	ObservedLockedDeltaWei     string `json:"observedLockedDeltaWei"`
	ObservedReleaseWei         string `json:"observedReleaseWei"`
	DifferenceWei              string `json:"differenceWei"`
	Match                      bool   `json:"match"`
}

type blockResult struct {
	Number                   uint64             `json:"number"`
	Hash                     string             `json:"hash"`
	ParentHash               string             `json:"parentHash"`
	Producer                 string             `json:"producer"`
	QueuePosition            int                `json:"queuePosition"`
	Committee                []string           `json:"committee"`
	Era                      uint64             `json:"era"`
	ExpectedRewardWei        string             `json:"expectedRewardWei"`
	ExpectedRewardRAB        string             `json:"expectedRewardRab"`
	ObservedEmissionWei      string             `json:"observedEmissionWei"`
	ObservedEmissionRAB      string             `json:"observedEmissionRab"`
	DifferenceWei            string             `json:"differenceWei"`
	Transactions             int                `json:"transactions"`
	TransactionTraceReliable bool               `json:"transactionTraceReliable"`
	ExpectedIndexCount       uint64             `json:"expectedVestingIndexCount"`
	ObservedIndexCount       uint64             `json:"observedVestingIndexCount"`
	StateMismatchAddresses   []string           `json:"stateMismatchAddresses,omitempty"`
	Allocations              []allocationResult `json:"allocations"`
	Status                   string             `json:"status"`
	Notes                    []string           `json:"notes,omitempty"`
}

type eraResult struct {
	Era                 uint64 `json:"era"`
	BlocksScanned       uint64 `json:"blocksScanned"`
	FirstScannedBlock   uint64 `json:"firstScannedBlock"`
	LastScannedBlock    uint64 `json:"lastScannedBlock"`
	RewardPerBlockWei   string `json:"rewardPerBlockWei"`
	RewardPerBlockRAB   string `json:"rewardPerBlockRab"`
	ExpectedEmissionWei string `json:"expectedEmissionWei"`
	ObservedEmissionWei string `json:"observedEmissionWei"`
	DifferenceWei       string `json:"differenceWei"`
}

type participantResult struct {
	Address                    string `json:"address"`
	ProducerBlocks             uint64 `json:"producerBlocks"`
	CommitteeAssignments       uint64 `json:"committeeAssignments"`
	ExpectedProducerRewardWei  string `json:"expectedProducerRewardWei"`
	ExpectedCommitteeRewardWei string `json:"expectedCommitteeRewardWei"`
	ExpectedTotalRewardWei     string `json:"expectedTotalRewardWei"`
	ObservedEmissionWei        string `json:"observedEmissionWei"`
	DifferenceWei              string `json:"differenceWei"`
}

type halvingResult struct {
	FromEra             uint64 `json:"fromEra"`
	ToEra               uint64 `json:"toEra"`
	BoundaryBlock       uint64 `json:"boundaryBlock"`
	RewardBeforeWei     string `json:"rewardBeforeWei"`
	RewardAtBoundaryWei string `json:"rewardAtBoundaryWei"`
	CoveredByScan       bool   `json:"coveredByScan"`
	ObservedMatch       *bool  `json:"observedMatch,omitempty"`
}

type supplyReport struct {
	GenesisAllocationWei          string `json:"genesisAllocationWei"`
	GenesisAllocationRAB          string `json:"genesisAllocationRab"`
	ExpectedScannedEmissionWei    string `json:"expectedScannedEmissionWei"`
	ObservedScannedEmissionWei    string `json:"observedScannedEmissionWei"`
	ScannedDifferenceWei          string `json:"scannedDifferenceWei"`
	ScheduledEmissionThroughToWei string `json:"scheduledEmissionThroughToWei"`
	ScheduledEmissionThroughToRAB string `json:"scheduledEmissionThroughToRab"`
	GenesisPlusScheduledWei       string `json:"genesisPlusScheduledWei"`
	GenesisPlusScheduledRAB       string `json:"genesisPlusScheduledRab"`
	TerminalRewardWei             string `json:"terminalRewardWei"`
	TerminalRewardRAB             string `json:"terminalRewardRab"`
	CappedSupply                  bool   `json:"cappedSupply"`
}

type summaryReport struct {
	BlocksScanned              uint64 `json:"blocksScanned"`
	PassingBlocks              uint64 `json:"passingBlocks"`
	FailingBlocks              uint64 `json:"failingBlocks"`
	IncompleteBlocks           uint64 `json:"incompleteBlocks"`
	CommitteeBlocks            uint64 `json:"committeeBlocks"`
	ProducerOnlyBlocks         uint64 `json:"producerOnlyBlocks"`
	LockedRewardBlocks         uint64 `json:"lockedRewardBlocks"`
	TransactionBlocks          uint64 `json:"transactionBlocks"`
	RewardMismatchBlocks       uint64 `json:"rewardMismatchBlocks"`
	StateMismatchBlocks        uint64 `json:"stateMismatchBlocks"`
	VestingIndexMismatchBlocks uint64 `json:"vestingIndexMismatchBlocks"`
	UnauthorizedProducerBlocks uint64 `json:"unauthorizedProducerBlocks"`
	ObservedReleaseEvents      uint64 `json:"observedReleaseEvents"`
}

type auditReport struct {
	AuditVersion            string              `json:"auditVersion"`
	GeneratedAt             string              `json:"generatedAt"`
	Status                  string              `json:"status"`
	RewardRuntimeStatus     string              `json:"rewardRuntimeStatus"`
	ArchitectureStatus      string              `json:"architectureStatus"`
	RPC                     string              `json:"rpc"`
	GenesisFile             string              `json:"genesisFile"`
	FromBlock               uint64              `json:"fromBlock"`
	ToBlock                 uint64              `json:"toBlock"`
	HeadAtStart             uint64              `json:"headAtStart"`
	Config                  configReport        `json:"config"`
	Summary                 summaryReport       `json:"summary"`
	Supply                  supplyReport        `json:"supply"`
	Eras                    []eraResult         `json:"eras"`
	Halvings                []halvingResult     `json:"halvings"`
	Participants            []participantResult `json:"participants"`
	IndexedVestingAddresses []string            `json:"indexedVestingAddresses"`
	Findings                []finding           `json:"findings"`
	Blocks                  []blockResult       `json:"blocks"`
}
