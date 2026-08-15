package lqc

import (
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/misc/eip1559"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
)

func londonHeaderSecurityFixture(t *testing.T) (*LQC, *testHeaderChain, *types.Header, *types.Header) {
	t.Helper()
	ResetRuntimeRegistry()
	t.Cleanup(ResetRuntimeRegistry)

	producer := testParticipants(t, 1)[0]
	lqcConfig := &params.LQCConfig{
		RegistryMode:          "bootstrap",
		BootstrapParticipants: []common.Address{producer},
		TargetBlockTimeMs:     15_000,
		FallbackWindowMs:      5_000,
	}
	chainConfig := &params.ChainConfig{
		ChainID:     big.NewInt(928),
		LondonBlock: big.NewInt(0),
		LQC:         lqcConfig,
	}
	parent := &types.Header{
		Number:     big.NewInt(0),
		Time:       100,
		GasLimit:   30_000_000,
		GasUsed:    0,
		BaseFee:    new(big.Int).SetUint64(params.InitialBaseFee),
		Difficulty: big.NewInt(0),
	}
	chain := &testHeaderChain{
		config:  chainConfig,
		headers: map[common.Hash]*types.Header{parent.Hash(): parent},
		current: parent,
	}
	header := &types.Header{
		ParentHash: parent.Hash(),
		Coinbase:   producer,
		Difficulty: big.NewInt(0),
		Number:     big.NewInt(1),
		GasLimit:   parent.GasLimit,
		GasUsed:    0,
		Time:       115,
		Extra:      appendEmptyProducerSeal([]byte("LQC:1:1")),
		BaseFee:    eip1559.CalcBaseFee(chainConfig, parent),
	}
	engine := New(lqcConfig, nil)
	signTestHeader(t, chainConfig.ChainID, header)
	return engine, chain, parent, header
}

func TestLQCHeaderSecurityAcceptsValidLondonHeader(t *testing.T) {
	engine, chain, _, header := londonHeaderSecurityFixture(t)
	if err := engine.verifyHeaderAt(chain, header, nil, header.Time); err != nil {
		t.Fatalf("valid London header rejected: %v", err)
	}
}

func TestLQCHeaderSecurityFutureTimestampBoundary(t *testing.T) {
	engine, chain, _, header := londonHeaderSecurityFixture(t)

	atBoundary := types.CopyHeader(header)
	atBoundary.Time = 130
	signTestHeader(t, chain.Config().ChainID, atBoundary)
	if err := engine.verifyHeaderAt(chain, atBoundary, nil, 100); err != nil {
		t.Fatalf("header at future tolerance boundary rejected: %v", err)
	}

	beyondBoundary := types.CopyHeader(header)
	beyondBoundary.Time = 131
	if err := engine.verifyHeaderAt(chain, beyondBoundary, nil, 100); !errors.Is(err, consensus.ErrFutureBlock) {
		t.Fatalf("future timestamp error = %v, want %v", err, consensus.ErrFutureBlock)
	}
}

func TestLQCHeaderSecurityRejectsInvalidGasAndBaseFee(t *testing.T) {
	engine, chain, _, header := londonHeaderSecurityFixture(t)

	tests := []struct {
		name   string
		mutate func(*types.Header)
	}{
		{
			name: "gas used above gas limit",
			mutate: func(candidate *types.Header) {
				candidate.GasUsed = candidate.GasLimit + 1
			},
		},
		{
			name: "gas limit jump",
			mutate: func(candidate *types.Header) {
				candidate.GasLimit += candidate.GasLimit/params.GasLimitBoundDivisor + 1
			},
		},
		{
			name: "wrong base fee",
			mutate: func(candidate *types.Header) {
				candidate.BaseFee = new(big.Int).Add(candidate.BaseFee, big.NewInt(1))
			},
		},
		{
			name: "missing base fee",
			mutate: func(candidate *types.Header) {
				candidate.BaseFee = nil
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := types.CopyHeader(header)
			test.mutate(candidate)
			if err := engine.verifyHeaderAt(chain, candidate, nil, candidate.Time); err == nil {
				t.Fatal("invalid header accepted")
			}
		})
	}
}

func TestLQCHeaderSecurityRejectsUnconfiguredForkFields(t *testing.T) {
	engine, chain, _, header := londonHeaderSecurityFixture(t)
	zeroHash := common.Hash{}
	zeroUint64 := uint64(0)

	tests := []struct {
		name   string
		mutate func(*types.Header)
	}{
		{"withdrawalsHash", func(candidate *types.Header) { candidate.WithdrawalsHash = &zeroHash }},
		{"excessBlobGas", func(candidate *types.Header) { candidate.ExcessBlobGas = &zeroUint64 }},
		{"blobGasUsed", func(candidate *types.Header) { candidate.BlobGasUsed = &zeroUint64 }},
		{"parentBeaconRoot", func(candidate *types.Header) { candidate.ParentBeaconRoot = &zeroHash }},
		{"requestsHash", func(candidate *types.Header) { candidate.RequestsHash = &zeroHash }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := types.CopyHeader(header)
			test.mutate(candidate)
			if err := engine.verifyHeaderAt(chain, candidate, nil, candidate.Time); err == nil {
				t.Fatal("unconfigured fork field accepted")
			}
		})
	}
}

func TestLQCHeaderSecurityRejectsUnsupportedForkActivation(t *testing.T) {
	engine, chain, _, header := londonHeaderSecurityFixture(t)
	activation := uint64(0)
	chain.config.ShanghaiTime = &activation
	if err := engine.verifyHeaderAt(chain, header, nil, header.Time); err == nil {
		t.Fatal("unsupported Shanghai activation accepted")
	}
}

func TestLQCHeaderSecurityRejectsOversizedBlockNumber(t *testing.T) {
	engine, chain, _, header := londonHeaderSecurityFixture(t)
	header.Number = new(big.Int).Lsh(big.NewInt(1), 65)
	if err := engine.verifyHeaderAt(chain, header, nil, header.Time); !errors.Is(err, errInvalidBlockNumber) {
		t.Fatalf("oversized number error = %v, want %v", err, errInvalidBlockNumber)
	}
}

func TestLQCHeaderSecurityAcceptsFrozenGenesisExtraData(t *testing.T) {
	engine := New(&params.LQCConfig{}, nil)
	genesis := &types.Header{
		Number: big.NewInt(0),
		Extra:  []byte("RABBIT_MAINNET_GENESIS_V1"),
	}
	if err := engine.verifyHeaderAt(nil, genesis, nil, 0); err != nil {
		t.Fatalf("frozen genesis rejected: %v", err)
	}
}

func TestLQCHeaderSecurityDetectsSlotTimeOverflow(t *testing.T) {
	engine := &LQC{}
	if _, err := engine.minAllowedTimeChecked(^uint64(0)-5, 0); !errors.Is(err, errBlockTimeOverflow) {
		t.Fatalf("slot overflow error = %v, want %v", err, errBlockTimeOverflow)
	}
	if got := engine.minAllowedTime(^uint64(0)-5, 0); got != ^uint64(0) {
		t.Fatalf("saturated slot time = %d, want %d", got, ^uint64(0))
	}
}
