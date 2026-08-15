package main

import (
	"context"
	"fmt"
	"math/big"
	"sort"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
)

const auditVersion = "rabbit-reward-auditor/1.4.0"

type auditOptions struct {
	RPCEndpoint   string
	GenesisPath   string
	FromBlock     uint64
	ToBlock       uint64
	ProgressEvery uint64
}

type eraAccumulator struct {
	era      uint64
	blocks   uint64
	first    uint64
	last     uint64
	reward   *big.Int
	expected *big.Int
	observed *big.Int
}

type participantAccumulator struct {
	address           common.Address
	producerBlocks    uint64
	committeeBlocks   uint64
	expectedProducer  *big.Int
	expectedCommittee *big.Int
	observed          *big.Int
}

type auditRunner struct {
	options              auditOptions
	genesis              *core.Genesis
	rpcClient            *rpc.Client
	ethClient            *ethclient.Client
	eraLength            uint64
	committeeBPS         uint64
	selection            selectionConfig
	participants         []common.Address
	registrySnapshot     *lqc.RegistrySnapshot
	registryRules        lqc.RegistrySnapshotRules
	registryActivation   uint64
	balanceModelReliable bool
	eraStats             map[uint64]*eraAccumulator
	participantStats     map[common.Address]*participantAccumulator
	observedByBlock      map[uint64]*big.Int
	firstIndexMismatch   uint64
	rewardTraceFailures  uint64
}

func newAuditRunner(options auditOptions, genesis *core.Genesis, rpcClient *rpc.Client) *auditRunner {
	participants := uniqueSortedAddresses(genesis.Config.LQC.BootstrapParticipants)
	rules := lqc.RegistrySnapshotRules{
		ProofDifficulty: genesis.Config.LQC.ProofDifficulty,
		ActivationDelay: genesis.Config.LQC.ActivationDelay,
		HeartbeatWindow: genesis.Config.LQC.HeartbeatWindow,
		HeartbeatGrace:  genesis.Config.LQC.HeartbeatGrace,
		JailBlocks:      genesis.Config.LQC.JailBlocks,
		MaxMissedTurns:  genesis.Config.LQC.MaxMissedTurns,
	}
	if rules.HeartbeatWindow == 0 {
		rules.HeartbeatWindow = 64
	}
	if rules.HeartbeatGrace == 0 {
		rules.HeartbeatGrace = 16
	}
	if rules.JailBlocks == 0 {
		rules.JailBlocks = 256
	}
	if rules.MaxMissedTurns == 0 {
		rules.MaxMissedTurns = 3
	}
	return &auditRunner{
		options:              options,
		genesis:              genesis,
		rpcClient:            rpcClient,
		ethClient:            ethclient.NewClient(rpcClient),
		eraLength:            effectiveEraLength(genesis.Config.LQC),
		committeeBPS:         effectiveCommitteeBPS(genesis.Config.LQC),
		selection:            effectiveSelectionConfig(genesis.Config.LQC),
		participants:         participants,
		registryRules:        rules,
		registryActivation:   genesis.Config.LQC.RegistryProtocolBlock,
		balanceModelReliable: true,
		eraStats:             make(map[uint64]*eraAccumulator),
		participantStats:     make(map[common.Address]*participantAccumulator),
		observedByBlock:      make(map[uint64]*big.Int),
	}
}

// initializeRegistrySnapshot reconstructs the parent registry exclusively from
// canonical headers. This makes a fresh auditor see REGISTER, HEARTBEAT and EXIT
// operations even when the scan starts after block 1.
func (runner *auditRunner) initializeRegistrySnapshot(ctx context.Context, parentBlock *types.Block) error {
	if runner.registryActivation == 0 || parentBlock == nil {
		return nil
	}
	parentNumber := parentBlock.NumberU64()
	activationParent := runner.registryActivation - 1
	if parentNumber < activationParent {
		return nil
	}
	baseBlock := parentBlock
	if parentNumber != activationParent {
		var err error
		baseBlock, err = runner.ethClient.BlockByNumber(ctx, new(big.Int).SetUint64(activationParent))
		if err != nil {
			return fmt.Errorf("read registry activation parent %d: %w", activationParent, err)
		}
	}
	snapshot, err := lqc.NewBootstrapRegistrySnapshot(
		activationParent,
		baseBlock.Hash(),
		runner.genesis.Config.LQC.BootstrapParticipants,
	)
	if err != nil {
		return fmt.Errorf("create bootstrap registry snapshot: %w", err)
	}
	for number := runner.registryActivation; number <= parentNumber; number++ {
		block, err := runner.ethClient.BlockByNumber(ctx, new(big.Int).SetUint64(number))
		if err != nil {
			return fmt.Errorf("read registry history block %d: %w", number, err)
		}
		snapshot, err = snapshot.ApplyHeader(runner.genesis.Config.ChainID, runner.registryRules, block.Header())
		if err != nil {
			return fmt.Errorf("reconstruct registry at block %d: %w", number, err)
		}
	}
	runner.registrySnapshot = snapshot
	runner.addRegistryParticipants(snapshot)
	return nil
}

