//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package lqc

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
)

type workV1BranchTestChain struct {
	*testHeaderChain
	canonical map[uint64]*types.Header
}

func (c *workV1BranchTestChain) GetHeaderByNumber(
	number uint64,
) *types.Header {
	return c.canonical[number]
}

func TestWorkV1EngineLabRuntimeReconstructsRequestedBranchByHash(
	t *testing.T,
) {
	participants := testParticipants(t, 2)
	config := canonicalRegistryEngineConfig(participants, 1)
	config.RegistryProtocolBlock = 0
	config.ProofDifficulty = 4096
	genesis := &types.Header{
		Number:   big.NewInt(0),
		Time:     100,
		GasLimit: 30_000_000,
	}
	a1 := &types.Header{
		ParentHash: genesis.Hash(),
		Number:     big.NewInt(1),
		Time:       101,
		GasLimit:   genesis.GasLimit,
		Extra:      []byte("branch-a-1"),
	}
	a2 := &types.Header{
		ParentHash: a1.Hash(),
		Number:     big.NewInt(2),
		Time:       102,
		GasLimit:   genesis.GasLimit,
		Extra:      []byte("branch-a-2"),
	}
	b1 := &types.Header{
		ParentHash: genesis.Hash(),
		Number:     big.NewInt(1),
		Time:       103,
		GasLimit:   genesis.GasLimit,
		Extra:      []byte("branch-b-1"),
	}
	b2 := &types.Header{
		ParentHash: b1.Hash(),
		Number:     big.NewInt(2),
		Time:       104,
		GasLimit:   genesis.GasLimit,
		Extra:      []byte("branch-b-2"),
	}
	base := &testHeaderChain{
		config: &params.ChainConfig{
			ChainID: big.NewInt(928),
			LQC:     config,
		},
		headers: map[common.Hash]*types.Header{
			genesis.Hash(): genesis,
			a1.Hash():      a1,
			a2.Hash():      a2,
			b1.Hash():      b1,
			b2.Hash():      b2,
		},
		current: a2,
	}
	chain := &workV1BranchTestChain{
		testHeaderChain: base,
		canonical: map[uint64]*types.Header{
			0: genesis,
			1: a1,
			2: a2,
		},
	}

	engine := New(config, rawdb.NewMemoryDatabase())
	runtime, err := engine.workV1EngineLabRuntimeAt(
		chain,
		2,
		b2.Hash(),
	)
	if err != nil {
		t.Fatalf("reconstruct branch B: %v", err)
	}
	if runtime.Work.Hash != b2.Hash() || runtime.Work.Number != 2 {
		t.Fatalf(
			"runtime=%d/%s want=2/%s",
			runtime.Work.Number,
			runtime.Work.Hash,
			b2.Hash(),
		)
	}
	if runtime.Difficulty.OddDifficulty.Cmp(big.NewInt(4096)) != 0 ||
		runtime.Difficulty.EvenDifficulty.Cmp(big.NewInt(4096)) != 0 {
		t.Fatalf(
			"initial difficulty odd/even=%s/%s want=4096/4096",
			runtime.Difficulty.OddDifficulty,
			runtime.Difficulty.EvenDifficulty,
		)
	}
	if ancestor := workV1EngineLabAncestorHeader(
		chain,
		2,
		b2.Hash(),
		1,
	); ancestor == nil || ancestor.Hash() != b1.Hash() {
		t.Fatal("branch-aware anchor resolver jumped to canonical branch A")
	}
}

func TestRabbitVRFDKGBridgeContextV1DisabledHasNoRuntimeSideEffects(
	t *testing.T,
) {
	config := canonicalRegistryEngineConfig(
		testParticipants(t, 2),
		1,
	)
	config.VRFProtocolBlock = 0

	engine := New(config, rawdb.NewMemoryDatabase())
	genesis := &types.Header{
		Number:   big.NewInt(0),
		Time:     100,
		GasLimit: 30_000_000,
	}
	chain := canonicalRegistryTestChain(config, genesis)

	if _, exists := workV1EngineLabRuntimes.Load(engine); exists {
		t.Fatal("work runtime unexpectedly exists before disabled VRF call")
	}

	bridge, ok, err := engine.RabbitVRFDKGBridgeContextV1(
		chain,
		256,
		common.HexToHash("0xdeadbeef"),
		257,
	)
	if err != nil {
		t.Fatalf("disabled VRF bridge returned error: %v", err)
	}
	if ok {
		t.Fatal("disabled VRF bridge reported active context")
	}
	if bridge.SourceWorkEpoch != 0 ||
		bridge.SessionID != (common.Hash{}) ||
		len(bridge.Members) != 0 {
		t.Fatalf("disabled VRF bridge returned non-zero context: %+v", bridge)
	}

	if _, exists := workV1EngineLabRuntimes.Load(engine); exists {
		t.Fatal("disabled VRF bridge created Work/RandomX runtime side effect")
	}
}

