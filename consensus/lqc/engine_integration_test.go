package lqc

import (
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
)

var _ consensus.LocalParticipantResolver = (*LQC)(nil)

type testHeaderChain struct {
	config  *params.ChainConfig
	headers map[common.Hash]*types.Header
	current *types.Header
}

func (c *testHeaderChain) Config() *params.ChainConfig  { return c.config }
func (c *testHeaderChain) CurrentHeader() *types.Header { return c.current }
func (c *testHeaderChain) GetHeader(hash common.Hash, number uint64) *types.Header {
	header := c.headers[hash]
	if header != nil && header.Number != nil && header.Number.Uint64() == number {
		return header
	}
	return nil
}
func (c *testHeaderChain) GetHeaderByNumber(number uint64) *types.Header {
	for _, header := range c.headers {
		if header.Number != nil && header.Number.Uint64() == number {
			return header
		}
	}
	return nil
}
func (c *testHeaderChain) GetHeaderByHash(hash common.Hash) *types.Header { return c.headers[hash] }

func TestResolveLocalParticipantUsesNextBlockQueue(t *testing.T) {
	ResetRuntimeRegistry()
	participants := testParticipants(t, 8)
	config := &params.LQCConfig{
		RegistryMode:          "bootstrap",
		BootstrapParticipants: participants,
		CommitteeSize:         3,
		FallbackCount:         2,
		TargetBlockTimeMs:     10_000,
		FallbackWindowMs:      3_000,
	}
	engine := New(config, nil)
	current := &types.Header{Number: big.NewInt(12), Time: 120, Extra: []byte("current")}
	next := &types.Header{ParentHash: current.Hash(), Number: big.NewInt(13)}
	selection := engine.selectionForHeader(nil, next)
	if len(selection.Ordered) != len(participants) {
		t.Fatalf("ordered participants = %d, want %d", len(selection.Ordered), len(participants))
	}
	wantPos := 4
	want := selection.Ordered[wantPos].Address
	got := engine.ResolveLocalParticipant(nil, current, []common.Address{want})
	if !got.Allowed || got.Address != want || got.QueuePos != wantPos {
		t.Fatalf("resolved = %+v, want address %s at queue position %d", got, want, wantPos)
	}
}

func TestPreparePreservesRealTimestampAfterSlotMinimum(t *testing.T) {
	ResetRuntimeRegistry()
	participants := testParticipants(t, 8)
	lqcConfig := &params.LQCConfig{
		RegistryMode:          "bootstrap",
		BootstrapParticipants: participants,
		CommitteeSize:         3,
		FallbackCount:         2,
		TargetBlockTimeMs:     10_000,
		FallbackWindowMs:      3_000,
	}
	engine := New(lqcConfig, nil)
	parent := &types.Header{Number: big.NewInt(0), Time: 0, GasLimit: 30_000_000}
	chain := &testHeaderChain{
		config:  &params.ChainConfig{LQC: lqcConfig},
		headers: map[common.Hash]*types.Header{parent.Hash(): parent},
		current: parent,
	}
	header := &types.Header{
		ParentHash: parent.Hash(),
		Number:     big.NewInt(1),
		GasLimit:   parent.GasLimit,
		Time:       1_800_000_000,
	}
	selection := engine.selectionForHeader(nil, header)
	if selection.Producer == nil {
		t.Fatal("no producer selected")
	}
	header.Coinbase = selection.Producer.Address
	if err := engine.Prepare(chain, header); err != nil {
		t.Fatalf("prepare header: %v", err)
	}
	if header.Time != 1_800_000_000 {
		t.Fatalf("prepared timestamp = %d, want real miner timestamp 1800000000", header.Time)
	}
}

