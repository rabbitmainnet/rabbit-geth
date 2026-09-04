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
	ErrWorkV2AdmissionParticipantInvalid = errors.New(
		"lqc Work V2 admission participant is invalid",
	)
	ErrWorkV1EngineLabSelectionUnavailable = errors.New(
		"lqc Work V1 seat selection unavailable",
	)
)

// workV2EngineLabAdmissionEligibility keeps Work V2 admission permissionless.
// Ownership is proven by the signed ticket and admission cost by RandomX. A
// participant that already owns a persistent seat is rejected separately by
// the canonical runtime, so registry membership must not be a prerequisite
// for obtaining the first seat.
func workV2EngineLabAdmissionEligibility() WorkRelayEligibilityCheckV1 {
	return func(address common.Address) error {
		if address == (common.Address{}) {
			return ErrWorkV2AdmissionParticipantInvalid
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

	// A canonical Work V2 seat is persistent consensus eligibility. Registry
	// membership is required when the admission proof is accepted, but a later
	// permissionless recovery must not silently strand an already-owned seat.
	eligibleSeats := make([]WorkSeatV1, 0, len(seats))
	for _, seat := range seats {
		if seat.TicketHash == (common.Hash{}) ||
			seat.Participant == (common.Address{}) {
			return HybridSelection{}, false,
				ErrInvalidWorkSelectionV1
		}
		eligibleSeats = append(eligibleSeats, seat)
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
		selection, err := workV1EngineLabActivationFallback(
			parentRegistry,
			header,
		)
		return selection, false, err
	}
	if parent.Work.SelectionEpoch != sourceEpoch ||
		parent.Work.SelectionRoot == (common.Hash{}) {
		return HybridSelection{}, false,
			ErrWorkV1EngineLabSelectionUnavailable
	}

	// Block 1 and post-timeout recovery install a sequence-zero activation
	// anchor. Keep that anchor's temporary production lease until its own
	// admission becomes a canonical persistent seat. Otherwise stale offline
	// seats could stop the recovering chain before the N+2 admission delay ends.
	lease, err := workV2EngineLabActivationLease(
		parentRegistry,
		header,
		parent.Work.SelectionSeats,
	)
	if err != nil {
		return HybridSelection{}, false, err
	}
	if lease.Producer != nil {
		return lease, false, nil
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

	selection, active, err := l.workV1EngineLabBuildSeatSelection(
		chain.Config().ChainID,
		sourceEpoch,
		parent.Work.SelectionRoot,
		parent.Work.SelectionSeats,
		datasetKey,
		header.Number.Uint64(),
		registry,
		l.registryRules(),
		WorkSelectionBeaconHasherV1(state.cachedSelectionBeaconHash),
	)
	if err != nil || active {
		return selection, active, err
	}
	selection, err = workV1EngineLabActivationFallback(
		parentRegistry,
		header,
	)
	return selection, false, err
}

func workV2EngineLabActivationLease(
	parent *RegistrySnapshot,
	header *types.Header,
	seats []WorkSeatV1,
) (HybridSelection, error) {
	selection, err := workV1EngineLabActivationFallback(parent, header)
	if err != nil || selection.Producer == nil {
		return HybridSelection{}, err
	}
	for _, seat := range seats {
		if seat.Participant == selection.Producer.Address {
			return HybridSelection{}, nil
		}
	}
	return selection, nil
}

// workV1EngineLabActivationFallback keeps bootstrap and zero-work liveness
// without turning every registered address into a free consensus seat. Only
// the sequence-zero activation identity may produce until WorkSeats exist.
// If that identity disappears, the permissionless recovery timeout reopens
// activation for a new address.
func workV1EngineLabActivationFallback(
	parent *RegistrySnapshot,
	header *types.Header,
) (HybridSelection, error) {
	if parent == nil || header == nil || header.Number == nil {
		return HybridSelection{}, ErrWorkV1EngineLabSelectionUnavailable
	}
	registry, err := parent.Registry()
	if err != nil {
		return HybridSelection{}, err
	}
	anchors := make([]HybridParticipant, 0, 1)
	for _, participant := range registry.Participants() {
		if !participant.Active || participant.Sequence != 0 {
			continue
		}
		anchors = append(anchors, HybridParticipant{
			Address:       participant.Address,
			Payout:        participant.Address,
			Bond:          big.NewInt(0),
			RegisteredAt:  participant.RegisteredAt,
			LastHeartbeat: participant.LastHeartbeat,
			JailedUntil:   participant.JailedUntil,
			MissedTurns:   participant.MissedTurns,
			Status:        ParticipantActiveCandidate,
		})
	}
	ordered := DeterministicallyOrderParticipants(
		anchors,
		header.ParentHash,
		header.Number.Uint64(),
	)
	selection := HybridSelection{}
	if len(ordered) == 0 {
		return selection, nil
	}
	selection.Ordered = ordered[:1]
	selection.Producer = &selection.Ordered[0]
	return selection, nil
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

	parentRuntime, err := l.workV1EngineLabRuntimeAt(
		chain,
		header.Number.Uint64()-1,
		header.ParentHash,
	)
	if err != nil {
		return HybridSelection{}
	}
	parentRegistry, err := l.registryParentSnapshot(
		chain,
		header,
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
	if active || len(selection.Ordered) > 0 {
		return selection
	}
	return HybridSelection{}
}

// Work-seat mode deliberately does NOT apply the legacy registry
// address-strike rule. Eligibility and role assignment come from the canonical
// unique-wallet WorkSeat set for the source epoch.
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
	if l.openActivationForHeader(chain, header) {
		selection, err := l.prepareCanonicalRegistryExtra(chain, header)
		if err != nil {
			return HybridSelection{}, err
		}
		if err := l.prepareWorkV1EngineLabHook(chain, header); err != nil {
			return HybridSelection{}, err
		}
		return selection, nil
	}
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

	parentRuntime, err := l.workV1EngineLabRuntimeAt(
		chain,
		header.Number.Uint64()-1,
		header.ParentHash,
	)
	if err != nil {
		return HybridSelection{}, err
	}
	parentRegistry, err := l.registryParentSnapshot(
		chain,
		header,
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
	if !active && len(selection.Ordered) == 0 {
		return HybridSelection{}, ErrWorkV1EngineLabSelectionUnavailable
	}
	if err := l.workV1EngineLabPrepareRegistryBySeats(
		chain,
		parentRegistry,
		header,
		selection,
	); err != nil {
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