func (runner *auditRunner) addRegistryParticipants(snapshot *lqc.RegistrySnapshot) []common.Address {
	if snapshot == nil {
		return nil
	}
	known := make(map[common.Address]struct{}, len(runner.participants))
	for _, address := range runner.participants {
		known[address] = struct{}{}
	}
	added := make([]common.Address, 0)
	for _, participant := range snapshot.Participants {
		if participant.Address == (common.Address{}) {
			continue
		}
		if _, exists := known[participant.Address]; exists {
			continue
		}
		known[participant.Address] = struct{}{}
		added = append(added, participant.Address)
		runner.participants = append(runner.participants, participant.Address)
	}
	runner.participants = uniqueSortedAddresses(runner.participants)
	return uniqueSortedAddresses(added)
}

// canonicalSelectionForBlock uses the parent snapshot, matching the engine's
// consensus order. It then validates and applies the current header to obtain
// the post-block snapshot used by the next height.
func (runner *auditRunner) canonicalSelectionForBlock(block *types.Block) ([]common.Address, []common.Address, *lqc.RegistrySnapshot, error) {
	if block == nil || runner.registryActivation == 0 || block.NumberU64() < runner.registryActivation {
		return nil, nil, nil, nil
	}
	if runner.registrySnapshot == nil {
		if block.NumberU64() != runner.registryActivation {
			return nil, nil, nil, fmt.Errorf("registry snapshot unavailable before block %d", block.NumberU64())
		}
		var err error
		runner.registrySnapshot, err = lqc.NewBootstrapRegistrySnapshot(
			block.NumberU64()-1,
			block.ParentHash(),
			runner.genesis.Config.LQC.BootstrapParticipants,
		)
		if err != nil {
			return nil, nil, nil, err
		}
	}
	if runner.registrySnapshot.Number != block.NumberU64()-1 || runner.registrySnapshot.Hash != block.ParentHash() {
		return nil, nil, nil, fmt.Errorf("registry snapshot continuity failure at block %d", block.NumberU64())
	}
	registry, err := runner.registrySnapshot.Registry()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("read parent registry at block %d: %w", block.NumberU64(), err)
	}
	ordered := registry.OrderedParticipantsForBlock(
		block.ParentHash(),
		block.NumberU64(),
		runner.registryRules.ActivationDelay,
		runner.registryRules.HeartbeatWindow,
		runner.registryRules.HeartbeatGrace,
	)
	queue := make([]common.Address, 0, len(ordered))
	for _, participant := range ordered {
		queue = append(queue, participant.Address)
	}
	committee := canonicalRewardCommittee(queue, block.Coinbase(), runner.selection)
	post, err := runner.registrySnapshot.ApplyHeader(
		runner.genesis.Config.ChainID,
		runner.registryRules,
		block.Header(),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("validate canonical registry at block %d: %w", block.NumberU64(), err)
	}
	return queue, committee, post, nil
}