func TestRabbitVRFDKGBridgeContextV1UsesCanonicalWorkRuntime(
	t *testing.T,
) {
	config := canonicalRegistryEngineConfig(
		testParticipants(t, 2),
		1,
	)
	config.EpochLength = WorkProtocolEpochLengthV1
	config.ProofDifficulty = 17
	config.RegistryProtocolBlock = 0
	config.VRFProtocolBlock = 385

	engine := New(config, rawdb.NewMemoryDatabase())
	genesis := &types.Header{
		Number:   big.NewInt(0),
		Time:     100,
		GasLimit: 30_000_000,
	}
	chain := canonicalRegistryTestChain(config, genesis)

	runtime, err := NewCanonicalWorkRuntimeStateV1(
		chain.Config().ChainID,
		0,
		genesis.Hash(),
		WorkProtocolEpochLengthV1,
		big.NewInt(17),
	)
	if err != nil {
		t.Fatal(err)
	}

	parentHash := genesis.Hash()
	for number := uint64(1); number <= 256; number++ {
		header := &types.Header{
			ParentHash: parentHash,
			Number:     new(big.Int).SetUint64(number),
			Time:       100 + number,
			GasLimit:   30_000_000,
		}

		var challenge common.Hash
		var verified []VerifiedRandomXWorkTicketV1
		if number >= 129 {
			challenge = genesis.Hash()
		}
		if number == 129 {
			verified = []VerifiedRandomXWorkTicketV1{
				runtimeVerifiedV1(1, 0),
				runtimeVerifiedV1(1, 1),
			}
		}

		runtime, err = runtime.ApplyVerifiedBlockV1(
			chain.Config().ChainID,
			number,
			header.Hash(),
			runtime.Work.Hash,
			challenge,
			verified,
		)
		if err != nil {
			t.Fatalf("apply block %d: %v", number, err)
		}

		chain.headers[header.Hash()] = header
		chain.current = header
		parentHash = header.Hash()
	}

	if runtime.Work.SelectionEpoch != 1 {
		t.Fatalf(
			"selection epoch=%d want=1",
			runtime.Work.SelectionEpoch,
		)
	}
	if len(runtime.Work.SelectionSeats) != 2 {
		t.Fatalf(
			"selection seats=%d want=2",
			len(runtime.Work.SelectionSeats),
		)
	}

	if err := engine.workV1EngineLabRemember(
		chain.current.Hash(),
		runtime,
	); err != nil {
		t.Fatal(err)
	}

	bridge, ok, err := engine.RabbitVRFDKGBridgeContextV1(
		chain,
		256,
		chain.current.Hash(),
		257,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("active Rabbit VRF bridge context unavailable")
	}

	wantDatasetKey, err := RandomXWorkDatasetKeyV1(
		chain.Config().ChainID,
		1,
		genesis.Hash(),
	)
	if err != nil {
		t.Fatal(err)
	}

	if bridge.SourceWorkEpoch != 1 ||
		bridge.PreparationEpoch != 3 ||
		bridge.TargetVRFEpoch != 4 {
		t.Fatalf(
			"epochs source/prep/target=%d/%d/%d want=1/3/4",
			bridge.SourceWorkEpoch,
			bridge.PreparationEpoch,
			bridge.TargetVRFEpoch,
		)
	}
	if bridge.SelectionRoot != runtime.Work.SelectionRoot {
		t.Fatal("bridge selection root differs from canonical Work runtime")
	}
	if bridge.DatasetKey != wantDatasetKey {
		t.Fatalf(
			"dataset key=%s want=%s",
			bridge.DatasetKey,
			wantDatasetKey,
		)
	}
	if len(bridge.Members) != 2 {
		t.Fatalf("members=%d want=2", len(bridge.Members))
	}
	if bridge.Members[0].ShareID != 1 ||
		bridge.Members[1].ShareID != 2 {
		t.Fatalf(
			"share ids=%d/%d want=1/2",
			bridge.Members[0].ShareID,
			bridge.Members[1].ShareID,
		)
	}
	if bridge.Session.TargetVRFEpoch != 4 ||
		bridge.Session.CommitteeSize != 2 ||
		bridge.Session.CommitteeRoot != bridge.CommitteeRoot ||
		bridge.SessionID == (common.Hash{}) {
		t.Fatalf("unexpected DKG session: %+v", bridge.Session)
	}

	state, err := workV1EngineLabRuntimeFor(engine)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if actual, loaded := workV1EngineLabRuntimes.LoadAndDelete(engine); loaded {
			runtimeState := actual.(*workV1EngineLabRuntime)
			if runtimeState.close != nil {
				runtimeState.close()
			}
		}
	})

	state.mu.Lock()
	cacheEntries := len(state.selectionBeaconCache)
	state.mu.Unlock()
	if cacheEntries != 1 {
		t.Fatalf(
			"selection beacon cache entries=%d want=1",
			cacheEntries,
		)
	}
}

