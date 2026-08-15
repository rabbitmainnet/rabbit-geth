//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package lqc

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core/types"
)

var (
	ErrWorkV1EngineLabHistoricalIneligible = errors.New(
		"lqc Work V1 ticket participant was not historically eligible",
	)
	ErrWorkV1EngineLabSelectionUnavailable = errors.New(
		"lqc Work V1 seat selection unavailable",
	)
)

func workV1EngineLabHistoricalEligibilityFromRegistry(
	registry *CanonicalRegistry,
	blockNumber uint64,
	rules RegistrySnapshotRules,
) WorkRelayEligibilityCheckV1 {
	return func(address common.Address) error {
		if registry == nil ||
			address == (common.Address{}) ||
			!registry.IsEligibleParticipant(
				address,
				blockNumber,
				rules.ActivationDelay,
				rules.HeartbeatWindow,
				rules.HeartbeatGrace,
			) {
			return ErrWorkV1EngineLabHistoricalIneligible
		}
		return nil
	}
}

// Committee sizing uses eligible WORK SEATS. Explicit CommitteeSize remains a
// test/devnet override; otherwise configured min/max bounds are used.
func (l *LQC) workV1EngineLabCommitteeSize(
	seatCount uint64,
) uint64 {
	if l != nil && l.config != nil && l.config.CommitteeSize > 0 {
		return l.config.CommitteeSize
	}
	minimum := uint64(32)
	maximum := uint64(128)
	if l != nil && l.config != nil {
		if l.config.CommitteeMin > 0 {
			minimum = l.config.CommitteeMin
		}
		if l.config.CommitteeMax > 0 {
			maximum = l.config.CommitteeMax
		}
	}
	return ComputeCommitteeSizeWithBounds(
		seatCount,
		minimum,
		maximum,
	)
}

// workV1EngineLabRoleCounts reserves the requested committee capacity before
// assigning the configured fallback prefix. The whole deterministic Ordered
// queue remains eligible for delayed production through IsAuthorAllowed, so
// shortening the named fallback slice does not reduce LAB liveness.
func workV1EngineLabRoleCounts(
	seatCount uint64,
	fallbackCount uint64,
	committeeSize uint64,
) (uint64, uint64) {
	if seatCount == 0 {
		return 0, 0
	}

	availableAfterProducer := seatCount - 1
	if committeeSize > availableAfterProducer {
		committeeSize = availableAfterProducer
	}
	maxFallbackCount := availableAfterProducer - committeeSize
	if fallbackCount > maxFallbackCount {
		fallbackCount = maxFallbackCount
	}
	return fallbackCount, committeeSize
}

func workV1EngineLabHybridParticipant(
	seat WorkSeatV1,
) HybridParticipant {
	return HybridParticipant{
		Address:     seat.Participant,
		Payout:      seat.Participant,
		Status:      ParticipantActiveCandidate,
		IsBootstrap: false,
	}
}

func workV1EngineLabHybridSelection(
	selection WorkSelectionV1,
) HybridSelection {
	out := HybridSelection{
		Ordered: make(
			[]HybridParticipant,
			0,
			len(selection.Ordered),
		),
		Fallbacks: make(
			[]HybridParticipant,
			0,
			len(selection.Fallbacks),
		),
		Committee: make(
			[]HybridParticipant,
			0,
			len(selection.Committee),
		),
	}
	for _, seat := range selection.Ordered {
		out.Ordered = append(
			out.Ordered,
			workV1EngineLabHybridParticipant(seat),
		)
	}
	if len(out.Ordered) > 0 {
		out.Producer = &out.Ordered[0]
	}
	for _, seat := range selection.Fallbacks {
		out.Fallbacks = append(
			out.Fallbacks,
			workV1EngineLabHybridParticipant(seat),
		)
	}
	for _, seat := range selection.Committee {
		out.Committee = append(
			out.Committee,
			workV1EngineLabHybridParticipant(seat),
		)
	}
	return out
}

