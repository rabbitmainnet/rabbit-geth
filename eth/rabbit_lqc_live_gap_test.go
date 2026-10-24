//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx && rabbit_vrf_localnet

package eth

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/ethash"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/types"
	ethproto "github.com/ethereum/go-ethereum/eth/protocols/eth"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/params"
)

// A complete NewBlock must initiate gap recovery even if the separate range
// announcement/header query is missing or delayed. It must never skip import
// validation, mutate the canonical head, or let a stale gap replace a newer one.
func TestLQCLiveGapSchedulesRecoveryWithoutRangeV1(t *testing.T) {
	config := *params.TestChainConfig
	config.ChainID = big.NewInt(19280)
	config.Ethash = nil
	config.LQC = &params.LQCConfig{CommitteeMin: 32, CommitteeMax: 128, FallbackSlots: 5, EpochLength: 128, TargetBlockTimeMs: 10000}
	chain, err := core.NewBlockChain(rawdb.NewMemoryDatabase(), &core.Genesis{Config: &config}, ethash.NewFaker(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer chain.Stop()
	h := &ethHandler{chain: chain, lqcSyncRunning: true}
	left, right := p2p.MsgPipe()
	defer left.Close()
	defer right.Close()
	peer := ethproto.NewPeer(ethproto.ETH69, p2p.NewPeer(enode.ID{1}, "gap-test", nil), left, nil, nil, nil)
	defer peer.Close()
	head := chain.CurrentBlock().Hash()
	for _, number := range []int64{3, 5, 4} {
		header := &types.Header{Number: big.NewInt(number), ParentHash: common.HexToHash("0xabcdef")}
		if err := h.Handle(peer, &ethproto.NewBlockPacket{Block: types.NewBlockWithHeader(header)}); err != nil {
			t.Fatal(err)
		}
		if chain.CurrentBlock().Hash() != head {
			t.Fatal("unvalidated gap changed canonical state")
		}
		want := number
		if number == 4 {
			want = 5
		}
		if h.lqcSyncPending == nil || h.lqcSyncPending.header.Number.Int64() != want {
			t.Fatalf("NewBlock gap %d lost: no recovery without a separate range response", number)
		}
	}
}
