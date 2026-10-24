//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package lqc

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core/types"
	"math/big"
	"testing"
)

type rabbitAncestorReaderTestV1 struct {
	consensus.ChainHeaderReader
	headers map[common.Hash]*types.Header
	reads   int
}

func (r *rabbitAncestorReaderTestV1) GetHeader(hash common.Hash, number uint64) *types.Header {
	r.reads++
	header := r.headers[hash]
	if header == nil || header.Number.Uint64() != number {
		return nil
	}
	return header
}

type rabbitIndexedReaderTestV1 struct {
	*rabbitAncestorReaderTestV1
	calls int
}

func (r *rabbitIndexedReaderTestV1) GetAncestor(
	hash common.Hash, number, distance uint64, budget *uint64,
) (common.Hash, uint64) {
	r.calls++
	for distance > 0 {
		header := r.headers[hash]
		if header == nil || header.Number.Uint64() != number {
			return common.Hash{}, 0
		}
		hash = header.ParentHash
		number--
		distance--
	}
	return hash, number
}

func TestRabbitVRFIndexedAncestorBranchesV1(t *testing.T) {
	reader := &rabbitAncestorReaderTestV1{headers: make(map[common.Hash]*types.Header)}
	genesis := &types.Header{Number: big.NewInt(0)}
	reader.headers[genesis.Hash()] = genesis
	tips := []common.Hash{}
	for branch := byte(1); branch <= 2; branch++ {
		parent := genesis.Hash()
		for number := int64(1); number <= 128; number++ {
			header := &types.Header{
				Number: big.NewInt(number), ParentHash: parent,
				Extra: []byte{branch, byte(number)},
			}
			reader.headers[header.Hash()] = header
			parent = header.Hash()
		}
		tips = append(tips, parent)
	}
	indexed := &rabbitIndexedReaderTestV1{rabbitAncestorReaderTestV1: reader}
	for _, tip := range tips {
		for _, target := range []uint64{0, 1, 64, 127, 128} {
			want := workV1EngineLabAncestorHeader(reader, 128, tip, target)
			got := workV1EngineLabAncestorHeader(indexed, 128, tip, target)
			if want == nil || got == nil || got.Hash() != want.Hash() {
				t.Fatalf("branch ancestor mismatch at %d", target)
			}
		}
	}
	reader.reads = 0
	got := workV1EngineLabAncestorHeader(indexed, 128, tips[0], 0)
	if got == nil || reader.reads != 1 || indexed.calls == 0 {
		t.Fatalf("indexed path not used: reads=%d", reader.reads)
	}
	if workV1EngineLabAncestorHeader(indexed, 128, tips[0], 129) != nil {
		t.Fatal("future ancestor accepted")
	}
	if workV1EngineLabAncestorHeader(indexed, 128, common.Hash{}, 0) != nil {
		t.Fatal("zero origin accepted")
	}
	missing := common.HexToHash("0xdeadbeef")
	if workV1EngineLabAncestorHeader(indexed, 128, missing, 0) != nil {
		t.Fatal("missing origin accepted")
	}
}