// Pure boundary used by tests and by the active LAB wrapper.
func (l *LQC) workV1EngineLabBuildSeatSelection(
	chainID *big.Int,
	sourceEpoch uint64,
	selectionRoot common.Hash,
	seats []WorkSeatV1,
	datasetKey common.Hash,
	blockNumber uint64,
	registry *CanonicalRegistry,
	rules RegistrySnapshotRules,
	hasher WorkSelectionBeaconHasherV1,
) (HybridSelection, bool, error) {
	if chainID == nil ||
		chainID.Sign() <= 0 ||
		sourceEpoch == 0 ||
		selectionRoot == (common.Hash{}) ||
		datasetKey == (common.Hash{}) ||
		blockNumber == 0 ||
		registry == nil ||
		hasher == nil {
		return HybridSelection{}, false,
			ErrWorkV1EngineLabSelectionUnavailable
	}

	// Production zero-work policy: keep registry liveness, but do not activate
	// WorkSeat rewards while no eligible seats exist.
	if len(seats) == 0 {
		return HybridSelection{}, false, nil
	}

	eligibleSeats := make([]WorkSeatV1, 0, len(seats))
	for _, seat := range seats {
		if seat.TicketHash == (common.Hash{}) ||
			seat.Participant == (common.Address{}) {
			return HybridSelection{}, false,
				ErrInvalidWorkSelectionV1
		}
		if registry.IsEligibleParticipant(
			seat.Participant,
			blockNumber,
			rules.ActivationDelay,
			rules.HeartbeatWindow,
			rules.HeartbeatGrace,
		) {
			eligibleSeats = append(eligibleSeats, seat)
		}
	}
	if len(eligibleSeats) == 0 {
		return HybridSelection{}, false, nil
	}

	_, selectionSeed, err := DeterministicWorkSelectionSeedV1(
		chainID,
		sourceEpoch,
		selectionRoot,
		datasetKey,
		blockNumber,
		hasher,
	)
	if err != nil {
		return HybridSelection{}, false, err
	}

	fallbackCount := uint64(0)
	if l != nil {
		fallbackCount = l.hybridCfg().FallbackCount
	}
	committeeSize := l.workV1EngineLabCommitteeSize(
		uint64(len(eligibleSeats)),
	)
	fallbackCount, committeeSize = workV1EngineLabRoleCounts(
		uint64(len(eligibleSeats)),
		fallbackCount,
		committeeSize,
	)

	workSelection, err := BuildWorkSelectionV1(
		eligibleSeats,
		selectionSeed,
		fallbackCount,
		committeeSize,
	)
	if err != nil {
		return HybridSelection{}, false, err
	}
	return workV1EngineLabHybridSelection(workSelection),
		true,
		nil
}

func (l *LQC) workV1EngineLabSelectionForHeader(
	chain consensus.ChainHeaderReader,
	parent *CanonicalWorkRuntimeStateV1,
	parentRegistry *RegistrySnapshot,
	header *types.Header,
) (HybridSelection, bool, error) {
	if chain == nil ||
		chain.Config() == nil ||
		chain.Config().ChainID == nil ||
		parent == nil ||
		parentRegistry == nil ||
		header == nil ||
		header.Number == nil ||
		header.Number.Sign() <= 0 {
		return HybridSelection{}, false,
			ErrWorkV1EngineLabSelectionUnavailable
	}

	sourceEpoch, hasSource, err := WorkSelectionSourceEpochV1(
		header.Number.Uint64(),
		parent.Work.EpochLength,
	)
	if err != nil {
		return HybridSelection{}, false, err
	}
	if !hasSource {
		return HybridSelection{}, false, nil
	}
	if parent.Work.SelectionEpoch != sourceEpoch ||
		parent.Work.SelectionRoot == (common.Hash{}) {
		return HybridSelection{}, false,
			ErrWorkV1EngineLabSelectionUnavailable
	}

	datasetNumber, err := WorkDatasetAnchorBlockV1(
		sourceEpoch,
		parent.Work.EpochLength,
	)
	if err != nil {
		return HybridSelection{}, false, err
	}
	datasetHeader := workV1EngineLabAncestorHeader(
		chain,
		parent.Work.Number,
		parent.Work.Hash,
		datasetNumber,
	)
	if datasetHeader == nil {
		return HybridSelection{}, false,
			ErrWorkV1EngineLabParentMissing
	}
	datasetKey, err := RandomXWorkDatasetKeyV1(
		chain.Config().ChainID,
		sourceEpoch,
		datasetHeader.Hash(),
	)
	if err != nil {
		return HybridSelection{}, false, err
	}

	registry, err := parentRegistry.Registry()
	if err != nil {
		return HybridSelection{}, false, err
	}
	state, err := workV1EngineLabRuntimeFor(l)
	if err != nil {
		return HybridSelection{}, false, err
	}

	return l.workV1EngineLabBuildSeatSelection(
		chain.Config().ChainID,
		sourceEpoch,
		parent.Work.SelectionRoot,
		parent.Work.SelectionSeats,
		datasetKey,
		header.Number.Uint64(),
		registry,
		l.registryRules(),
		WorkSelectionBeaconHasherV1(state.hasher),
	)
}

