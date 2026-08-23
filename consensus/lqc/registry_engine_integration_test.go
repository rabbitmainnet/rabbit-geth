package lqc

import (
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
	"github.com/holiman/uint256"
)

func canonicalRegistryEngineConfig(participants []common.Address, activation uint64) *params.LQCConfig {
	return &params.LQCConfig{
		RegistryMode:          "bootstrap",
		RegistryProtocolBlock: activation,
		BootstrapParticipants: participants,
		ProofDifficulty:       1,
		ActivationDelay:       0,
		HeartbeatWindow:       64,
		HeartbeatGrace:        16,
		CommitteeSize:         2,
		FallbackCount:         2,
		TargetBlockTimeMs:     10_000,
		FallbackWindowMs:      3_000,
		EpochLength:           1,
	}
}

func canonicalRegistryTestChain(config *params.LQCConfig, genesis *types.Header) *testHeaderChain {
	return &testHeaderChain{
		config:  &params.ChainConfig{ChainID: big.NewInt(928), LQC: config},
		headers: map[common.Hash]*types.Header{genesis.Hash(): genesis},
		current: genesis,
	}
}

func prepareCanonicalTestHeader(t *testing.T, engine *LQC, chain *testHeaderChain, parent *types.Header) *types.Header {
	t.Helper()
	header := &types.Header{
		ParentHash: parent.Hash(),
		Number:     new(big.Int).Add(parent.Number, big.NewInt(1)),
		GasLimit:   parent.GasLimit,
	}
	selection := engine.selectionForHeader(chain, header)
	if selection.Producer == nil {
		t.Fatal("canonical selection has no producer")
	}
	header.Coinbase = selection.Producer.Address
	if err := engine.Prepare(chain, header); err != nil {
		t.Fatalf("prepare canonical header: %v", err)
	}
	signTestHeader(t, chain.Config().ChainID, header)
	return header
}

func decodePreparedRegistryExtra(t *testing.T, extra []byte) RegistryHeaderEnvelope {
	t.Helper()

	envelope, err := DecodeLQCHeaderExtraV3(
		extra,
		MaxWorkTicketsPerBlockV1,
	)
	if err == nil {
		return RegistryHeaderEnvelope{
			Version:      RegistryHeaderEnvelopeVersion,
			BlockNumber:  envelope.BlockNumber,
			RegistryRoot: envelope.RegistryRoot,
			Operations:   envelope.RegistryOperations,
		}
	}
	registryEnvelope, registryErr := DecodeRegistryHeaderExtra(extra)
	if registryErr != nil {
		t.Fatalf("prepared header is neither Registry V2 nor LQC V3: v3=%v v2=%v", err, registryErr)
	}
	return registryEnvelope
}

func TestRegistryProtocolActivationKeepsLegacyBeforeFork(t *testing.T) {
	ResetRuntimeRegistry()
	t.Cleanup(ResetRuntimeRegistry)
	participants := testParticipants(t, 6)
	config := canonicalRegistryEngineConfig(participants, 2)
	engine := New(config, rawdb.NewMemoryDatabase())
	genesis := &types.Header{Number: big.NewInt(0), Time: 100, GasLimit: 30_000_000}
	chain := canonicalRegistryTestChain(config, genesis)

	header1 := prepareCanonicalTestHeader(t, engine, chain, genesis)
	legacyExtra, _, err := splitProducerSeal(header1.Extra)
	if err != nil {
		t.Fatal(err)
	}
	if string(legacyExtra) != "LQC:1:1" {
		t.Fatalf("pre-activation extra = %q, want legacy format", header1.Extra)
	}
	if err := engine.VerifyHeader(chain, header1); err != nil {
		t.Fatalf("legacy header rejected before activation: %v", err)
	}
	chain.headers[header1.Hash()] = header1
	chain.current = header1
	header2 := prepareCanonicalTestHeader(t, engine, chain, header1)
	if !IsRegistryHeaderExtra(header2.Extra) {
		t.Fatal("activation block did not use canonical registry envelope")
	}
	if err := engine.VerifyHeader(chain, header2); err != nil {
		t.Fatalf("activation header rejected: %v", err)
	}
	legacyAtActivation := types.CopyHeader(header2)
	legacyAtActivation.Extra = engine.makeExtra(2)
	if err := engine.VerifyHeader(chain, legacyAtActivation); err == nil {
		t.Fatal("legacy extra accepted at canonical activation block")
	}
}

