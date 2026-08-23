package eth

import (
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/eth/ethconfig"
	"github.com/ethereum/go-ethereum/params"
)

func TestLQCWorkTicketLabTransportActivationGate(t *testing.T) {
	lqcChain := &params.ChainConfig{ChainID: big.NewInt(928), LQC: &params.LQCConfig{}}
	mainnetGenesis := types.NewBlockWithHeader(&types.Header{Extra: []byte(frozenRabbitMainnetGenesisMarker)})
	testnetChain := &params.ChainConfig{ChainID: big.NewInt(9280), LQC: &params.LQCConfig{}}
	testnetGenesis := types.NewBlockWithHeader(&types.Header{Extra: []byte(frozenRabbitTestnetGenesisMarker)})
	labGenesis := types.NewBlockWithHeader(&types.Header{Extra: []byte("RABBIT_WORK_TICKET_LAB_V1")})

	disabled := &ethconfig.Config{}
	if err := validateLQCWorkTicketLabTransport(disabled, lqcChain, mainnetGenesis); err != nil {
		t.Fatalf("disabled transport rejected harmless config: %v", err)
	}

	enabled := &ethconfig.Config{WorkTicketLabTransport: true}
	if err := validateLQCWorkTicketLabTransport(enabled, lqcChain, mainnetGenesis); err == nil {
		t.Fatal("frozen Rabbit mainnet genesis accepted laboratory transport")
	}
	if err := validateLQCWorkTicketLabTransport(enabled, testnetChain, testnetGenesis); err == nil {
		t.Fatal("frozen Rabbit testnet genesis accepted laboratory transport")
	}
	if err := validateLQCWorkTicketLabTransport(enabled, &params.ChainConfig{ChainID: big.NewInt(928)}, labGenesis); err == nil {
		t.Fatal("non-LQC chain accepted work-ticket laboratory transport")
	}
	if err := validateLQCWorkTicketLabTransport(enabled, lqcChain, nil); err == nil {
		t.Fatal("nil genesis accepted work-ticket laboratory transport")
	}
	if err := validateLQCWorkTicketLabTransport(enabled, lqcChain, labGenesis); err != nil {
		t.Fatalf("isolated LQC laboratory rejected: %v", err)
	}
}

func TestLQCWorkV1ProductionActivationGate(t *testing.T) {
	lqcChain := &params.ChainConfig{
		ChainID: big.NewInt(928),
		LQC:     &params.LQCConfig{},
	}
	mainnetGenesis := types.NewBlockWithHeader(
		&types.Header{Extra: []byte(frozenRabbitMainnetGenesisMarker)},
	)
	testnetChain := &params.ChainConfig{
		ChainID: big.NewInt(9280),
		LQC:     &params.LQCConfig{},
	}
	testnetGenesis := types.NewBlockWithHeader(
		&types.Header{Extra: []byte(frozenRabbitTestnetGenesisMarker)},
	)
	labGenesis := types.NewBlockWithHeader(
		&types.Header{Extra: []byte("RABBIT_WORK_V1_LIVE_LAB_V1")},
	)

	enabled, production, err := lqcWorkV1TransportActivation(
		&ethconfig.Config{},
		lqcChain,
		mainnetGenesis,
	)
	if lqc.WorkV1ProductionEnabled() {
		if err != nil || !enabled || !production {
			t.Fatalf(
				"production build activation=%t/%t err=%v",
				enabled,
				production,
				err,
			)
		}
	} else if err == nil {
		t.Fatal("non-production build accepted frozen Rabbit mainnet")
	}

	enabled, production, err = lqcWorkV1TransportActivation(
		&ethconfig.Config{},
		testnetChain,
		testnetGenesis,
	)
	if lqc.WorkV1ProductionEnabled() {
		if err != nil || !enabled || !production {
			t.Fatalf(
				"testnet production build activation=%t/%t err=%v",
				enabled,
				production,
				err,
			)
		}
	} else if err == nil {
		t.Fatal("non-production build accepted frozen Rabbit testnet")
	}

	wrongPair := types.NewBlockWithHeader(
		&types.Header{Extra: []byte(frozenRabbitMainnetGenesisMarker)},
	)
	enabled, production, err = lqcWorkV1TransportActivation(
		&ethconfig.Config{},
		testnetChain,
		wrongPair,
	)
	if err != nil || enabled || production {
		t.Fatalf("mismatched chain/marker activated production transport")
	}

	enabled, production, err = lqcWorkV1TransportActivation(
		&ethconfig.Config{},
		lqcChain,
		labGenesis,
	)
	if err != nil || enabled || production {
		t.Fatalf(
			"unrequested lab activation=%t/%t err=%v",
			enabled,
			production,
			err,
		)
	}

	enabled, production, err = lqcWorkV1TransportActivation(
		&ethconfig.Config{WorkTicketLabTransport: true},
		lqcChain,
		labGenesis,
	)
	if err != nil || !enabled || production {
		t.Fatalf(
			"explicit lab activation=%t/%t err=%v",
			enabled,
			production,
			err,
		)
	}
}

func TestLQCForcesFullSyncWithoutChangingEthereumChains(t *testing.T) {
	lqcConfig := &ethconfig.Config{SyncMode: ethconfig.SnapSync}
	changed := enforceLQCFullSync(
		lqcConfig,
		&params.ChainConfig{
			ChainID: big.NewInt(9280),
			LQC:     &params.LQCConfig{},
		},
	)
	if !changed || lqcConfig.SyncMode != ethconfig.FullSync {
		t.Fatalf("LQC sync mode=%v changed=%t, want full/true", lqcConfig.SyncMode, changed)
	}

	fullConfig := &ethconfig.Config{SyncMode: ethconfig.FullSync}
	if enforceLQCFullSync(
		fullConfig,
		&params.ChainConfig{ChainID: big.NewInt(928), LQC: &params.LQCConfig{}},
	) {
		t.Fatal("already-full LQC config reported a change")
	}

	ethereumConfig := &ethconfig.Config{SyncMode: ethconfig.SnapSync}
	if enforceLQCFullSync(
		ethereumConfig,
		&params.ChainConfig{ChainID: big.NewInt(1)},
	) || ethereumConfig.SyncMode != ethconfig.SnapSync {
		t.Fatal("non-LQC chain sync mode was changed")
	}
}

func TestLQCWorkTicketLocalSubmissionUsesGlobalBudget(t *testing.T) {
	transport, err := newLQCWorkTicketTransport(testWorkTicketTransportConfig())
	if err != nil {
		t.Fatal(err)
	}
	transport.globalBudgetUsed = lqcWorkTicketGlobalBudget
	transport.globalBudgetWindowFrom = time.Now()
	ticket := signedTransportWorkTicket(t, transport.chainID, 1, common.Hash{})
	if _, err := transport.Submit(ticket); !errors.Is(err, errLQCWorkTicketRateLimited) {
		t.Fatalf("local submission budget error = %v", err)
	}
}
