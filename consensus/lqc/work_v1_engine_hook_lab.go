//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package lqc

import (
	"errors"
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto/rabbitx"
)

var (
	ErrWorkV1EngineLabUnavailable = errors.New(
		"lqc Work V1 engine laboratory runtime unavailable",
	)
	ErrWorkV1EngineLabParentMissing = errors.New(
		"lqc Work V1 engine laboratory parent runtime missing",
	)
)

type WorkV1EngineLabTicketProvider func(
	blockNumber uint64,
	commitEpoch uint64,
) ([]SignedRandomXWorkTicketV1, error)

type workV1EngineLabRuntime struct {
	mu       sync.Mutex
	hasher   WorkRelayHasherV1
	close    func()
	runtimes map[common.Hash]*CanonicalWorkRuntimeStateV1
	provider WorkV1EngineLabTicketProvider
}

var workV1EngineLabRuntimes sync.Map

func workV1EngineLabRuntimeFor(
	engine *LQC,
) (*workV1EngineLabRuntime, error) {
	if engine == nil {
		return nil, ErrWorkV1EngineLabUnavailable
	}
	if existing, ok := workV1EngineLabRuntimes.Load(engine); ok {
		return existing.(*workV1EngineLabRuntime), nil
	}

	hasher, err := rabbitx.NewLightHasher()
	if err != nil {
		return nil, err
	}
	created := &workV1EngineLabRuntime{
		hasher:   hasher.Hash,
		close:    hasher.Close,
		runtimes: make(map[common.Hash]*CanonicalWorkRuntimeStateV1),
	}
	actual, loaded := workV1EngineLabRuntimes.LoadOrStore(
		engine,
		created,
	)
	if loaded {
		hasher.Close()
		return actual.(*workV1EngineLabRuntime), nil
	}
	return created, nil
}