func (runner *auditRunner) run(ctx context.Context) (*auditReport, error) {
	chainID, err := runner.ethClient.ChainID(ctx)
	if err != nil {
		return nil, fmt.Errorf("read chain ID: %w", err)
	}
	if chainID.Cmp(runner.genesis.Config.ChainID) != 0 {
		return nil, fmt.Errorf("chain ID mismatch: node=%s genesis=%s", chainID, runner.genesis.Config.ChainID)
	}
	head, err := runner.ethClient.BlockNumber(ctx)
	if err != nil {
		return nil, fmt.Errorf("read chain head: %w", err)
	}
	toBlock := runner.options.ToBlock
	if toBlock == 0 || toBlock > head {
		toBlock = head
	}
	if runner.options.FromBlock == 0 {
		return nil, fmt.Errorf("from block must be at least 1")
	}
	if toBlock < runner.options.FromBlock {
		return nil, fmt.Errorf("empty range: from=%d to=%d head=%d", runner.options.FromBlock, toBlock, head)
	}
	parentNumber := runner.options.FromBlock - 1
	parentBlock, err := runner.ethClient.BlockByNumber(ctx, new(big.Int).SetUint64(parentNumber))
	if err != nil {
		return nil, fmt.Errorf("read parent block %d: %w", parentNumber, err)
	}
	if err := runner.initializeRegistrySnapshot(ctx, parentBlock); err != nil {
		return nil, err
	}
	observedPrevious, err := fetchState(ctx, runner.rpcClient, parentBlock.Hash(), runner.participants)
	if err != nil {
		return nil, fmt.Errorf("read state at block %d: %w", parentNumber, err)
	}
	expected := make(map[common.Address]accountState, len(observedPrevious.Accounts))
	for address, state := range observedPrevious.Accounts {
		expected[address] = cloneAccountState(state)
	}
	report := runner.newReport(chainID, head, toBlock)
	for number := runner.options.FromBlock; number <= toBlock; number++ {
		block, err := runner.ethClient.BlockByNumber(ctx, new(big.Int).SetUint64(number))
		if err != nil {
			return nil, fmt.Errorf("read block %d: %w", number, err)
		}
		if block.ParentHash() != parentBlock.Hash() {
			return nil, fmt.Errorf("canonical continuity failure at block %d: parent=%s previous=%s", number, block.ParentHash(), parentBlock.Hash())
		}
		queue := deterministicQueue(runner.participants, block.ParentHash(), number)
		committee := rewardCommittee(queue, block.Coinbase(), runner.selection)
		var postRegistrySnapshot *lqc.RegistrySnapshot
		if runner.registryActivation > 0 && number >= runner.registryActivation {
			queue, committee, postRegistrySnapshot, err = runner.canonicalSelectionForBlock(block)
			if err != nil {
				return nil, err
			}
		}
		queuePosition := addressPosition(queue, block.Coinbase())
		reward, era := rewardForBlock(number, runner.eraLength)
		allocations := allocateReward(reward, block.Coinbase(), committee, runner.committeeBPS)
		newAddresses := make([]common.Address, 0)
		for _, address := range runner.addRegistryParticipants(postRegistrySnapshot) {
			newAddresses = append(newAddresses, address)
		}
		if !containsAddress(runner.participants, block.Coinbase()) {
			newAddresses = append(newAddresses, block.Coinbase())
		}
		for _, allocation := range allocations {
			if !containsAddress(runner.participants, allocation.Address) && !containsAddress(newAddresses, allocation.Address) {
				newAddresses = append(newAddresses, allocation.Address)
			}
		}
		if len(newAddresses) > 0 {
			baseline, err := fetchState(ctx, runner.rpcClient, parentBlock.Hash(), newAddresses)
			if err != nil {
				return nil, fmt.Errorf("read new participant baseline at block %d: %w", number-1, err)
			}
			for _, address := range newAddresses {
				if !containsAddress(runner.participants, address) {
					runner.participants = append(runner.participants, address)
				}
				observedPrevious.Accounts[address] = cloneAccountState(baseline.Accounts[address])
				expected[address] = cloneAccountState(baseline.Accounts[address])
			}
			runner.participants = uniqueSortedAddresses(runner.participants)
		}
		observedCurrent, err := fetchState(ctx, runner.rpcClient, block.Hash(), runner.participants)
		if err != nil {
			return nil, fmt.Errorf("read state at block %d: %w", number, err)
		}
		transactionDeltas := make(map[common.Address]*big.Int)
		traceReliable := true
		if len(block.Transactions()) > 0 {
			report.Summary.TransactionBlocks++
			transactionDeltas, err = transactionBalanceDeltas(ctx, runner.rpcClient, number)
			if err != nil {
				traceReliable = false
				runner.balanceModelReliable = false
				runner.rewardTraceFailures++
			}
		}
		for _, address := range runner.participants {
			state := cloneAccountState(expected[address])
			if traceReliable {
				if delta := transactionDeltas[address]; delta != nil {
					state.Balance.Add(state.Balance, delta)
				}
			}
			expected[address] = state
		}
		expectedAmounts := make(map[common.Address]*big.Int)
		roles := make(map[common.Address]string)
		for _, allocation := range allocations {
			state := cloneAccountState(expected[allocation.Address])
			creditExpected(&state, allocation.Amount)
			expected[allocation.Address] = state
			expectedAmounts[allocation.Address] = cloneBig(allocation.Amount)
			roles[allocation.Address] = allocation.Role
		}
		result := runner.auditBlock(block, era, reward, queuePosition, committee, expectedAmounts, roles, transactionDeltas, traceReliable, expected, observedPrevious, observedCurrent)
		report.Blocks = append(report.Blocks, result)
		runner.updateSummary(&report.Summary, result)
		runner.updateAggregates(block, era, reward, allocations, result)
		observedPrevious = observedCurrent
		parentBlock = block
		if postRegistrySnapshot != nil {
			runner.registrySnapshot = postRegistrySnapshot
		}
		if runner.options.ProgressEvery > 0 && (number-runner.options.FromBlock+1)%runner.options.ProgressEvery == 0 {
			fmt.Printf("auditados %d/%d blocos (altura %d)\n", number-runner.options.FromBlock+1, toBlock-runner.options.FromBlock+1, number)
		}
	}
	runner.finishReport(ctx, report, parentBlock, observedPrevious)
	return report, nil
}

