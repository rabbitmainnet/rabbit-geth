// Copyright 2026 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package miner

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/core/types"
)

type lqcBlockInserterTestChain struct {
	current  *types.Header
	inserted types.Blocks
}

func (chain *lqcBlockInserterTestChain) CurrentBlock() *types.Header {
	return chain.current
}

func (chain *lqcBlockInserterTestChain) InsertChain(blocks types.Blocks) (int, error) {
	chain.inserted = append(chain.inserted, blocks...)
	return len(blocks), nil
}

func TestInsertLQCBlockIfParentCurrentDropsStaleFallback(t *testing.T) {
	parent329 := &types.Header{
		Number: big.NewInt(329),
		Time:   1_000,
		Extra:  []byte("parent-329"),
	}
	fallback330 := types.NewBlockWithHeader(&types.Header{
		ParentHash: parent329.Hash(),
		Number:     big.NewInt(330),
		Time:       1_002,
		Extra:      []byte("fallback-330"),
	})
	primary330 := &types.Header{
		ParentHash: parent329.Hash(),
		Number:     big.NewInt(330),
		Time:       1_001,
		Extra:      []byte("primary-330"),
	}

	chain := &lqcBlockInserterTestChain{current: parent329}
	capturedParent := chain.CurrentBlock()
	chain.current = primary330 // Primary arrives while the fallback is building.

	inserted, err := insertLQCBlockIfParentCurrent(chain, capturedParent, fallback330)
	if err != nil {
		t.Fatal(err)
	}
	if inserted {
		t.Fatal("stale fallback block was reported as inserted")
	}
	if len(chain.inserted) != 0 {
		t.Fatalf("stale fallback inserted %d block(s), want 0", len(chain.inserted))
	}
}

func TestInsertLQCBlockIfParentCurrentAcceptsCurrentChild(t *testing.T) {
	parent := &types.Header{
		Number: big.NewInt(329),
		Time:   1_000,
		Extra:  []byte("parent-329"),
	}
	child := types.NewBlockWithHeader(&types.Header{
		ParentHash: parent.Hash(),
		Number:     big.NewInt(330),
		Time:       1_001,
		Extra:      []byte("primary-330"),
	})
	chain := &lqcBlockInserterTestChain{current: parent}

	inserted, err := insertLQCBlockIfParentCurrent(chain, parent, child)
	if err != nil {
		t.Fatal(err)
	}
	if !inserted {
		t.Fatal("current child was not inserted")
	}
	if len(chain.inserted) != 1 || chain.inserted[0].Hash() != child.Hash() {
		t.Fatalf("inserted blocks = %d, want the current child", len(chain.inserted))
	}
}
