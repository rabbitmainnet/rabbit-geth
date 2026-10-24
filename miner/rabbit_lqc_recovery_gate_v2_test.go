package miner

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/ethash"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/params"
)

// A missing target must block even direct miner API startup, not only the
// Ethereum sync-gate caller. A nil backend detects accidental signing access.
func TestLQCProducerBlockedByRecoveryTargetV2(t *testing.T) {
	config := *params.AllEthashProtocolChanges
	config.ChainID = big.NewInt(9280)
	config.Ethash = nil
	config.LQC = &params.LQCConfig{CommitteeMin: 32, CommitteeMax: 128, FallbackSlots: 5, TargetBlockTimeMs: 10000}
	db := rawdb.NewMemoryDatabase()
	defer db.Close()
	engine := ethash.NewFullFaker()
	defer engine.Close()
	genesis := &core.Genesis{Config: &config, BaseFee: big.NewInt(params.InitialBaseFee)}
	chain, err := core.NewBlockChain(db, genesis, engine, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer chain.Stop()
	target := common.HexToHash("0x123456")
	if err := db.Put([]byte("rabbit-lqc-local-state-recovery-v1"), target[:]); err != nil {
		t.Fatal(err)
	}
	miner := &Miner{chainConfig: &config, chain: chain}
	miner.StartLQCDevnet()
	if miner.lqcRunning {
		t.Fatal("producer started with missing recovery target")
	}
	pending, err := chain.LQCLocalRecoveryPendingV2()
	if err != nil || !pending {
		t.Fatalf("recovery guard was discarded: %v", err)
	}
}