func (runner *auditRunner) newReport(chainID *big.Int, head, to uint64) *auditReport {
	bootstrapParticipants := uniqueSortedAddresses(runner.genesis.Config.LQC.BootstrapParticipants)
	bootstrap := make([]string, len(bootstrapParticipants))
	for i, address := range bootstrapParticipants {
		bootstrap[i] = address.Hex()
	}
	selectionSizing := "fixed committeeSize from genesis"
	if runner.selection.DynamicCommittee {
		selectionSizing = "ceil(active*10%), bounded by committeeMin/committeeMax"
	}
	return &auditReport{
		AuditVersion:        auditVersion,
		GeneratedAt:         time.Now().UTC().Format(time.RFC3339),
		Status:              "INCOMPLETE",
		RewardRuntimeStatus: "INCOMPLETE",
		ArchitectureStatus:  "INCOMPLETE",
		RPC:                 runner.options.RPCEndpoint,
		GenesisFile:         runner.options.GenesisPath,
		FromBlock:           runner.options.FromBlock,
		ToBlock:             to,
		HeadAtStart:         head,
		Config: configReport{
			Engine:                "consensus/lqc",
			SelectionSizing:       selectionSizing,
			ChainID:               chainID.String(),
			RegistryMode:          runner.genesis.Config.LQC.RegistryMode,
			BootstrapParticipants: bootstrap,
			EraLength:             runner.eraLength,
			CommitteeRatioBPS:     runner.committeeBPS,
			ProducerRatioBPS:      10_000 - runner.committeeBPS,
			FallbackCount:         runner.selection.FallbackCount,
			CommitteeSize:         runner.selection.CommitteeSize,
			CommitteeMin:          runner.selection.CommitteeMin,
			CommitteeMax:          runner.selection.CommitteeMax,
			RewardMode:            "immediate-liquid",
			RegistryProtocolBlock: runner.registryActivation,
			LockedThroughBlock:    0,
			ReleaseStage1Block:    0,
			ReleaseStage2Block:    0,
			ReleaseStage3Block:    0,
			ReleaseStage4Block:    0,
		},
		Blocks: make([]blockResult, 0, to-runner.options.FromBlock+1),
	}
}

