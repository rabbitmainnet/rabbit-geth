package core

import (
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/ethash"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
)

func TestLQCLocalRecoveryNetworkGateV2(t *testing.T) {
	for _, mode := range []string{"absent", "missing-target", "await-import", "canonical-restored", "conflicting-target", "missing-state", "corrupt-marker"} {
		t.Run(mode, func(t *testing.T) {
			config := *params.AllEthashProtocolChanges
			config.ChainID = big.NewInt(9280)
			config.Ethash = nil
			config.LQC = &params.LQCConfig{CommitteeMin: 32, CommitteeMax: 128, FallbackSlots: 5, TargetBlockTimeMs: 10000}
			genesis := &Genesis{Config: &config, BaseFee: big.NewInt(params.InitialBaseFee)}
			engine := ethash.NewFullFaker()
			defer engine.Close()
			_, blocks, _ := GenerateChainWithGenesis(genesis, engine, 3, nil)
			db := rawdb.NewMemoryDatabase()
			defer db.Close()
			chain, err := NewBlockChain(db, genesis, engine, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer chain.Stop()
			if _, err := chain.InsertChain(blocks[:1]); err != nil {
				t.Fatal(err)
			}
			target := blocks[2].Hash()
			if mode != "absent" {
				if err := db.Put(lqcLocalRecoveryKeyV1, target[:]); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "corrupt-marker" {
				if err := db.Put(lqcLocalRecoveryKeyV1, []byte{1}); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "await-import" {
				rawdb.WriteBlock(db, blocks[2])
			}
			if mode == "canonical-restored" || mode == "conflicting-target" || mode == "missing-state" {
				if _, err := chain.InsertChain(blocks[1:]); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "conflicting-target" {
				header := types.CopyHeader(blocks[2].Header())
				header.Extra = []byte("different-recovery-target")
				other := types.NewBlockWithHeader(header)
				rawdb.WriteBlock(db, other)
				target = other.Hash()
				if err := db.Put(lqcLocalRecoveryKeyV1, target[:]); err != nil {
					t.Fatal(err)
				}
			}
			realHead := chain.CurrentBlock()
			defer chain.currentBlock.Store(realHead)
			if mode == "missing-state" {
				head := types.CopyHeader(chain.CurrentBlock())
				head.Root = common.HexToHash("0xabcdef")
				chain.currentBlock.Store(head)
			}
			before := chain.CurrentBlock().Hash()
			if mode == "missing-target" {
				err := chain.RecoverLQCLocalStateV1(target)
				if !errors.Is(err, ErrLQCLocalRecoveryNeedsSyncV2) {
					t.Fatalf("missing target not classified for sync: %v", err)
				}
			}
			ready, err := chain.LQCLocalRecoveryReadyV2()
			wantError := mode == "conflicting-target" || mode == "corrupt-marker"
			wantReady := mode == "absent" || mode == "canonical-restored"
			if (err != nil) != wantError || ready != wantReady {
				t.Fatalf("ready=%v err=%v", ready, err)
			}
			if chain.CurrentBlock().Hash() != before {
				t.Fatal("gate changed the head")
			}
			has, err := db.Has(lqcLocalRecoveryKeyV1)
			if err != nil {
				t.Fatal(err)
			}
			if has == wantReady {
				t.Fatalf("persistent guard retained=%v ready=%v", has, ready)
			}
			if mode == "missing-target" || mode == "await-import" {
				if _, err := chain.InsertChain(blocks[1:]); err != nil {
					t.Fatal(err)
				}
				ready, err = chain.LQCLocalRecoveryReadyV2()
				if !ready || err != nil {
					t.Fatalf("normal validated import did not release guard: %v", err)
				}
			}
		})
	}
}