func TestRegistryProtocolActivationAtBlockOneDoesNotDelayBootstrap(t *testing.T) {
	participants := testParticipants(t, 6)
	config := canonicalRegistryEngineConfig(participants, 1)
	config.ActivationDelay = 2
	engine := New(config, rawdb.NewMemoryDatabase())
	genesis := &types.Header{Number: big.NewInt(0), Time: 100, GasLimit: 30_000_000}
	chain := canonicalRegistryTestChain(config, genesis)

	header1 := prepareCanonicalTestHeader(t, engine, chain, genesis)
	if !IsRegistryHeaderExtra(header1.Extra) {
		t.Fatal("activation block did not use canonical registry envelope")
	}
	if err := engine.VerifyHeader(chain, header1); err != nil {
		t.Fatalf("bootstrap producer rejected at block-one activation: %v", err)
	}
}

func TestPermissionlessBlockOneAndRecoveryReopenRegistry(t *testing.T) {
	producers := testParticipants(t, 2)
	config := canonicalRegistryEngineConfig(nil, 1)
	config.RegistryMode = "native"
	config.RecoveryTimeoutMs = 60_000
	db := rawdb.NewMemoryDatabase()
	engine := New(config, db)
	genesis := &types.Header{Number: big.NewInt(0), Time: 100, GasLimit: 30_000_000}
	chain := canonicalRegistryTestChain(config, genesis)

	header1 := &types.Header{
		ParentHash: genesis.Hash(),
		Number:     big.NewInt(1),
		Coinbase:   producers[0],
		Time:       110,
		GasLimit:   genesis.GasLimit,
	}
	if err := engine.Prepare(chain, header1); err != nil {
		t.Fatalf("prepare permissionless block 1: %v", err)
	}
	signTestHeader(t, chain.Config().ChainID, header1)
	if err := engine.VerifyHeader(chain, header1); err != nil {
		t.Fatalf("verify permissionless block 1: %v", err)
	}
	chain.headers[header1.Hash()] = header1
	chain.current = header1
	nextSelection := engine.selectionForHeaderMaybeWorkV1Lab(chain, &types.Header{
		ParentHash: header1.Hash(),
		Number:     big.NewInt(2),
	})
	if nextSelection.Producer == nil || nextSelection.Producer.Address != producers[0] {
		t.Fatalf("block-1 activator was delayed from the next queue: %+v", nextSelection)
	}

	tooEarly := &types.Header{
		ParentHash: header1.Hash(),
		Number:     big.NewInt(2),
		Coinbase:   producers[1],
		Time:       header1.Time + 59,
		GasLimit:   header1.GasLimit,
	}
	if err := engine.Prepare(chain, tooEarly); err == nil {
		t.Fatal("unselected producer reopened registry before recovery timeout")
	}

	recovery := &types.Header{
		ParentHash: header1.Hash(),
		Number:     big.NewInt(2),
		Coinbase:   producers[1],
		Time:       header1.Time + 60,
		GasLimit:   header1.GasLimit,
	}
	if err := engine.Prepare(chain, recovery); err != nil {
		t.Fatalf("prepare permissionless recovery: %v", err)
	}
	signTestHeader(t, chain.Config().ChainID, recovery)
	if err := engine.VerifyHeader(chain, recovery); err != nil {
		t.Fatalf("verify permissionless recovery: %v", err)
	}
	snapshot, ok := engine.cachedRegistrySnapshot(2, recovery.Hash())
	if !ok {
		t.Fatal("recovery snapshot was not cached")
	}
	registry, err := snapshot.Registry()
	if err != nil {
		t.Fatal(err)
	}
	participants := registry.Participants()
	if len(participants) != 1 || participants[0].Address != producers[1] {
		t.Fatalf("recovery registry = %+v, want only %s", participants, producers[1])
	}
	chain.headers[recovery.Hash()] = recovery
	chain.current = recovery
	restarted := New(config, db)
	nextSelection = restarted.selectionForHeaderMaybeWorkV1Lab(chain, &types.Header{
		ParentHash: recovery.Hash(),
		Number:     big.NewInt(3),
	})
	if nextSelection.Producer == nil || nextSelection.Producer.Address != producers[1] {
		t.Fatalf("recovery activator was delayed from the next queue: %+v", nextSelection)
	}
}