func (runner *auditRunner) auditBlock(
	block *types.Block,
	era uint64,
	reward *big.Int,
	queuePosition int,
	committee []common.Address,
	expectedAmounts map[common.Address]*big.Int,
	roles map[common.Address]string,
	transactionDeltas map[common.Address]*big.Int,
	traceReliable bool,
	expected map[common.Address]accountState,
	previous stateSnapshot,
	current stateSnapshot,
) blockResult {
	result := blockResult{
		Number:                   block.NumberU64(),
		Hash:                     block.Hash().Hex(),
		ParentHash:               block.ParentHash().Hex(),
		Producer:                 block.Coinbase().Hex(),
		QueuePosition:            queuePosition,
		Committee:                addressStrings(committee),
		Era:                      era,
		ExpectedRewardWei:        reward.String(),
		ExpectedRewardRAB:        formatRAB(reward),
		Transactions:             len(block.Transactions()),
		TransactionTraceReliable: traceReliable,
		ExpectedIndexCount:       previous.IndexCount,
		ObservedIndexCount:       current.IndexCount,
		Status:                   "PASS",
	}
	if !traceReliable {
		result.Status = "INCOMPLETE"
		result.Notes = append(result.Notes, "transaction trace unavailable; reward balance delta is not isolated")
	}
	if queuePosition < 0 {
		result.Status = "FAIL"
		result.Notes = append(result.Notes, "producer is absent from the canonical registry queue")
	}
	observedTotal := new(big.Int)
	addresses := uniqueSortedAddresses(runner.participants)
	for _, address := range addresses {
		before := previous.Accounts[address]
		after := current.Accounts[address]
		balanceDelta := new(big.Int).Sub(after.Balance, before.Balance)
		transactionDelta := cloneBig(transactionDeltas[address])
		consensusLiquid := new(big.Int).Sub(cloneBig(balanceDelta), transactionDelta)
		lockedDelta := new(big.Int).Sub(after.Locked, before.Locked)
		observedEmission := cloneBig(consensusLiquid)
		expectedAmount := cloneBig(expectedAmounts[address])
		difference := new(big.Int).Sub(cloneBig(observedEmission), expectedAmount)
		observedRelease := new(big.Int)
		if lockedDelta.Sign() < 0 {
			observedRelease.Neg(lockedDelta)
		}
		if traceReliable {
			observedTotal.Add(observedTotal, observedEmission)
		}
		include := expectedAmount.Sign() != 0 || observedEmission.Sign() != 0 || lockedDelta.Sign() != 0 || observedRelease.Sign() != 0
		if include {
			expectedLiquid := cloneBig(expectedAmount)
			expectedLocked := new(big.Int)
			match := traceReliable && difference.Sign() == 0
			result.Allocations = append(result.Allocations, allocationResult{
				Address:                    address.Hex(),
				Role:                       roleOrUnexpected(roles[address]),
				ExpectedWei:                expectedAmount.String(),
				ExpectedRAB:                formatRAB(expectedAmount),
				ExpectedLiquidCreditWei:    expectedLiquid.String(),
				ExpectedLockedCreditWei:    expectedLocked.String(),
				ObservedEmissionWei:        observedEmission.String(),
				ObservedEmissionRAB:        formatRAB(observedEmission),
				ObservedBalanceDeltaWei:    balanceDelta.String(),
				TransactionBalanceDeltaWei: transactionDelta.String(),
				ConsensusLiquidDeltaWei:    consensusLiquid.String(),
				ObservedLockedDeltaWei:     lockedDelta.String(),
				ObservedReleaseWei:         observedRelease.String(),
				DifferenceWei:              difference.String(),
				Match:                      match,
			})
			if traceReliable && difference.Sign() != 0 {
				result.Status = "FAIL"
			}
		}
		modeled := expected[address]
		balanceMatches := !runner.balanceModelReliable || modeled.Balance.Cmp(after.Balance) == 0
		if !balanceMatches || modeled.Locked.Cmp(after.Locked) != 0 || modeled.Original.Cmp(after.Original) != 0 || modeled.Stage != after.Stage {
			result.StateMismatchAddresses = append(result.StateMismatchAddresses, address.Hex())
			result.Status = "FAIL"
		}
	}
	result.ObservedEmissionWei = observedTotal.String()
	result.ObservedEmissionRAB = formatRAB(observedTotal)
	difference := new(big.Int).Sub(cloneBig(observedTotal), reward)
	result.DifferenceWei = difference.String()
	if traceReliable && difference.Sign() != 0 {
		result.Status = "FAIL"
	}
	if result.ExpectedIndexCount != result.ObservedIndexCount {
		result.Status = "FAIL"
		result.Notes = append(result.Notes, "legacy vesting index changed although mining rewards are immediate")
		if runner.firstIndexMismatch == 0 {
			runner.firstIndexMismatch = block.NumberU64()
		}
	}
	return result
}

func (runner *auditRunner) updateSummary(summary *summaryReport, block blockResult) {
	summary.BlocksScanned++
	switch block.Status {
	case "PASS":
		summary.PassingBlocks++
	case "INCOMPLETE":
		summary.IncompleteBlocks++
	default:
		summary.FailingBlocks++
	}
	if len(block.Committee) > 0 {
		summary.CommitteeBlocks++
	} else {
		summary.ProducerOnlyBlocks++
	}
	if block.DifferenceWei != "0" && block.TransactionTraceReliable {
		summary.RewardMismatchBlocks++
	}
	if len(block.StateMismatchAddresses) > 0 {
		summary.StateMismatchBlocks++
	}
	if block.ExpectedIndexCount != block.ObservedIndexCount {
		summary.VestingIndexMismatchBlocks++
	}
	if block.QueuePosition < 0 {
		summary.UnauthorizedProducerBlocks++
	}
	for _, allocation := range block.Allocations {
		if allocation.ObservedReleaseWei != "0" {
			summary.ObservedReleaseEvents++
		}
	}
}

