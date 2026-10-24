package eth

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/ethash"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/params"
)

func TestLQCRecoveryKeepsSyncedFeaturesBlockedV2(t *testing.T) {
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
	s := &Ethereum{blockchain: chain, handler: &handler{}}
	s.handler.synced.Store(true)
	if !s.Synced() {
		t.Fatal("healthy node incorrectly blocked")
	}
	target := common.HexToHash("0x123456")
	key := []byte("rabbit-lqc-local-state-recovery-v1")
	if err := db.Put(key, target[:]); err != nil {
		t.Fatal(err)
	}
	if s.Synced() {
		t.Fatal("pending recovery accepted downloader readiness")
	}
	if err := db.Put(key, []byte{1}); err != nil {
		t.Fatal(err)
	}
	if s.Synced() {
		t.Fatal("corrupt recovery marker enabled synced features")
	}
}