func TestRabbitVRFDKGBridgeContextV1UsesParentBranchDatasetAnchor(
	t *testing.T,
) {
	config := canonicalRegistryEngineConfig(
		testParticipants(t, 2),
		1,
	)
	config.RegistryProtocolBlock = 0
	config.EpochLength = WorkProtocolEpochLengthV1
	config.ProofDifficulty = 17
	config.VRFProtocolBlock = 641

	engine := New(config, rawdb.NewMemoryDatabase())

	genesis := &types.Header{
		Number:   big.NewInt(0),
		Time:     100,
		GasLimit: 30_000_000,
	}

	base := canonicalRegistryTestChain(config, genesis)
	chain := &workV1BranchTestChain{
		testHeaderChain: base,
		canonical:       make(map[uint64]*types.Header),
	}
	chain.canonical[0] = genesis

	runtime, err := NewCanonicalWorkRuntimeStateV1(
		chain.Config().ChainID,
		0,
		genesis.Hash(),
		WorkProtocolEpochLengthV1,
		big.NewInt(17),
	)
	if err != nil {
		t.Fatal(err)
	}

	parentHash := genesis.Hash()
	var canonical128 *types.Header
	var branch128 *types.Header
	var branch256 *types.Header

	for number := uint64(1); number <= 512; number++ {
		if number == 128 {
			canonical128 = &types.Header{
				ParentHash: parentHash,
				Number:     new(big.Int).SetUint64(number),
				Time:       100 + number,
				GasLimit:   30_000_000,
				Extra:      []byte("canonical-a-128"),
			}
			chain.headers[canonical128.Hash()] = canonical128
			chain.canonical[number] = canonical128
		}

		header := &types.Header{
			ParentHash: parentHash,
			Number:     new(big.Int).SetUint64(number),
			Time:       100 + number,
			GasLimit:   30_000_000,
		}
		if number == 128 {
			header.Extra = []byte("branch-b-128")
			branch128 = header
		}

		var challenge common.Hash
		switch {
		case number <= 128:
		case number <= 256:
			challenge = genesis.Hash()
		case number <= 384:
			challenge = branch128.Hash()
		default:
			challenge = branch256.Hash()
		}

		var verified []VerifiedRandomXWorkTicketV1
		if number == 385 {
			verified = []VerifiedRandomXWorkTicketV1{
				runtimeVerifiedV1(3, 0),
				runtimeVerifiedV1(3, 1),
			}
		}

		runtime, err = runtime.ApplyVerifiedBlockV1(
			chain.Config().ChainID,
			number,
			header.Hash(),
			runtime.Work.Hash,
			challenge,
			verified,
		)
		if err != nil {
			t.Fatalf("apply block %d: %v", number, err)
		}

		chain.headers[header.Hash()] = header
		chain.current = header
		parentHash = header.Hash()

		if number == 256 {
			branch256 = header
		}
	}

	if canonical128 == nil || branch128 == nil || branch256 == nil {
		t.Fatal("branch anchor fixture incomplete")
	}
	if canonical128.Hash() == branch128.Hash() {
		t.Fatal("branch dataset anchors unexpectedly match")
	}
	if runtime.Work.SelectionEpoch != 3 {
		t.Fatalf(
			"selection epoch=%d want=3",
			runtime.Work.SelectionEpoch,
		)
	}
	if len(runtime.Work.SelectionSeats) != 2 {
		t.Fatalf(
			"selection seats=%d want=2",
			len(runtime.Work.SelectionSeats),
		)
	}

	if err := engine.workV1EngineLabRemember(
		chain.current.Hash(),
		runtime,
	); err != nil {
		t.Fatal(err)
	}

	bridge, ok, err := engine.RabbitVRFDKGBridgeContextV1(
		chain,
		512,
		chain.current.Hash(),
		513,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("branch Rabbit VRF bridge context unavailable")
	}

	wantBranchKey, err := RandomXWorkDatasetKeyV1(
		chain.Config().ChainID,
		3,
		branch128.Hash(),
	)
	if err != nil {
		t.Fatal(err)
	}
	wrongCanonicalKey, err := RandomXWorkDatasetKeyV1(
		chain.Config().ChainID,
		3,
		canonical128.Hash(),
	)
	if err != nil {
		t.Fatal(err)
	}

	if bridge.DatasetKey != wantBranchKey {
		t.Fatalf(
			"dataset key=%s want parent-branch key=%s",
			bridge.DatasetKey,
			wantBranchKey,
		)
	}
	if bridge.DatasetKey == wrongCanonicalKey {
		t.Fatal("VRF bridge jumped to canonical branch dataset anchor")
	}
	if bridge.SourceWorkEpoch != 3 ||
		bridge.PreparationEpoch != 5 ||
		bridge.TargetVRFEpoch != 6 {
		t.Fatalf(
			"epochs source/prep/target=%d/%d/%d want=3/5/6",
			bridge.SourceWorkEpoch,
			bridge.PreparationEpoch,
			bridge.TargetVRFEpoch,
		)
	}

	t.Cleanup(func() {
		if actual, loaded := workV1EngineLabRuntimes.LoadAndDelete(engine); loaded {
			runtimeState := actual.(*workV1EngineLabRuntime)
			if runtimeState.close != nil {
				runtimeState.close()
			}
		}
	})
}