func (runner *auditRunner) updateAggregates(block *types.Block, era uint64, reward *big.Int, allocations []rewardAllocation, result blockResult) {
	eraStat := runner.eraStats[era]
	if eraStat == nil {
		eraStat = &eraAccumulator{
			era:      era,
			first:    block.NumberU64(),
			reward:   cloneBig(reward),
			expected: new(big.Int),
			observed: new(big.Int),
		}
		runner.eraStats[era] = eraStat
	}
	eraStat.blocks++
	eraStat.last = block.NumberU64()
	eraStat.expected.Add(eraStat.expected, reward)
	observed, ok := new(big.Int).SetString(result.ObservedEmissionWei, 10)
	if !ok {
		observed = new(big.Int)
	}
	eraStat.observed.Add(eraStat.observed, observed)
	runner.observedByBlock[block.NumberU64()] = cloneBig(observed)
	for _, allocation := range allocations {
		stat := runner.participantStat(allocation.Address)
		if allocation.Role == "producer" {
			stat.producerBlocks++
			stat.expectedProducer.Add(stat.expectedProducer, allocation.Amount)
		} else {
			stat.committeeBlocks++
			stat.expectedCommittee.Add(stat.expectedCommittee, allocation.Amount)
		}
	}
	for _, allocation := range result.Allocations {
		observedAmount, ok := new(big.Int).SetString(allocation.ObservedEmissionWei, 10)
		if !ok {
			continue
		}
		runner.participantStat(common.HexToAddress(allocation.Address)).observed.Add(
			runner.participantStat(common.HexToAddress(allocation.Address)).observed,
			observedAmount,
		)
	}
}

func (runner *auditRunner) participantStat(address common.Address) *participantAccumulator {
	stat := runner.participantStats[address]
	if stat == nil {
		stat = &participantAccumulator{
			address:           address,
			expectedProducer:  new(big.Int),
			expectedCommittee: new(big.Int),
			observed:          new(big.Int),
		}
		runner.participantStats[address] = stat
	}
	return stat
}

func (runner *auditRunner) finishReport(ctx context.Context, report *auditReport, finalBlock *types.Block, observed stateSnapshot) {
	report.Eras = runner.eraResults()
	report.Participants = runner.participantResults()
	report.Halvings = runner.halvingResults(report.FromBlock, report.ToBlock)
	report.Supply = runner.supplyResults(report.FromBlock, report.ToBlock)
	report.Findings = append(report.Findings, finding{
		Severity:    "INFO",
		Code:        "ACTIVE_ENGINE_IS_CANONICAL_LQC",
		Title:       "O cliente está conectado ao consensus/lqc",
		Description: "eth/ethconfig.CreateConsensusEngine instancia consensus/lqc para todo genesis com config.lqc. Esse é o único engine canônico da Rabbit Chain.",
	})
	report.Findings = append(report.Findings, finding{
		Severity:    "INFO",
		Code:        "CANONICAL_REGISTRY_RECONSTRUCTED",
		Title:       "Registry, fila e committee foram reconstruídos pelos headers",
		Description: fmt.Sprintf("O auditor validou os snapshots canônicos, incluindo REGISTER, HEARTBEAT e EXIT, e acompanhou %d participantes conhecidos. Fallbacks configurados: %d.", len(runner.participants), runner.selection.FallbackCount),
	})
	report.Findings = append(report.Findings, finding{
		Severity:    "INFO",
		Code:        "MINING_REWARDS_ARE_IMMEDIATE",
		Title:       "Recompensas de mineração são líquidas imediatamente",
		Description: "O modelo independente credita produtor e committee diretamente no saldo líquido. O armazenamento legado de vesting deve permanecer inalterado.",
	})
	if report.Supply.ExpectedScannedEmissionWei != "0" && report.Supply.ObservedScannedEmissionWei == "0" {
		report.Findings = append(report.Findings, finding{
			Severity:    "CRITICAL",
			Code:        "ACTIVE_RUNTIME_MINTS_ZERO",
			Title:       "O binário ativo não emitiu recompensa em nenhum bloco",
			Description: "A soma observada de créditos líquidos foi zero em todo o intervalo, embora cada bloco devesse emitir reward. Isso é compatível com um laboratório iniciado por um build anterior ao patch de rewards.",
			FirstBlock:  report.FromBlock,
		})
	}
	if observed.IndexCount > 0 && observed.IndexCount <= 100_000 {
		indexed, err := fetchIndexedAddresses(ctx, runner.rpcClient, finalBlock.Hash(), observed.IndexCount)
		if err == nil {
			report.IndexedVestingAddresses = addressStrings(indexed)
		}
	}
	legacyStateAddresses := make([]common.Address, 0)
	for address, state := range observed.Accounts {
		if state.Locked.Sign() != 0 || state.Original.Sign() != 0 || state.Stage != 0 {
			legacyStateAddresses = append(legacyStateAddresses, address)
		}
	}
	if observed.IndexCount > 0 || len(legacyStateAddresses) > 0 || report.Summary.VestingIndexMismatchBlocks > 0 {
		report.Findings = append(report.Findings, finding{
			Severity:    "CRITICAL",
			Code:        "LEGACY_VESTING_STATE_CHANGED",
			Title:       "O estado legado de vesting não permaneceu vazio e estável",
			Description: fmt.Sprintf("A Rabbit Chain usa recompensa imediata. Índice legado=%d e carteiras com estado legado=%d; ambos devem permanecer zerados.", observed.IndexCount, len(legacyStateAddresses)),
			FirstBlock:  runner.firstIndexMismatch,
		})
	}
	if report.Summary.RewardMismatchBlocks > 0 {
		report.Findings = append(report.Findings, finding{
			Severity:    "CRITICAL",
			Code:        "REWARD_EMISSION_MISMATCH",
			Title:       "A emissão observada difere do cronograma",
			Description: "Pelo menos um bloco possui diferença entre a emissão esperada e o crédito líquido isolado das transações.",
		})
	}
	if report.Summary.StateMismatchBlocks > 0 {
		report.Findings = append(report.Findings, finding{
			Severity:    "CRITICAL",
			Code:        "REWARD_STATE_MODEL_MISMATCH",
			Title:       "O estado de reward ou locker difere do modelo independente",
			Description: "Saldo líquido ou armazenamento legado de vesting de pelo menos uma carteira difere da transição esperada para recompensas imediatas.",
		})
	}
	if report.Summary.UnauthorizedProducerBlocks > 0 {
		report.Findings = append(report.Findings, finding{
			Severity:    "CRITICAL",
			Code:        "PRODUCER_OUTSIDE_QUEUE",
			Title:       "Produtor ausente da fila determinística",
			Description: "Pelo menos um bloco canônico foi produzido por uma carteira fora da fila reconstruída a partir dos snapshots canônicos do registry.",
		})
	}
	if runner.rewardTraceFailures > 0 {
		report.Findings = append(report.Findings, finding{
			Severity:    "WARNING",
			Code:        "TRANSACTION_TRACE_UNAVAILABLE",
			Title:       "Alguns efeitos de transações não foram isolados",
			Description: "Um ou mais blocos com transações não puderam ser rastreados; a comparação da recompensa líquida nesses blocos ficou incompleta.",
		})
	}
	report.Findings = append(report.Findings, finding{
		Severity:    "INFO",
		Code:        "TERMINAL_REWARD_CONTINUES",
		Title:       "A recompensa da Era 3 continua indefinidamente",
		Description: "O cronograma mantém 0,15 RAB por bloco na Era 3 e em todas as seguintes. A emissão é previsível, mas não existe cap máximo de supply.",
	})
	switch {
	case report.Summary.FailingBlocks > 0:
		report.RewardRuntimeStatus = "FAIL"
	case report.Summary.IncompleteBlocks > 0 || runner.rewardTraceFailures > 0:
		report.RewardRuntimeStatus = "INCOMPLETE"
	default:
		report.RewardRuntimeStatus = "PASS"
	}
	switch {
	case hasCriticalFinding(report.Findings):
		report.ArchitectureStatus = "FAIL"
	case hasWarningFinding(report.Findings):
		report.ArchitectureStatus = "WARNING"
	default:
		report.ArchitectureStatus = "PASS"
	}
	switch {
	case report.RewardRuntimeStatus == "FAIL" || report.ArchitectureStatus == "FAIL":
		report.Status = "FAIL"
	case report.RewardRuntimeStatus == "INCOMPLETE":
		report.Status = "INCOMPLETE"
	default:
		report.Status = "PASS"
	}
}