func TestCanonicalPrepareVerifyAndRestartFromCheckpoint(t *testing.T) {
	participants := testParticipants(t, 8)
	config := canonicalRegistryEngineConfig(participants, 1)
	db := rawdb.NewMemoryDatabase()
	engine := New(config, db)
	genesis := &types.Header{Number: big.NewInt(0), Time: 100, GasLimit: 30_000_000}
	chain := canonicalRegistryTestChain(config, genesis)

	header1 := prepareCanonicalTestHeader(t, engine, chain, genesis)
	envelope := decodePreparedRegistryExtra(t, header1.Extra)
	if envelope.BlockNumber != 1 || len(envelope.Operations) != 0 {
		t.Fatalf("unexpected prepared envelope: %+v", envelope)
	}
	if err := engine.VerifyHeader(chain, header1); err != nil {
		t.Fatal(err)
	}
	chain.headers[header1.Hash()] = header1
	chain.current = header1

	restarted := New(config, db)
	header2 := prepareCanonicalTestHeader(t, restarted, chain, header1)
	if err := restarted.VerifyHeader(chain, header2); err != nil {
		t.Fatalf("restarted engine rejected canonical child: %v", err)
	}
	if snapshot, ok := restarted.cachedRegistrySnapshot(1, header1.Hash()); !ok || snapshot.RegistryRoot != envelope.RegistryRoot {
		t.Fatal("restarted engine did not load the hash-indexed checkpoint")
	}
}

func TestCanonicalVerifyHeadersUsesSnapshotsInsideBatch(t *testing.T) {
	participants := testParticipants(t, 8)
	config := canonicalRegistryEngineConfig(participants, 1)
	builder := New(config, rawdb.NewMemoryDatabase())
	genesis := &types.Header{Number: big.NewInt(0), Time: 100, GasLimit: 30_000_000}
	chain := canonicalRegistryTestChain(config, genesis)

	header1 := prepareCanonicalTestHeader(t, builder, chain, genesis)
	if err := builder.VerifyHeader(chain, header1); err != nil {
		t.Fatal(err)
	}
	chain.headers[header1.Hash()] = header1
	chain.current = header1
	header2 := prepareCanonicalTestHeader(t, builder, chain, header1)

	baseChain := canonicalRegistryTestChain(config, genesis)
	verifier := New(config, rawdb.NewMemoryDatabase())
	abort, results := verifier.VerifyHeaders(baseChain, []*types.Header{header1, header2})
	defer close(abort)
	for index := 1; index <= 2; index++ {
		if err := <-results; err != nil {
			t.Fatalf("canonical batch header %d rejected: %v", index, err)
		}
	}

	bad := types.CopyHeader(header2)
	decoded := decodePreparedRegistryExtra(t, bad.Extra)
	badExtra, err := EncodeRegistryHeaderExtra(decoded.BlockNumber, common.HexToHash("0x1234"), decoded.Operations)
	if err != nil {
		t.Fatal(err)
	}
	bad.Extra = badExtra
	signTestHeader(t, baseChain.Config().ChainID, bad)
	verifier = New(config, rawdb.NewMemoryDatabase())
	abort, results = verifier.VerifyHeaders(baseChain, []*types.Header{header1, bad})
	defer close(abort)
	if err := <-results; err != nil {
		t.Fatalf("first header rejected: %v", err)
	}
	if err := <-results; !errors.Is(err, ErrRegistryRootMismatch) {
		t.Fatalf("root error = %v, want %v", err, ErrRegistryRootMismatch)
	}
}

func TestCanonicalEngineAcceptsPermissionlessRegistrationWithoutBond(t *testing.T) {
	chainID := big.NewInt(928)
	participants := testParticipants(t, 3)
	newcomerKey := testRegistryKey(t, "2525252525252525252525252525252525252525252525252525252525252525")
	config := canonicalRegistryEngineConfig(participants, 1)
	engine := New(config, rawdb.NewMemoryDatabase())
	genesis := &types.Header{Number: big.NewInt(0), Time: 100, GasLimit: 30_000_000}
	chain := canonicalRegistryTestChain(config, genesis)
	skeleton := &types.Header{ParentHash: genesis.Hash(), Number: big.NewInt(1)}
	selection := engine.selectionForHeader(chain, skeleton)
	if selection.Producer == nil {
		t.Fatal("no bootstrap producer selected")
	}
	parentSnapshot, err := engine.registryParentSnapshot(chain, skeleton)
	if err != nil {
		t.Fatal(err)
	}
	register := registerOperation(t, chainID, newcomerKey, 1, 20, 1)
	header1 := buildRegistrySnapshotHeader(t, parentSnapshot, chainID, engine.registryRules(), selection.Producer.Address, []RegistryOperation{register})
	header1.Time = engine.minAllowedTime(genesis.Time, 0)
	header1.Difficulty = big.NewInt(0)
	header1.GasLimit = genesis.GasLimit
	signTestHeader(t, chainID, header1)
	if err := engine.VerifyHeader(chain, header1); err != nil {
		t.Fatalf("permissionless registration header rejected: %v", err)
	}
	chain.headers[header1.Hash()] = header1
	chain.current = header1

	header2 := &types.Header{ParentHash: header1.Hash(), Number: big.NewInt(2)}
	selection = engine.selectionForHeader(chain, header2)
	if len(selection.Ordered) != len(participants)+1 {
		t.Fatalf("canonical queue size = %d, want %d", len(selection.Ordered), len(participants)+1)
	}
	found := false
	for _, participant := range selection.Ordered {
		if participant.Address == register.Address {
			found = true
			if participant.Bond == nil || participant.Bond.Sign() != 0 {
				t.Fatalf("newcomer bond = %v, want zero", participant.Bond)
			}
		}
	}
	if !found {
		t.Fatal("permissionless newcomer missing from canonical queue")
	}
}