func TestVerifyHeadersAcceptsParentsInsideBatch(t *testing.T) {
	ResetRuntimeRegistry()
	participants := testParticipants(t, 8)
	lqcConfig := &params.LQCConfig{
		RegistryMode:          "bootstrap",
		BootstrapParticipants: participants,
		CommitteeSize:         3,
		FallbackCount:         2,
		TargetBlockTimeMs:     10_000,
		FallbackWindowMs:      3_000,
	}
	chainConfig := &params.ChainConfig{ChainID: big.NewInt(928), LQC: lqcConfig}
	engine := New(lqcConfig, nil)
	genesis := &types.Header{Number: big.NewInt(0), Time: 100, GasLimit: 30_000_000}
	baseChain := &testHeaderChain{
		config:  chainConfig,
		headers: map[common.Hash]*types.Header{genesis.Hash(): genesis},
		current: genesis,
	}

	header1 := preparedHeader(t, engine, baseChain, genesis)
	preparationChain := &testHeaderChain{
		config: chainConfig,
		headers: map[common.Hash]*types.Header{
			genesis.Hash(): genesis,
			header1.Hash(): header1,
		},
		current: header1,
	}
	header2 := preparedHeader(t, engine, preparationChain, header1)

	abort, results := engine.VerifyHeaders(baseChain, []*types.Header{header1, header2})
	defer close(abort)
	for index := 1; index <= 2; index++ {
		if err := <-results; err != nil {
			t.Fatalf("header %d rejected: %v", index, err)
		}
	}

	invalid := types.CopyHeader(header2)
	invalid.Extra = []byte("invalid")
	abort, results = engine.VerifyHeaders(baseChain, []*types.Header{header1, invalid})
	defer close(abort)
	if err := <-results; err != nil {
		t.Fatalf("first header rejected: %v", err)
	}
	if err := <-results; err == nil {
		t.Fatal("invalid second header accepted")
	}
}

func preparedHeader(t *testing.T, engine *LQC, chain consensus.ChainHeaderReader, parent *types.Header) *types.Header {
	t.Helper()
	header := &types.Header{
		ParentHash: parent.Hash(),
		Number:     new(big.Int).Add(parent.Number, big.NewInt(1)),
		GasLimit:   parent.GasLimit,
	}
	selection := engine.selectionForHeader(nil, header)
	if selection.Producer == nil {
		t.Fatal("no producer selected")
	}
	header.Coinbase = selection.Producer.Address
	if err := engine.Prepare(chain, header); err != nil {
		t.Fatalf("prepare header: %v", err)
	}
	signTestHeader(t, chain.Config().ChainID, header)
	return header
}

func testParticipantKey(t *testing.T, index int) *ecdsa.PrivateKey {
	t.Helper()
	return testRegistryKey(t, fmt.Sprintf("%064x", index+1))
}

func testParticipants(t *testing.T, count int) []common.Address {
	t.Helper()
	participants := make([]common.Address, count)
	for index := range participants {
		participants[index] = crypto.PubkeyToAddress(testParticipantKey(t, index).PublicKey)
	}
	return participants
}

func signTestHeader(t *testing.T, chainID *big.Int, header *types.Header) {
	t.Helper()
	var key *ecdsa.PrivateKey
	for index := 0; index < 1024; index++ {
		candidate := testParticipantKey(t, index)
		if crypto.PubkeyToAddress(candidate.PublicKey) == header.Coinbase {
			key = candidate
			break
		}
	}
	if key == nil {
		t.Fatalf("no test key for producer %s", header.Coinbase)
	}
	if len(header.Extra) < ProducerSealLength {
		header.Extra = appendEmptyProducerSeal(header.Extra)
	}
	hash, err := producerSealHash(chainID, header)
	if err != nil {
		t.Fatalf("build producer seal hash: %v", err)
	}
	signature, err := crypto.Sign(hash[:], key)
	if err != nil {
		t.Fatalf("sign producer header: %v", err)
	}
	payload, _, err := splitProducerSeal(header.Extra)
	if err != nil {
		t.Fatalf("split producer seal: %v", err)
	}
	header.Extra = append(append([]byte(nil), payload...), signature...)
	if err := VerifyProducerSeal(chainID, header); err != nil {
		t.Fatalf("verify test producer seal: %v", err)
	}
}