func hasCriticalFinding(findings []finding) bool {
	for _, item := range findings {
		if item.Severity == "CRITICAL" {
			return true
		}
	}
	return false
}

func hasWarningFinding(findings []finding) bool {
	for _, item := range findings {
		if item.Severity == "WARNING" {
			return true
		}
	}
	return false
}

func (runner *auditRunner) eraResults() []eraResult {
	eras := make([]uint64, 0, len(runner.eraStats))
	for era := range runner.eraStats {
		eras = append(eras, era)
	}
	sort.Slice(eras, func(i, j int) bool { return eras[i] < eras[j] })
	results := make([]eraResult, 0, len(eras))
	for _, era := range eras {
		stat := runner.eraStats[era]
		difference := new(big.Int).Sub(cloneBig(stat.observed), stat.expected)
		results = append(results, eraResult{
			Era:                 era,
			BlocksScanned:       stat.blocks,
			FirstScannedBlock:   stat.first,
			LastScannedBlock:    stat.last,
			RewardPerBlockWei:   stat.reward.String(),
			RewardPerBlockRAB:   formatRAB(stat.reward),
			ExpectedEmissionWei: stat.expected.String(),
			ObservedEmissionWei: stat.observed.String(),
			DifferenceWei:       difference.String(),
		})
	}
	return results
}