func TestWorkV2EngineLabRelayContextUsesCanonicalRuntimeAndPermissionlessAdmission(
	t *testing.T,
) {
	participant := common.HexToAddress(
		"0x00000000000000000000000000000000000000a1",
	)
	outsider := common.HexToAddress(
		"0x00000000000000000000000000000000000000b2",
	)
	config := canonicalRegistryEngineConfig(
		[]common.Address{participant},
		1,
	)
	config.EpochLength = WorkProtocolEpochLengthV1
	config.ProofDifficulty = 17

	engine := New(config, rawdb.NewMemoryDatabase())
	genesis := &types.Header{
		Number:   big.NewInt(0),
		Time:     100,
		GasLimit: 30_000_000,
	}
	chain := canonicalRegistryTestChain(config, genesis)
	runtime, err := NewCanonicalWorkRuntimeStateV1(
		chain.Config().ChainID,
		0,
		genesis.Hash(),
		WorkProtocolEpochLengthV1,
		big.NewInt(17),
	)
	if err != nil {
		t.Fatal(err)
	}

	var challenge *types.Header
	parentHash := genesis.Hash()
	for number := uint64(1); number <= WorkProtocolEpochLengthV1; number++ {
		header := &types.Header{
			ParentHash: parentHash,
			Number:     new(big.Int).SetUint64(number),
			Time:       100 + number,
			GasLimit:   30_000_000,
		}
		runtime, err = runtime.ApplyVerifiedBlockV1(
			chain.Config().ChainID,
			number,
			header.Hash(),
			runtime.Work.Hash,
			common.Hash{},
			nil,
		)
		if err != nil {
			t.Fatalf("apply block %d: %v", number, err)
		}
		chain.headers[header.Hash()] = header
		chain.current = header
		parentHash = header.Hash()
		if number == 1 {
			challenge = header
		}
	}
	if challenge == nil {
		t.Fatal("challenge header missing")
	}
	if err := engine.workV1EngineLabRemember(
		chain.current.Hash(),
		runtime,
	); err != nil {
		t.Fatal(err)
	}
	epoch,
		datasetAnchor,
		challengeAnchor,
		difficulty,
		eligibility,
		err := engine.WorkV1EngineLabRelayContext(
		chain,
		WorkProtocolEpochLengthV1,
		chain.current.Hash(),
		WorkProtocolEpochLengthV1+1,
	)
	if err != nil {
		t.Fatal(err)
	}
	if epoch != 1 {
		t.Fatalf("epoch=%d want=1", epoch)
	}
	if datasetAnchor != genesis.Hash() {
		t.Fatalf("dataset=%s want=%s", datasetAnchor, genesis.Hash())
	}
	if challengeAnchor != challenge.Hash() {
		t.Fatalf(
			"challenge=%s want=%s",
			challengeAnchor,
			challenge.Hash(),
		)
	}
	if difficulty == nil || difficulty.Cmp(big.NewInt(17)) != 0 {
		t.Fatalf("difficulty=%v want=17", difficulty)
	}
	if err := eligibility(participant); err != nil {
		t.Fatalf("existing participant rejected: %v", err)
	}
	if err := eligibility(outsider); err != nil {
		t.Fatalf("permissionless new participant rejected: %v", err)
	}
}