func (l *LQC) selectionForHeaderMaybeWorkV1Lab(
	chain consensus.ChainHeaderReader,
	header *types.Header,
) HybridSelection {
	if header == nil ||
		header.Number == nil ||
		header.Number.Sign() <= 0 ||
		!l.registryProtocolEnabled(header.Number) {
		return l.selectionForHeader(chain, header)
	}

	parentRegistry, err := l.registryParentSnapshot(
		chain,
		header,
	)
	if err != nil {
		return HybridSelection{}
	}
	parentRuntime, err := l.workV1EngineLabRuntimeAt(
		chain,
		header.Number.Uint64()-1,
		header.ParentHash,
	)
	if err != nil {
		return HybridSelection{}
	}
	selection, active, err := l.workV1EngineLabSelectionForHeader(
		chain,
		parentRuntime,
		parentRegistry,
		header,
	)
	if err != nil {
		return HybridSelection{}
	}
	if active {
		return selection
	}
	return l.selectionForHeader(chain, header)
}

// Work-seat mode deliberately does NOT apply address-based missed-turn
// penalties. Repeated seats make the old address-strike rule invalid.
func (l *LQC) workV1EngineLabPrepareRegistryBySeats(
	chain consensus.ChainHeaderReader,
	parent *RegistrySnapshot,
	header *types.Header,
	selection HybridSelection,
) error {
	if chain == nil ||
		parent == nil ||
		header == nil ||
		header.Number == nil {
		return ErrWorkV1EngineLabSelectionUnavailable
	}
	allowed, _ := IsAuthorAllowed(selection, header.Coinbase)
	if !allowed {
		return fmt.Errorf(
			"%w: %s",
			ErrUnauthorizedRegistryProducer,
			header.Coinbase,
		)
	}

	registry, err := parent.Registry()
	if err != nil {
		return err
	}
	rules := l.registryRules()
	blockNumber := header.Number.Uint64()

	if !registry.IsEligibleParticipant(
		header.Coinbase,
		blockNumber,
		rules.ActivationDelay,
		rules.HeartbeatWindow,
		rules.HeartbeatGrace,
	) {
		return ErrUnauthorizedRegistryProducer
	}
	if err := registry.MarkProducerHeartbeat(
		header.Coinbase,
		blockNumber,
	); err != nil {
		return err
	}

	operations := make(
		[]RegistryOperation,
		0,
		MaxRegistryOperationsPerBlock,
	)
	if l.registryPool != nil {
		l.registryPool.PruneExpired(blockNumber)
		for _, operation := range l.registryPool.Pending(blockNumber) {
			if len(operations) >= MaxRegistryOperationsPerBlock {
				break
			}
			candidate := registry.Clone()
			chainID, chainErr := registryChainID(chain)
			if chainErr != nil {
				return chainErr
			}
			if applyErr := candidate.ApplyOperation(
				chainID,
				blockNumber,
				rules.ProofDifficulty,
				operation,
			); applyErr != nil {
				continue
			}
			registry = candidate
			operations = append(operations, operation)
		}
	}

	header.Extra, err = EncodeRegistryHeaderExtra(
		blockNumber,
		registry.Root(),
		operations,
	)
	return err
}

