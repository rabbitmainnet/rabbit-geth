//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package lqc

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
	"github.com/holiman/uint256"
)

func TestRewardV5CreditsProducerStateAfterActivation(t *testing.T) {
	chainID := big.NewInt(9280)
	fixture := headerV4RuntimeContextV1(
		t, chainID, NewCommitteeClaimLedgerV1(), nil,
	)

	headers := make(map[common.Hash]*types.Header)
	canonical := make(map[uint64]*types.Header)
	var previous *types.Header
	for n := uint64(0); n <= 128; n++ {
		h := &types.Header{
			Number:   new(big.Int).SetUint64(n),
			Time:     100 + n*10,
			GasLimit: 30000000,
		}
		if previous != nil {
			h.ParentHash = previous.Hash()
		}
		headers[h.Hash()] = h
		canonical[n] = h
		previous = h
	}

	config := &params.LQCConfig{
		ConsensusLivenessV3Block: 1,
		MiningRewardV5Block:      129,
		VRFProtocolBlock:         1,
	}
	chain := &workV1BranchTestChain{
		testHeaderChain: &testHeaderChain{
			config: &params.ChainConfig{
				ChainID: chainID,
				LQC:     config,
			},
			headers: headers,
			current: previous,
		},
		canonical: canonical,
	}
	engine := New(config, rawdb.NewMemoryDatabase())
	parent := fixture.Work.Parent
	parent.Work.Hash = previous.Hash()
	if err := engine.workV1EngineLabRemember(previous.Hash(), parent); err != nil {
		t.Fatal(err)
	}
	if err := engine.workV1EngineLabRememberClaimLedger(
		previous.Hash(), NewCommitteeClaimLedgerV1(),
	); err != nil {
		t.Fatal(err)
	}

	work, err := engine.workV1EngineLabContext(
		chain, parent, 129, fixture.Work.RegistryRoot,
	)
	if err != nil {
		t.Fatal(err)
	}
	context, err := engine.workV1EngineLabV4Context(
		chain, work, previous.Hash(),
	)
	if err != nil {
		t.Fatal(err)
	}
	v4extra, _, _, err := BuildLQCHeaderExtraV4WithCanonicalRuntimeV1(
		context, nil, nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	v4, err := ValidateLQCHeaderExtraV4(
		129, MaxWorkTicketsPerBlockV1, v4extra,
	)
	if err != nil {
		t.Fatal(err)
	}
	extra, err := EncodeLQCHeaderExtraV5(
		129, v4.RegistryRoot, v4.WorkStateRoot,
		v4.CommitteeClaimRoot, v4.RegistryOperations,
		v4.WorkTickets, v4.CommitteeParticipationClaims,
		nil, MaxWorkTicketsPerBlockV1,
	)
	if err != nil {
		t.Fatal(err)
	}
	producer := HybridParticipant{
		Address: common.HexToAddress(
			"0xd61952bfc5708907e2985cc7f538e2e236acc118",
		),
	}
	committee := common.HexToAddress("0x2001")
	header := &types.Header{
		Number:     big.NewInt(129),
		ParentHash: previous.Hash(),
		Coinbase:   producer.Address,
		Time:       previous.Time + 10,
		GasLimit:   previous.GasLimit,
		Extra:      extra,
	}
	if _, _, _, _, err := ValidateAndApplyLQCHeaderExtraV5WithCanonicalRuntimeV1(
		context, header.Hash(), extra,
	); err != nil {
		t.Fatalf("canonical V5 transition invalid: %v", err)
	}
	selection := HybridSelection{
		Producer:  &producer,
		Ordered:   []HybridParticipant{producer},
		Committee: []HybridParticipant{{Address: committee}},
	}

	for _, enabled := range []bool{false, true} {
		name := "historical"
		if enabled {
			name = "activated"
		}
		t.Run(name, func(t *testing.T) {
			config.MiningRewardV5Block = 0
			if enabled {
				config.MiningRewardV5Block = 129
			}
			sdb, err := state.New(
				types.EmptyRootHash, state.NewDatabaseForTesting(),
			)
			if err != nil {
				t.Fatal(err)
			}
			engine.distributeWorkV1ClaimRewardsV3(
				chain, header, sdb,
				uint256.NewInt(1200000000000000000),
				selection,
			)
			want := uint256.NewInt(0)
			if enabled {
				want = uint256.NewInt(840000000000000000)
			}
			got := sdb.GetBalance(producer.Address)
			if got.Cmp(want) != 0 {
				t.Fatalf("producer balance=%s want=%s", got, want)
			}
			if !sdb.GetBalance(committee).IsZero() {
				t.Fatal("committee paid without a participation claim")
			}
			t.Logf("producer credited=%s wei", got)
		})
	}
}
