//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/ethash"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
)

type rabbitVRFReorgTestEngine struct {
	consensus.Engine
}

func (*rabbitVRFReorgTestEngine) RabbitVRFValidatedFinalizations(common.Hash) ([]consensus.RabbitVRFValidatedFinalization, bool, error) {
	return nil, true, nil
}

func TestRabbitVRFRequestScanCursorV1ReorgRewinds(t *testing.T) {
	activation := uint64(100)
	oldHash := common.HexToHash("0x01")
	newHash := common.HexToHash("0x02")

	cursor := rabbitVRFRequestScanCursorV1{
		Next:     151,
		LastHash: oldHash,
	}

	if !cursor.reconcilePreviousV1(activation, newHash) {
		t.Fatal("canonical reorg was not detected")
	}
	if cursor.Next != activation {
		t.Fatalf("reorg cursor next=%d want=%d", cursor.Next, activation)
	}
	if cursor.LastHash != (common.Hash{}) {
		t.Fatal("reorg cursor retained stale canonical hash")
	}
}

func TestRabbitVRFRequestScanCursorV1CanonicalContinuation(t *testing.T) {
	activation := uint64(100)
	canonicalHash := common.HexToHash("0x1234")
	cursor := rabbitVRFRequestScanCursorV1{
		Next:     151,
		LastHash: canonicalHash,
	}

	if cursor.reconcilePreviousV1(activation, canonicalHash) {
		t.Fatal("canonical cursor was incorrectly rewound")
	}
	if cursor.Next != 151 || cursor.LastHash != canonicalHash {
		t.Fatal("canonical cursor changed unexpectedly")
	}
}

func TestRabbitVRFRequestScanCursorV1MissingPreviousRewinds(t *testing.T) {
	activation := uint64(100)
	cursor := rabbitVRFRequestScanCursorV1{
		Next:     151,
		LastHash: common.HexToHash("0x1234"),
	}

	if !cursor.reconcilePreviousV1(activation, common.Hash{}) {
		t.Fatal("missing previous canonical block did not rewind cursor")
	}
	if cursor.Next != activation || cursor.LastHash != (common.Hash{}) {
		t.Fatal("missing previous canonical block did not reset cursor")
	}
}

func TestRabbitVRFRequestIDsFromReceiptsV1MultipleAndDuplicate(t *testing.T) {
	requestA := common.HexToHash("0xaa")
	requestB := common.HexToHash("0xbb")

	requested := func(address common.Address, requestID common.Hash) *types.Log {
		return &types.Log{
			Address: address,
			Topics: []common.Hash{
				rabbitVRFRandomnessRequestedTopicV1,
				requestID,
				common.HexToHash("0x11"),
				common.HexToHash("0x22"),
			},
		}
	}

	receipts := types.Receipts{
		&types.Receipt{Logs: []*types.Log{
			requested(params.RabbitVRFCoordinatorV1Address, requestA),
			requested(params.RabbitVRFCoordinatorV1Address, requestB),
			requested(params.RabbitVRFCoordinatorV1Address, requestA),
		}},
		&types.Receipt{Logs: []*types.Log{
			requested(common.HexToAddress("0x1234"), common.HexToHash("0xcc")),
			requested(params.RabbitVRFCoordinatorV1Address, common.Hash{}),
		}},
	}

	got := rabbitVRFRequestIDsFromReceiptsV1(receipts)
	if len(got) != 2 {
		t.Fatalf("request count=%d want=2", len(got))
	}
	if got[0] != requestA || got[1] != requestB {
		t.Fatalf("request order=%v want=[%s %s]", got, requestA, requestB)
	}
}