func (l *LQC) prepareCanonicalRegistryExtraMaybeWorkV1Lab(
	chain consensus.ChainHeaderReader,
	header *types.Header,
) (HybridSelection, error) {
	if header == nil ||
		header.Number == nil ||
		header.Number.Sign() <= 0 ||
		!l.registryProtocolEnabled(header.Number) {
		selection, err := l.prepareCanonicalRegistryExtra(
			chain,
			header,
		)
		if err != nil {
			return HybridSelection{}, err
		}
		if err := l.prepareWorkV1EngineLabHook(
			chain,
			header,
		); err != nil {
			return HybridSelection{}, err
		}
		return selection, nil
	}

	parentRegistry, err := l.registryParentSnapshot(
		chain,
		header,
	)
	if err != nil {
		return HybridSelection{}, err
	}
	parentRuntime, err := l.workV1EngineLabRuntimeAt(
		chain,
		header.Number.Uint64()-1,
		header.ParentHash,
	)
	if err != nil {
		return HybridSelection{}, err
	}

	selection, active, err := l.workV1EngineLabSelectionForHeader(
		chain,
		parentRuntime,
		parentRegistry,
		header,
	)
	if err != nil {
		return HybridSelection{}, err
	}
	if !active {
		selection, err = l.prepareCanonicalRegistryExtra(
			chain,
			header,
		)
		if err != nil {
			return HybridSelection{}, err
		}
	} else {
		if err := l.workV1EngineLabPrepareRegistryBySeats(
			chain,
			parentRegistry,
			header,
			selection,
		); err != nil {
			return HybridSelection{}, err
		}
	}

	if err := l.prepareWorkV1EngineLabHook(
		chain,
		header,
	); err != nil {
		return HybridSelection{}, err
	}
	return selection, nil
}

func rekeyWorkV1EngineLabRegistrySnapshot(
	header *types.Header,
	snapshot *RegistrySnapshot,
) (*RegistrySnapshot, error) {
	if header == nil ||
		header.Number == nil ||
		snapshot == nil {
		return nil, ErrInvalidRegistrySnapshot
	}
	registry, err := snapshot.Registry()
	if err != nil {
		return nil, err
	}
	return newRegistrySnapshot(
		header.Number.Uint64(),
		header.Hash(),
		registry,
	), nil
}

func (l *LQC) workV1EngineLabApplyRegistryBySeats(
	chain consensus.ChainHeaderReader,
	parent *RegistrySnapshot,
	header *types.Header,
	envelope LQCHeaderEnvelopeV3,
	selection HybridSelection,
) (*RegistrySnapshot, error) {
	if chain == nil ||
		chain.Config() == nil ||
		chain.Config().ChainID == nil ||
		parent == nil ||
		header == nil ||
		header.Number == nil ||
		parent.Number == ^uint64(0) ||
		header.Number.Uint64() != parent.Number+1 ||
		header.ParentHash != parent.Hash {
		return nil, ErrRegistrySnapshotChainMismatch
	}
	allowed, _ := IsAuthorAllowed(selection, header.Coinbase)
	if !allowed {
		return nil, ErrUnauthorizedRegistryProducer
	}

	registry, err := parent.Registry()
	if err != nil {
		return nil, err
	}
	rules := l.registryRules()
	blockNumber := header.Number.Uint64()

	if !registry.IsEligibleParticipant(
		header.Coinbase,
		blockNumber,
		rules.ActivationDelay,
		rules.HeartbeatWindow,
		rules.HeartbeatGrace,
	) {
		return nil, ErrUnauthorizedRegistryProducer
	}

	v2Extra, err := EncodeRegistryHeaderExtra(
		envelope.BlockNumber,
		envelope.RegistryRoot,
		envelope.RegistryOperations,
	)
	if err != nil {
		return nil, err
	}
	validated, err := ValidateRegistryHeaderExtra(
		chain.Config().ChainID,
		blockNumber,
		rules.ProofDifficulty,
		v2Extra,
	)
	if err != nil {
		return nil, err
	}

	if err := registry.MarkProducerHeartbeat(
		header.Coinbase,
		blockNumber,
	); err != nil {
		return nil, err
	}
	for _, operation := range validated.Operations {
		if err := registry.ApplyOperation(
			chain.Config().ChainID,
			blockNumber,
			rules.ProofDifficulty,
			operation,
		); err != nil {
			return nil, err
		}
	}
	if registry.Root() != validated.RegistryRoot {
		return nil, ErrRegistryRootMismatch
	}
	return newRegistrySnapshot(
		blockNumber,
		header.Hash(),
		registry,
	), nil
}
