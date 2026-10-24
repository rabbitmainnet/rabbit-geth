package core

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/ethash"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
	"math/big"
	"testing"
)

func TestRabbitIndexedAncestorRealHeaderChainV1(t *testing.T) {
	db := rawdb.NewMemoryDatabase()
	defer db.Close()
	genesis := &types.Header{Number: big.NewInt(0)}
	rawdb.WriteHeader(db, genesis)
	rawdb.WriteCanonicalHash(db, genesis.Hash(), 0)

	var branches [2][]*types.Header
	for branch := 0; branch < 2; branch++ {
		parent := genesis.Hash()
		for number := int64(1); number <= 128; number++ {
			header := &types.Header{
				Number: big.NewInt(number), ParentHash: parent,
				Extra: []byte{byte(branch + 1), byte(number)},
			}
			rawdb.WriteHeader(db, header)
			branches[branch] = append(branches[branch], header)
			parent = header.Hash()
		}
	}
	hc, err := NewHeaderChain(
		db, params.AllEthashProtocolChanges,
		ethash.NewFaker(), func() bool { return false },
	)
	if err != nil {
		t.Fatal(err)
	}

	for canonical := 0; canonical < 2; canonical++ {
		for _, header := range branches[canonical] {
			rawdb.WriteCanonicalHash(db, header.Hash(), header.Number.Uint64())
		}
		for branch := 0; branch < 2; branch++ {
			tip := branches[branch][127]
			for _, target := range []uint64{0, 1, 64, 127, 128} {
				budget := uint64(128)
				hash, number := hc.GetAncestor(
					tip.Hash(), 128, 128-target, &budget,
				)
				want := genesis.Hash()
				if target > 0 {
					want = branches[branch][target-1].Hash()
				}
				if hash != want || number != target {
					t.Fatalf("canonical=%d branch=%d target=%d got=%s/%d",
						canonical, branch, target, hash, number)
				}
			}
		}
	}
	budget := uint64(128)
	hash, _ := hc.GetAncestor(common.HexToHash("0xdeadbeef"), 128, 64, &budget)
	if hash != (common.Hash{}) {
		t.Fatal("missing branch accepted")
	}
}