func TestRabbitVRFRequestWorkerV1CanonicalChainReorgEndToEnd(t *testing.T) {
	const activation = uint64(4)

	config := *params.RabbitChainConfig
	config.ChainID = big.NewInt(9280)
	config.LQC = &params.LQCConfig{
		CommitteeMin:      1,
		CommitteeMax:      1,
		FallbackSlots:     1,
		TargetBlockTimeMs: 1000,
		EpochLength:       1,
		VRFProtocolBlock:  activation,
	}
	genesis := &core.Genesis{Config: &config}
	engine := &rabbitVRFReorgTestEngine{Engine: ethash.NewFaker()}

	db, chainA, _ := core.GenerateChainWithGenesis(
		genesis,
		engine,
		6,
		func(_ int, block *core.BlockGen) {
			block.SetExtra([]byte("rabbit-vrf-reorg-A"))
		},
	)

	blockchain, err := core.NewBlockChain(db, genesis, engine, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer blockchain.Stop()

	if _, err := blockchain.InsertChain(chainA); err != nil {
		t.Fatalf("insert branch A: %v", err)
	}
	if got := blockchain.CurrentBlock().Hash(); got != chainA[len(chainA)-1].Hash() {
		t.Fatalf("branch A is not canonical: got=%s want=%s", got, chainA[len(chainA)-1].Hash())
	}

	backend := &Ethereum{
		blockchain:        blockchain,
		vrfDKGInstanceDir: t.TempDir(),
	}
	runtime := &rabbitVRFDKGRuntime{
		backend:     backend,
		secretReady: true,
		current: rabbitVRFDKGLocalContextV1{
			SessionID: common.HexToHash("0xabc1"),
			Members: []lqc.RabbitVRFCommitteeMemberV1{
				{
					ShareID:     1,
					Participant: common.HexToAddress("0x1001"),
				},
			},
		},
	}
	backend.vrfDKGTransport = &rabbitVRFDKGTransport{runtime: runtime}

	cursor := rabbitVRFRequestScanCursorV1{Next: activation}
	if err := runtime.processCanonicalPendingRequestsV1(&cursor); err != nil {
		t.Fatalf("scan branch A: %v", err)
	}
	if cursor.Next != 7 || cursor.LastHash != chainA[5].Hash() {
		t.Fatalf(
			"branch A cursor=(next=%d hash=%s) want=(7 %s)",
			cursor.Next,
			cursor.LastHash,
			chainA[5].Hash(),
		)
	}

	genesisBlock := blockchain.GetBlockByNumber(0)
	if genesisBlock == nil {
		t.Fatal("canonical genesis unavailable")
	}
	chainB, _ := core.GenerateChain(
		&config,
		genesisBlock,
		engine,
		db,
		7,
		func(_ int, block *core.BlockGen) {
			block.SetExtra([]byte("rabbit-vrf-reorg-B"))
		},
	)
	if chainB[5].Hash() == chainA[5].Hash() {
		t.Fatal("test branches unexpectedly share the same block 6 hash")
	}

	if _, err := blockchain.InsertChain(chainB); err != nil {
		t.Fatalf("insert branch B: %v", err)
	}
	if got := blockchain.CurrentBlock().Hash(); got != chainB[len(chainB)-1].Hash() {
		t.Fatalf("branch B did not become canonical: got=%s want=%s", got, chainB[len(chainB)-1].Hash())
	}

	// The cursor still points after A6 here. The worker must notice that
	// canonical block 6 is now B6, rewind to activation and rescan B4..B7.
	if err := runtime.processCanonicalPendingRequestsV1(&cursor); err != nil {
		t.Fatalf("scan canonical branch B after reorg: %v", err)
	}
	if cursor.Next != 8 {
		t.Fatalf("post-reorg cursor next=%d want=8", cursor.Next)
	}
	if cursor.LastHash != chainB[6].Hash() {
		t.Fatalf("post-reorg cursor hash=%s want=%s", cursor.LastHash, chainB[6].Hash())
	}
	if cursor.LastHash == chainA[5].Hash() {
		t.Fatal("post-reorg cursor retained stale branch A hash")
	}
}