func SetWorkV1EngineLabTicketProvider(
	engine *LQC,
	provider WorkV1EngineLabTicketProvider,
) error {
	state, err := workV1EngineLabRuntimeFor(engine)
	if err != nil {
		return err
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	state.provider = provider
	return nil
}

// WorkV1EngineLabRelayContext exposes the exact canonical context used by the
// active Header V3 engine for lqcw admission. The transport must not maintain
// an independent difficulty or eligibility policy.
func (l *LQC) WorkV1EngineLabRelayContext(
	chain consensus.ChainHeaderReader,
	parentNumber uint64,
	parentHash common.Hash,
	blockNumber uint64,
) (
	uint64,
	common.Hash,
	common.Hash,
	*big.Int,
	WorkRelayEligibilityCheckV1,
	error,
) {
	if chain == nil ||
		chain.Config() == nil ||
		chain.Config().ChainID == nil ||
		parentNumber == ^uint64(0) ||
		blockNumber != parentNumber+1 {
		return 0, common.Hash{}, common.Hash{}, nil, nil,
			ErrWorkV1EngineLabUnavailable
	}

	parent, err := l.workV1EngineLabRuntimeAt(
		chain,
		parentNumber,
		parentHash,
	)
	if err != nil {
		return 0, common.Hash{}, common.Hash{}, nil, nil, err
	}
	epoch, hasCommit, err := WorkCommitTargetEpochV1(
		blockNumber,
		parent.Work.EpochLength,
	)
	if err != nil {
		return 0, common.Hash{}, common.Hash{}, nil, nil, err
	}
	if !hasCommit {
		return 0, common.Hash{}, common.Hash{}, nil, nil, nil
	}

	ctx, err := l.workV1EngineLabContext(
		chain,
		parent,
		blockNumber,
		common.Hash{},
	)
	if err != nil {
		return 0, common.Hash{}, common.Hash{}, nil, nil, err
	}
	difficulty, hasDifficulty, err := parent.CommitDifficultyV1(
		chain.Config().ChainID,
		blockNumber,
	)
	if err != nil {
		return 0, common.Hash{}, common.Hash{}, nil, nil, err
	}
	if !hasDifficulty || difficulty == nil || difficulty.Sign() <= 0 {
		return 0, common.Hash{}, common.Hash{}, nil, nil,
			ErrWorkV1EngineLabUnavailable
	}
	seated := make(map[common.Address]struct{}, len(parent.Work.SelectionSeats))
	for _, seat := range parent.Work.SelectionSeats {
		seated[seat.Participant] = struct{}{}
	}
	baseEligibility := ctx.Eligibility
	admissionEligibility := func(participant common.Address) error {
		if _, exists := seated[participant]; exists {
			return ErrWorkParticipantAlreadySeatedV1
		}
		return baseEligibility(participant)
	}
	return epoch,
		ctx.DatasetAnchor,
		ctx.ChallengeAnchor,
		new(big.Int).Set(difficulty),
		admissionEligibility,
		nil
}

// WorkV2ParticipantSeatStatus returns the canonical admission state at a
// specific head. It is intentionally read-only and branch-aware so user-facing
// software never mistakes a local relay acceptance for a consensus seat.
func (l *LQC) WorkV2ParticipantSeatStatus(
	chain consensus.ChainHeaderReader,
	parentNumber uint64,
	parentHash common.Hash,
	participant common.Address,
) (
	selectionEpoch uint64,
	seatCount uint64,
	active bool,
	committed bool,
	err error,
) {
	if chain == nil || participant == (common.Address{}) ||
		parentHash == (common.Hash{}) {
		return 0, 0, false, false, ErrWorkV1EngineLabUnavailable
	}
	runtime, err := l.workV1EngineLabRuntimeAt(
		chain,
		parentNumber,
		parentHash,
	)
	if err != nil {
		return 0, 0, false, false, err
	}
	for _, seat := range runtime.Work.SelectionSeats {
		if seat.Participant == participant {
			active = true
			break
		}
	}
	for _, seat := range runtime.Work.CommitSeats {
		if seat.Participant == participant {
			committed = true
			break
		}
	}
	return runtime.Work.SelectionEpoch,
		uint64(len(runtime.Work.SelectionSeats)),
		active,
		committed,
		nil
}

func (l *LQC) workV1EngineLabContext(
	chain consensus.ChainHeaderReader,
	parent *CanonicalWorkRuntimeStateV1,
	blockNumber uint64,
	registryRoot common.Hash,
) (LQCHeaderWorkRuntimeContextV1, error) {
	if chain == nil ||
		chain.Config() == nil ||
		chain.Config().ChainID == nil ||
		parent == nil {
		return LQCHeaderWorkRuntimeContextV1{},
			ErrWorkV1EngineLabUnavailable
	}

	ctx := LQCHeaderWorkRuntimeContextV1{
		ChainID:      new(big.Int).Set(chain.Config().ChainID),
		Parent:       parent,
		BlockNumber:  blockNumber,
		RegistryRoot: registryRoot,
		Eligibility: func(common.Address) error {
			return nil
		},
	}

	state, err := workV1EngineLabRuntimeFor(l)
	if err != nil {
		return LQCHeaderWorkRuntimeContextV1{}, err
	}
	ctx.Hasher = state.hasher

	epoch, hasCommit, err := WorkCommitTargetEpochV1(
		blockNumber,
		parent.Work.EpochLength,
	)
	if err != nil {
		return LQCHeaderWorkRuntimeContextV1{}, err
	}
	if !hasCommit {
		return ctx, nil
	}

	datasetNumber, err := WorkDatasetAnchorBlockV1(
		epoch,
		parent.Work.EpochLength,
	)
	if err != nil {
		return LQCHeaderWorkRuntimeContextV1{}, err
	}
	challengeNumber, err := WorkChallengeAnchorBlockV1(
		epoch,
		parent.Work.EpochLength,
	)
	if err != nil {
		return LQCHeaderWorkRuntimeContextV1{}, err
	}

	dataset := workV1EngineLabAncestorHeader(
		chain,
		parent.Work.Number,
		parent.Work.Hash,
		datasetNumber,
	)
	challenge := workV1EngineLabAncestorHeader(
		chain,
		parent.Work.Number,
		parent.Work.Hash,
		challengeNumber,
	)
	if dataset == nil || challenge == nil {
		return LQCHeaderWorkRuntimeContextV1{},
			ErrWorkV1EngineLabParentMissing
	}

	ctx.DatasetAnchor = dataset.Hash()
	ctx.ChallengeAnchor = challenge.Hash()

	ctx.Eligibility = workV2EngineLabAdmissionEligibility()
	return ctx, nil
}

func (l *LQC) workV1EngineLabRemember(
	headerHash common.Hash,
	runtime *CanonicalWorkRuntimeStateV1,
) error {
	if headerHash == (common.Hash{}) || runtime == nil {
		return ErrWorkV1EngineLabUnavailable
	}
	state, err := workV1EngineLabRuntimeFor(l)
	if err != nil {
		return err
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	state.runtimes[headerHash] = runtime
	return nil
}

func (l *LQC) workV1EngineLabCached(
	hash common.Hash,
) (*CanonicalWorkRuntimeStateV1, bool, error) {
	state, err := workV1EngineLabRuntimeFor(l)
	if err != nil {
		return nil, false, err
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	runtime, ok := state.runtimes[hash]
	return runtime, ok, nil
}

// workV1EngineLabAncestorHeader resolves anchors on the runtime's own branch.
// Height-only lookup is unsafe after a reorg because it silently jumps to the
// canonical header at that height.
func workV1EngineLabAncestorHeader(
	chain consensus.ChainHeaderReader,
	fromNumber uint64,
	fromHash common.Hash,
	targetNumber uint64,
) *types.Header {
	if chain == nil ||
		fromHash == (common.Hash{}) ||
		targetNumber > fromNumber {
		return nil
	}

	currentNumber := fromNumber
	currentHash := fromHash
	for {
		header := chain.GetHeader(currentHash, currentNumber)
		if header == nil ||
			header.Number == nil ||
			!header.Number.IsUint64() ||
			header.Number.Uint64() != currentNumber {
			return nil
		}
		if currentNumber == targetNumber {
			return header
		}
		currentHash = header.ParentHash
		currentNumber--
	}
}

func (l *LQC) workV1EngineLabGenesisRuntime(
	chain consensus.ChainHeaderReader,
) (*CanonicalWorkRuntimeStateV1, error) {
	if chain == nil ||
		chain.Config() == nil ||
		chain.Config().ChainID == nil {
		return nil, ErrWorkV1EngineLabUnavailable
	}
	genesis := chain.GetHeaderByNumber(0)
	if genesis == nil {
		return nil, ErrWorkV1EngineLabParentMissing
	}
	if cached, ok, err := l.workV1EngineLabCached(
		genesis.Hash(),
	); err != nil {
		return nil, err
	} else if ok {
		return cached, nil
	}

	if l == nil || l.config == nil || l.config.ProofDifficulty == 0 {
		return nil, ErrInvalidWorkDifficultyV1
	}
	runtime, err := NewCanonicalWorkRuntimeStateV1(
		chain.Config().ChainID,
		0,
		genesis.Hash(),
		WorkProtocolEpochLengthV1,
		new(big.Int).SetUint64(l.config.ProofDifficulty),
	)
	if err != nil {
		return nil, err
	}
	if err := l.workV1EngineLabRemember(
		genesis.Hash(),
		runtime,
	); err != nil {
		return nil, err
	}
	return runtime, nil
}

// workV1EngineLabReplayRegistryV3 rebuilds the canonical registry snapshot in
// lockstep with Work V1 runtime replay. Header V3 deliberately shares the V2
// registry magic, so the generic registry replay cannot decode it as a V2
// envelope after a process restart. Keeping both snapshots in lockstep also
// preserves the WorkSeat authorization rules once seats become active.
func (l *LQC) workV1EngineLabReplayRegistryV3(
	chain consensus.ChainHeaderReader,
	parentRuntime *CanonicalWorkRuntimeStateV1,
	header *types.Header,
	envelope LQCHeaderEnvelopeV3,
) (*RegistrySnapshot, error) {
	if chain == nil ||
		chain.Config() == nil ||
		chain.Config().ChainID == nil ||
		parentRuntime == nil ||
		header == nil ||
		header.Number == nil ||
		header.Number.Sign() <= 0 {
		return nil, ErrRegistrySnapshotChainMismatch
	}
	parent, err := l.registrySnapshotAt(
		chain,
		header.Number.Uint64()-1,
		header.ParentHash,
	)
	if err != nil {
		return nil, err
	}
	var (
		selection HybridSelection
	)
	if !l.openActivationForHeader(chain, header) {
		selection, _, err = l.workV1EngineLabSelectionForHeader(
			chain,
			parentRuntime,
			parent,
			header,
		)
		if err != nil {
			return nil, err
		}
	}
	if len(selection.Ordered) > 0 {
		return l.workV1EngineLabApplyRegistryBySeats(
			chain,
			parent,
			header,
			envelope,
			selection,
		)
	}

	v2Extra, err := EncodeRegistryHeaderExtra(
		envelope.BlockNumber,
		envelope.RegistryRoot,
		envelope.RegistryOperations,
	)
	if err != nil {
		return nil, err
	}
	synthetic := types.CopyHeader(header)
	synthetic.Extra = v2Extra
	snapshot, err := parent.ApplyHeaderWithOpenActivation(
		chain.Config().ChainID,
		l.registryRules(),
		synthetic,
		l.openActivationForHeader(chain, header),
	)
	if err != nil {
		return nil, err
	}
	return rekeyWorkV1EngineLabRegistrySnapshot(header, snapshot)
}

func (l *LQC) workV1EngineLabRuntimeAt(
	chain consensus.ChainHeaderReader,
	number uint64,
	hash common.Hash,
) (*CanonicalWorkRuntimeStateV1, error) {
	if chain == nil || hash == (common.Hash{}) {
		return nil, ErrWorkV1EngineLabParentMissing
	}
	if cached, ok, err := l.workV1EngineLabCached(hash); err != nil {
		return nil, err
	} else if ok && cached.Work.Number == number {
		return cached, nil
	}

	currentNumber := number
	currentHash := hash
	pending := make([]*types.Header, 0)
	var runtime *CanonicalWorkRuntimeStateV1

	for {
		cached, ok, err := l.workV1EngineLabCached(currentHash)
		if err != nil {
			return nil, err
		}
		if ok && cached.Work.Number == currentNumber {
			runtime = cached
			break
		}
		if currentNumber == 0 {
			genesisRuntime, genesisErr := l.workV1EngineLabGenesisRuntime(
				chain,
			)
			if genesisErr != nil {
				return nil, genesisErr
			}
			runtime = genesisRuntime
			if runtime.Work.Hash != currentHash {
				return nil, ErrWorkV1EngineLabParentMissing
			}
			break
		}
		header := chain.GetHeader(currentHash, currentNumber)
		if header == nil ||
			header.Number == nil ||
			!header.Number.IsUint64() ||
			header.Number.Uint64() != currentNumber {
			return nil, ErrWorkV1EngineLabParentMissing
		}
		pending = append(pending, header)
		currentHash = header.ParentHash
		currentNumber--
	}

	for index := len(pending) - 1; index >= 0; index-- {
		header := pending[index]
		current := header.Number.Uint64()

		if envelope, decodeErr := DecodeLQCHeaderExtraV3(
			header.Extra,
			MaxWorkTicketsPerBlockV1,
		); decodeErr == nil {
			registrySnapshot, err := l.workV1EngineLabReplayRegistryV3(
				chain,
				runtime,
				header,
				envelope,
			)
			if err != nil {
				return nil, err
			}
			ctx, err := l.workV1EngineLabContext(
				chain,
				runtime,
				current,
				envelope.RegistryRoot,
			)
			if err != nil {
				return nil, err
			}
			_, next, err :=
				ValidateAndApplyLQCHeaderExtraV3WithCanonicalWorkV1(
					ctx,
					header.Hash(),
					header.Extra,
				)
			if err != nil {
				return nil, err
			}
			runtime = next
			l.rememberRegistrySnapshot(registrySnapshot)
		} else {
			_, hasCommit, err := WorkCommitTargetEpochV1(
				current,
				runtime.Work.EpochLength,
			)
			if err != nil {
				return nil, err
			}
			if hasCommit {
				return nil, ErrWorkV1EngineLabParentMissing
			}
			next, applyErr := runtime.ApplyVerifiedBlockV1(
				chain.Config().ChainID,
				current,
				header.Hash(),
				runtime.Work.Hash,
				common.Hash{},
				nil,
			)
			if applyErr != nil {
				return nil, applyErr
			}
			runtime = next
		}
		if err := l.workV1EngineLabRemember(
			header.Hash(),
			runtime,
		); err != nil {
			return nil, err
		}
	}

	if runtime.Work.Number != number || runtime.Work.Hash != hash {
		return nil, ErrWorkV1EngineLabParentMissing
	}
	return runtime, nil
}

func (l *LQC) prepareWorkV1EngineLabHook(
	chain consensus.ChainHeaderReader,
	header *types.Header,
) error {
	if header == nil ||
		header.Number == nil ||
		header.Number.Sign() <= 0 {
		return nil
	}
	if !l.registryProtocolEnabled(header.Number) {
		return nil
	}

	registryEnvelope, err := DecodeRegistryHeaderExtra(header.Extra)
	if err != nil {
		return err
	}

	parent, err := l.workV1EngineLabRuntimeAt(
		chain,
		header.Number.Uint64()-1,
		header.ParentHash,
	)
	if err != nil {
		return err
	}
	ctx, err := l.workV1EngineLabContext(
		chain,
		parent,
		header.Number.Uint64(),
		registryEnvelope.RegistryRoot,
	)
	if err != nil {
		return err
	}

	var tickets []SignedRandomXWorkTicketV1
	state, err := workV1EngineLabRuntimeFor(l)
	if err != nil {
		return err
	}
	state.mu.Lock()
	provider := state.provider
	state.mu.Unlock()

	if provider != nil {
		epoch, ok, err := WorkCommitTargetEpochV1(
			header.Number.Uint64(),
			parent.Work.EpochLength,
		)
		if err != nil {
			return err
		}
		if ok {
			tickets, err = provider(
				header.Number.Uint64(),
				epoch,
			)
			if err != nil {
				return err
			}
		}
	}

	extra, _, err := BuildLQCHeaderExtraV3WithCanonicalWorkV1(
		ctx,
		registryEnvelope.Operations,
		tickets,
	)
	if err != nil {
		return err
	}
	header.Extra = extra
	return nil
}

func (l *LQC) verifyCanonicalRegistryHeaderMaybeWorkV1Lab(
	chain consensus.ChainHeaderReader,
	header *types.Header,
) (HybridSelection, *RegistrySnapshot, error) {
	if header == nil ||
		header.Number == nil ||
		!l.registryProtocolEnabled(header.Number) {
		return l.verifyCanonicalRegistryHeader(chain, header)
	}

	envelope, err := DecodeLQCHeaderExtraV3(
		header.Extra,
		MaxWorkTicketsPerBlockV1,
	)
	if err != nil {
		return l.verifyCanonicalRegistryHeader(chain, header)
	}

	parentRuntime, err := l.workV1EngineLabRuntimeAt(
		chain,
		header.Number.Uint64()-1,
		header.ParentHash,
	)
	if err != nil {
		return HybridSelection{}, nil, err
	}
	parentRegistry, err := l.registryParentSnapshot(
		chain,
		header,
	)
	if err != nil {
		return HybridSelection{}, nil, err
	}

	var selection HybridSelection
	if l.openActivationForHeader(chain, header) {
		selection.Ordered = []HybridParticipant{{
			Address:       header.Coinbase,
			Payout:        header.Coinbase,
			Bond:          big.NewInt(25),
			RegisteredAt:  header.Number.Uint64(),
			LastHeartbeat: header.Number.Uint64(),
			Status:        ParticipantActiveCandidate,
		}}
		selection.Producer = &selection.Ordered[0]
	} else {
		selection, _, err = l.workV1EngineLabSelectionForHeader(
			chain,
			parentRuntime,
			parentRegistry,
			header,
		)
		if err != nil {
			return HybridSelection{}, nil, err
		}
	}

	var registrySnapshot *RegistrySnapshot
	if !l.openActivationForHeader(chain, header) &&
		len(selection.Ordered) > 0 {
		registrySnapshot, err =
			l.workV1EngineLabApplyRegistryBySeats(
				chain,
				parentRegistry,
				header,
				envelope,
				selection,
			)
		if err != nil {
			return HybridSelection{}, nil, err
		}
	} else {
		v2Extra, err := EncodeRegistryHeaderExtra(
			envelope.BlockNumber,
			envelope.RegistryRoot,
			envelope.RegistryOperations,
		)
		if err != nil {
			return HybridSelection{}, nil, err
		}
		synthetic := types.CopyHeader(header)
		synthetic.Extra = v2Extra

		var syntheticSnapshot *RegistrySnapshot
		selection, syntheticSnapshot, err =
			l.verifyCanonicalRegistryHeader(
				chain,
				synthetic,
			)
		if err != nil {
			return HybridSelection{}, nil, err
		}
		registrySnapshot, err =
			rekeyWorkV1EngineLabRegistrySnapshot(
				header,
				syntheticSnapshot,
			)
		if err != nil {
			return HybridSelection{}, nil, err
		}
	}

	ctx, err := l.workV1EngineLabContext(
		chain,
		parentRuntime,
		header.Number.Uint64(),
		envelope.RegistryRoot,
	)
	if err != nil {
		return HybridSelection{}, nil, err
	}
	_, next, err :=
		ValidateAndApplyLQCHeaderExtraV3WithCanonicalWorkV1(
			ctx,
			header.Hash(),
			header.Extra,
		)
	if err != nil {
		return HybridSelection{}, nil, err
	}
	if err := l.workV1EngineLabRemember(
		header.Hash(),
		next,
	); err != nil {
		return HybridSelection{}, nil, err
	}
	l.rememberRegistrySnapshot(registrySnapshot)
	return selection, registrySnapshot, nil
}
