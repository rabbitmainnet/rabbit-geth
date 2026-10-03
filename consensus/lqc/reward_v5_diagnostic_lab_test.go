//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package lqc

import (
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/holiman/uint256"
)

type rewardV5DiagnosticChain struct {
	consensus.ChainHeaderReader
}

func TestRewardV5DiagnosticSilentNoCredit(t *testing.T) {
	extra, err := EncodeLQCHeaderExtraV5(
		142343,
		common.HexToHash("0x31"),
		common.HexToHash("0x32"),
		common.HexToHash("0x33"),
		nil, nil, nil, nil,
		MaxWorkTicketsPerBlockV1,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateLQCHeaderExtraV5(
		142343, MaxWorkTicketsPerBlockV1, extra,
	); err != nil {
		t.Fatalf("fixture V5 invalida: %v", err)
	}

	producer := HybridParticipant{
		Address: common.HexToAddress(
			"0xd61952bfc5708907e2985cc7f538e2e236acc118",
		),
	}
	header := &types.Header{
		Number:   big.NewInt(142343),
		Coinbase: producer.Address,
		Extra:    extra,
	}
	engine := &LQC{}
	chain := &rewardV5DiagnosticChain{}

	_, _, _, err = engine.workV1EngineLabVerifiedClaimsForRewardV3(
		chain, header,
	)
	if err == nil || !strings.Contains(err.Error(), "too many elements") {
		t.Fatalf("erro V5/V4 esperado; recebido: %v", err)
	}
	t.Logf("ERRO REPRODUZIDO: %v", err)

	selection := HybridSelection{
		Producer: &producer,
		Ordered:  []HybridParticipant{producer},
		Committee: []HybridParticipant{
			{Address: common.HexToAddress("0x2001")},
		},
	}
	if allowed, _ := engine.isAuthorAllowedAt(
		142343, selection, producer.Address,
	); !allowed {
		t.Fatal("fixture nao autoriza produtor")
	}

	sdb, err := state.New(
		types.EmptyRootHash, state.NewDatabaseForTesting(),
	)
	if err != nil {
		t.Fatal(err)
	}
	handled := engine.distributeWorkV1ClaimRewardsV3(
		chain, header, sdb,
		uint256.NewInt(1200000000000000000),
		selection,
	)
	balance := sdb.GetBalance(producer.Address)
	if !handled || !balance.IsZero() {
		t.Fatalf("handled=%v balance=%s", handled, balance)
	}
	t.Log("BUG REPRODUZIDO: produtor autorizado, header V5 valido, credito zero")
}