func TestWorkV1EngineLabRelayContextReplaysHeaderV3AfterRestart(
	t *testing.T,
) {
	participants := testParticipants(t, 4)
	config := canonicalRegistryEngineConfig(participants, 1)
	config.EpochLength = WorkProtocolEpochLengthV1
	db := rawdb.NewMemoryDatabase()
	builder := New(config, db)
	genesis := &types.Header{
		Number:   big.NewInt(0),
		Time:     100,
		GasLimit: 30_000_000,
	}
	chain := canonicalRegistryTestChain(config, genesis)
	parent := genesis

	for number := uint64(1); number <= WorkProtocolEpochLengthV1; number++ {
		header := prepareCanonicalTestHeader(
			t,
			builder,
			chain,
			parent,
		)
		if _, err := DecodeLQCHeaderExtraV3(
			header.Extra,
			MaxWorkTicketsPerBlockV1,
		); err != nil {
			t.Fatalf("block %d is not Header V3: %v", number, err)
		}
		if err := builder.VerifyHeader(chain, header); err != nil {
			t.Fatalf("verify block %d: %v", number, err)
		}
		chain.headers[header.Hash()] = header
		chain.current = header
		parent = header
	}

	restarted := New(config, db)
	epoch,
		datasetAnchor,
		challengeAnchor,
		difficulty,
		eligibility,
		err := restarted.WorkV1EngineLabRelayContext(
		chain,
		WorkProtocolEpochLengthV1,
		parent.Hash(),
		WorkProtocolEpochLengthV1+1,
	)
	if err != nil {
		t.Fatalf("relay context after restart: %v", err)
	}
	if epoch != 1 {
		t.Fatalf("epoch=%d want=1", epoch)
	}
	if datasetAnchor != genesis.Hash() {
		t.Fatalf("dataset=%s want=%s", datasetAnchor, genesis.Hash())
	}
	challenge := chain.GetHeaderByNumber(1)
	if challenge == nil || challengeAnchor != challenge.Hash() {
		t.Fatalf("challenge=%s want block-one hash", challengeAnchor)
	}
	if difficulty == nil || difficulty.Cmp(
		new(big.Int).SetUint64(config.ProofDifficulty),
	) != 0 {
		t.Fatalf(
			"difficulty=%v want=%d",
			difficulty,
			config.ProofDifficulty,
		)
	}
	if err := eligibility(participants[0]); err != nil {
		t.Fatalf("historical participant rejected: %v", err)
	}
	if runtime, ok, err := restarted.workV1EngineLabCached(parent.Hash()); err != nil {
		t.Fatalf("cached runtime after restart: %v", err)
	} else if !ok || runtime == nil || runtime.Work == nil ||
		runtime.Work.Number != WorkProtocolEpochLengthV1 ||
		runtime.Work.Hash != parent.Hash() {
		t.Fatal("canonical Work runtime checkpoint was not restored after restart")
	}
	if snapshot, ok := restarted.cachedRegistrySnapshot(
		WorkProtocolEpochLengthV1,
		parent.Hash(),
	); !ok || snapshot == nil {
		t.Fatal("canonical registry checkpoint was not available after restart")
	}
}