func TestCanonicalRewardsIgnoreProcessLocalRegistry(t *testing.T) {
	ResetRuntimeRegistry()
	t.Cleanup(ResetRuntimeRegistry)
	participants := testParticipants(t, 8)
	config := canonicalRegistryEngineConfig(participants, 1)
	config.CommitteeRatioBps = 3_000
	engine := New(config, rawdb.NewMemoryDatabase())
	genesis := &types.Header{Number: big.NewInt(0), Time: 100, GasLimit: 30_000_000}
	chain := canonicalRegistryTestChain(config, genesis)

	header1 := prepareCanonicalTestHeader(t, engine, chain, genesis)
	if err := engine.VerifyHeader(chain, header1); err != nil {
		t.Fatal(err)
	}
	chain.headers[header1.Hash()] = header1
	chain.current = header1
	header2 := prepareCanonicalTestHeader(t, engine, chain, header1)
	selection := engine.selectionForHeader(chain, header2)
	if selection.Producer == nil || len(selection.Committee) != 2 {
		t.Fatalf("unexpected canonical reward selection: %+v", selection)
	}

	// A process-local entry must have no influence after the fork.
	localOnly := common.HexToAddress("0x9999999999999999999999999999999999999999")
	RegisterParticipant(nil, localOnly, header2.Number.Uint64())
	UpdateParticipantActivity(nil, localOnly, header2.Number.Uint64())

	statedb, err := state.New(types.EmptyRootHash, state.NewDatabaseForTesting())
	if err != nil {
		t.Fatal(err)
	}
	engine.Finalize(chain, header2, statedb, new(types.Body), 0, nil)
	producerWant := uint256.NewInt(840_000_000_000_000_000)
	if got := statedb.GetBalance(header2.Coinbase); got.Cmp(producerWant) != 0 {
		t.Fatalf("canonical producer reward = %s, want %s", got, producerWant)
	}
	committeeWant := uint256.NewInt(180_000_000_000_000_000)
	for _, member := range selection.Committee {
		if got := statedb.GetBalance(member.Address); got.Cmp(committeeWant) != 0 {
			t.Fatalf("canonical committee reward for %s = %s, want %s", member.Address, got, committeeWant)
		}
	}
	if got := statedb.GetBalance(localOnly); !got.IsZero() {
		t.Fatalf("process-local participant received canonical reward: %s", got)
	}
}

func TestCanonicalPrepareIncludesValidatedPoolOperation(t *testing.T) {
	chainID := big.NewInt(928)
	participants := testParticipants(t, 8)
	config := canonicalRegistryEngineConfig(participants, 1)
	engine := New(config, rawdb.NewMemoryDatabase())
	genesis := &types.Header{Number: big.NewInt(0), Time: 100, GasLimit: 30_000_000}
	chain := canonicalRegistryTestChain(config, genesis)
	newcomerKey := testRegistryKey(t, "2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b")
	register := registerOperation(t, chainID, newcomerKey, 1, 20, 1)
	hash, err := engine.SubmitRegistryOperation(chain, register)
	if err != nil {
		t.Fatalf("submit pool operation: %v", err)
	}
	if !engine.registryPool.Has(hash) {
		t.Fatal("submitted operation missing from pool")
	}

	header1 := prepareCanonicalTestHeader(t, engine, chain, genesis)
	envelope := decodePreparedRegistryExtra(t, header1.Extra)
	if len(envelope.Operations) != 1 || envelope.Operations[0].Address != register.Address {
		t.Fatalf("prepared operations = %+v, want newcomer %s", envelope.Operations, register.Address)
	}
	if err := engine.VerifyHeader(chain, header1); err != nil {
		t.Fatalf("header containing pool operation rejected: %v", err)
	}
	chain.headers[header1.Hash()] = header1
	chain.current = header1
	header2 := &types.Header{ParentHash: header1.Hash(), Number: big.NewInt(2)}
	selection := engine.selectionForHeader(chain, header2)
	found := false
	for _, participant := range selection.Ordered {
		if participant.Address == register.Address {
			found = true
		}
	}
	if !found {
		t.Fatal("pool newcomer was not admitted to the next canonical queue")
	}
}