func (runner *auditRunner) participantResults() []participantResult {
	addresses := make([]common.Address, 0, len(runner.participantStats))
	for address := range runner.participantStats {
		addresses = append(addresses, address)
	}
	sort.Slice(addresses, func(i, j int) bool { return addresses[i].Hex() < addresses[j].Hex() })
	results := make([]participantResult, 0, len(addresses))
	for _, address := range addresses {
		stat := runner.participantStats[address]
		expected := new(big.Int).Add(cloneBig(stat.expectedProducer), stat.expectedCommittee)
		difference := new(big.Int).Sub(cloneBig(stat.observed), expected)
		results = append(results, participantResult{
			Address:                    address.Hex(),
			ProducerBlocks:             stat.producerBlocks,
			CommitteeAssignments:       stat.committeeBlocks,
			ExpectedProducerRewardWei:  stat.expectedProducer.String(),
			ExpectedCommitteeRewardWei: stat.expectedCommittee.String(),
			ExpectedTotalRewardWei:     expected.String(),
			ObservedEmissionWei:        stat.observed.String(),
			DifferenceWei:              difference.String(),
		})
	}
	return results
}

func (runner *auditRunner) halvingResults(from, to uint64) []halvingResult {
	results := make([]halvingResult, 0, 3)
	for era := uint64(1); era <= 3; era++ {
		boundary := era * runner.eraLength
		beforeReward, _ := rewardForBlock(boundary-1, runner.eraLength)
		afterReward, _ := rewardForBlock(boundary, runner.eraLength)
		covered := from <= boundary-1 && to >= boundary
		result := halvingResult{
			FromEra:             era - 1,
			ToEra:               era,
			BoundaryBlock:       boundary,
			RewardBeforeWei:     beforeReward.String(),
			RewardAtBoundaryWei: afterReward.String(),
			CoveredByScan:       covered,
		}
		if covered {
			beforeObserved := runner.observedByBlock[boundary-1]
			afterObserved := runner.observedByBlock[boundary]
			match := beforeObserved != nil && afterObserved != nil && beforeObserved.Cmp(beforeReward) == 0 && afterObserved.Cmp(afterReward) == 0
			result.ObservedMatch = &match
		}
		results = append(results, result)
	}
	return results
}

func (runner *auditRunner) supplyResults(from, to uint64) supplyReport {
	genesisTotal := new(big.Int)
	for _, account := range runner.genesis.Alloc {
		genesisTotal.Add(genesisTotal, account.Balance)
	}
	expectedScanned := scheduledRewards(from, to, runner.eraLength)
	observedScanned := new(big.Int)
	for _, amount := range runner.observedByBlock {
		observedScanned.Add(observedScanned, amount)
	}
	difference := new(big.Int).Sub(cloneBig(observedScanned), expectedScanned)
	scheduledThrough := scheduledRewards(1, to, runner.eraLength)
	genesisPlusScheduled := new(big.Int).Add(cloneBig(genesisTotal), scheduledThrough)
	terminal := rewardByEra[len(rewardByEra)-1]
	return supplyReport{
		GenesisAllocationWei:          genesisTotal.String(),
		GenesisAllocationRAB:          formatRAB(genesisTotal),
		ExpectedScannedEmissionWei:    expectedScanned.String(),
		ObservedScannedEmissionWei:    observedScanned.String(),
		ScannedDifferenceWei:          difference.String(),
		ScheduledEmissionThroughToWei: scheduledThrough.String(),
		ScheduledEmissionThroughToRAB: formatRAB(scheduledThrough),
		GenesisPlusScheduledWei:       genesisPlusScheduled.String(),
		GenesisPlusScheduledRAB:       formatRAB(genesisPlusScheduled),
		TerminalRewardWei:             terminal.String(),
		TerminalRewardRAB:             formatRAB(terminal),
		CappedSupply:                  false,
	}
}

func uniqueSortedAddresses(input []common.Address) []common.Address {
	seen := make(map[common.Address]struct{})
	for _, address := range input {
		if address != (common.Address{}) {
			seen[address] = struct{}{}
		}
	}
	result := make([]common.Address, 0, len(seen))
	for address := range seen {
		result = append(result, address)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Hex() < result[j].Hex() })
	return result
}

func containsAddress(addresses []common.Address, target common.Address) bool {
	for _, address := range addresses {
		if address == target {
			return true
		}
	}
	return false
}

func addressPosition(addresses []common.Address, target common.Address) int {
	for index, address := range addresses {
		if address == target {
			return index
		}
	}
	return -1
}

func addressStrings(addresses []common.Address) []string {
	result := make([]string, len(addresses))
	for i, address := range addresses {
		result[i] = address.Hex()
	}
	return result
}

func roleOrUnexpected(role string) string {
	if role == "" {
		return "unexpected"
	}
	return role
}