func TestWorkV1RegistrySnapshotReplaysV3PastCheckpointAfterRestart(t *testing.T) {
	participants := testParticipants(t, 4)
	config := canonicalRegistryEngineConfig(participants, 1)
	config.EpochLength = WorkProtocolEpochLengthV1
	db := rawdb.NewMemoryDatabase()
	builder := New(config, db)
	genesis := &types.Header{
		Number:   big.NewInt(0),
		Time:     100,
		GasLimit: 30_000_000,
	}
	chain := canonicalRegistryTestChain(config, genesis)
	parent := genesis
	end := WorkProtocolEpochLengthV1 + 2

	for number := uint64(1); number <= end; number++ {
		header := prepareCanonicalTestHeader(t, builder, chain, parent)
		if _, err := DecodeLQCHeaderExtraV3(
			header.Extra,
			MaxWorkTicketsPerBlockV1,
		); err != nil {
			t.Fatalf("block %d is not Header V3: %v", number, err)
		}
		if err := builder.VerifyHeader(chain, header); err != nil {
			t.Fatalf("verify block %d: %v", number, err)
		}
		chain.headers[header.Hash()] = header
		chain.current = header
		parent = header
	}

	restarted := New(config, db)
	snapshot, err := restarted.registrySnapshotAt(
		chain,
		parent.Number.Uint64(),
		parent.Hash(),
	)
	if err != nil {
		t.Fatalf("V3 registry replay past checkpoint after restart: %v", err)
	}
	envelope, err := DecodeLQCHeaderExtraV3(
		parent.Extra,
		MaxWorkTicketsPerBlockV1,
	)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Number != parent.Number.Uint64() ||
		snapshot.Hash != parent.Hash() ||
		snapshot.RegistryRoot != envelope.RegistryRoot {
		t.Fatalf("rebuilt snapshot mismatch: %+v", snapshot)
	}
}

func TestWorkV1ActiveEngineLabPrepareVerifyUsesHeaderV3(t *testing.T) {
	participants := testParticipants(t, 4)
	config := canonicalRegistryEngineConfig(participants, 1)
	config.EpochLength = WorkProtocolEpochLengthV1

	engine := New(config, rawdb.NewMemoryDatabase())
	genesis := &types.Header{
		Number:   big.NewInt(0),
		Time:     100,
		GasLimit: 30_000_000,
	}
	chain := canonicalRegistryTestChain(config, genesis)

	header1 := prepareCanonicalTestHeader(
		t,
		engine,
		chain,
		genesis,
	)

	envelope, err := DecodeLQCHeaderExtraV3(
		header1.Extra,
		MaxWorkTicketsPerBlockV1,
	)
	if err != nil {
		t.Fatalf("Prepare did not emit Header V3: %v", err)
	}
	if envelope.BlockNumber != 1 {
		t.Fatalf("block=%d want=1", envelope.BlockNumber)
	}
	if envelope.RegistryRoot == (common.Hash{}) {
		t.Fatal("zero registry root")
	}
	if envelope.WorkStateRoot == (common.Hash{}) {
		t.Fatal("zero work state root")
	}
	if len(envelope.WorkTickets) != 0 {
		t.Fatalf("first epoch tickets=%d want=0", len(envelope.WorkTickets))
	}

	if err := engine.VerifyHeader(chain, header1); err != nil {
		t.Fatalf("Header V3 rejected by active engine lab path: %v", err)
	}

	runtime, ok, err := engine.workV1EngineLabCached(
		header1.Hash(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || runtime == nil {
		t.Fatal("verified Header V3 runtime not cached")
	}
	if runtime.StateRoot != envelope.WorkStateRoot {
		t.Fatalf(
			"runtime root=%s header root=%s",
			runtime.StateRoot,
			envelope.WorkStateRoot,
		)
	}
}

func TestWorkV1EngineLabVerifyHeadersUsesBatchParentRuntime(
	t *testing.T,
) {
	participants := testParticipants(t, 4)
	config := canonicalRegistryEngineConfig(participants, 1)
	config.EpochLength = WorkProtocolEpochLengthV1
	genesis := &types.Header{
		Number:   big.NewInt(0),
		Time:     100,
		GasLimit: 30_000_000,
	}
	buildChain := canonicalRegistryTestChain(config, genesis)
	builder := New(config, rawdb.NewMemoryDatabase())
	headers := make([]*types.Header, 0, WorkProtocolEpochLengthV1+1)
	parent := genesis
	for number := uint64(1); number <= WorkProtocolEpochLengthV1+1; number++ {
		header := prepareCanonicalTestHeader(t, builder, buildChain, parent)
		if err := builder.VerifyHeader(buildChain, header); err != nil {
			t.Fatalf("build header %d: %v", number, err)
		}
		buildChain.headers[header.Hash()] = header
		buildChain.current = header
		headers = append(headers, header)
		parent = header
	}

	baseChain := canonicalRegistryTestChain(config, genesis)
	verifier := New(config, rawdb.NewMemoryDatabase())
	abort, results := verifier.VerifyHeaders(baseChain, headers)
	defer close(abort)
	for index := 1; index <= len(headers); index++ {
		if err := <-results; err != nil {
			t.Fatalf("Work V1 batch header %d rejected: %v", index, err)
		}
	}
}
