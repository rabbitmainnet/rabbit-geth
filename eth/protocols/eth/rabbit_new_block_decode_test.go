// Copyright 2026 The go-ethereum Authors
// This file is part of the go-ethereum library.

package eth

import (
	"bytes"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
)

func TestRabbitNewBlockPacketRoundTrip(t *testing.T) {
	header := &types.Header{Difficulty: big.NewInt(1), Number: big.NewInt(1)}
	want := &NewBlockPacket{Block: types.NewBlockWithHeader(header), TD: big.NewInt(2)}
	encoded, err := rlp.EncodeToBytes(want)
	if err != nil {
		t.Fatal(err)
	}
	var have NewBlockPacket
	if err := rlp.DecodeBytes(encoded, &have); err != nil {
		t.Fatal(err)
	}
	if have.Block == nil || have.Block.Hash() != want.Block.Hash() {
		t.Fatalf("block mismatch: have %v want %v", have.Block, want.Block.Hash())
	}
	if have.TD == nil || have.TD.Cmp(want.TD) != 0 {
		t.Fatalf("total difficulty mismatch: have %v want %v", have.TD, want.TD)
	}
}

func TestRabbitNewBlockPacketRejectsOversizedItemCountBeforeDecode(t *testing.T) {
	const itemCount = maxNewBlockTransactions + 1
	// Each 0x80 is an empty string and therefore one RLP item. The list is
	// intentionally syntactically valid but would force a large eager slice
	// allocation in the old decoder.
	count := uint32(itemCount)
	txList := append([]byte{0xfa, byte(count >> 16), byte(count >> 8), byte(count)}, bytes.Repeat([]byte{0x80}, itemCount)...)
	header := &types.Header{Difficulty: big.NewInt(1), Number: big.NewInt(1)}
	block, err := rlp.EncodeToBytes(struct {
		Header       *types.Header
		Transactions rlp.RawValue
		Uncles       rlp.RawValue
	}{Header: header, Transactions: txList, Uncles: rlp.RawValue{0xc0}})
	if err != nil {
		t.Fatal(err)
	}
	packet, err := rlp.EncodeToBytes(struct {
		Block rlp.RawValue
		TD    *big.Int
	}{Block: block, TD: big.NewInt(2)})
	if err != nil {
		t.Fatal(err)
	}
	var decoded NewBlockPacket
	err = rlp.DecodeBytes(packet, &decoded)
	if err == nil || !strings.Contains(err.Error(), "too many transactions") {
		t.Fatalf("unexpected error: %v", err)
	}
}
