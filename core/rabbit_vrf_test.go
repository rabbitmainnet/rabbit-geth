// Copyright 2026 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package core

import (
	"bytes"
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/ethash"
	"github.com/ethereum/go-ethereum/consensus/misc"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/holiman/uint256"
)

func TestRabbitVRFCoordinatorV1Artifact(t *testing.T) {
	wantAddr := common.HexToAddress("0xdFc21aeA108e3F527E5f236ebf354dc8262719da")
	if params.RabbitVRFCoordinatorV1Address != wantAddr {
		t.Fatalf("coordinator address = %s, want %s", params.RabbitVRFCoordinatorV1Address, wantAddr)
	}

	wantHash := common.HexToHash("0x287ca3746e61268d230076f24dfc476d62ffd4edd2aadb50cfec176e690edd26")
	gotHash := crypto.Keccak256Hash(params.RabbitVRFCoordinatorV1Code)
	if gotHash != wantHash {
		t.Fatalf("runtime code hash = %s, want %s", gotHash, wantHash)
	}

	if got := len(params.RabbitVRFCoordinatorV1Code); got != 5695 {
		t.Fatalf("runtime byte length = %d, want 5643", got)
	}
}

func TestApplyRabbitVRFCoordinatorV1(t *testing.T) {
	sdb := mkState(nil)

	misc.ApplyRabbitVRFCoordinatorV1(sdb)

	addr := params.RabbitVRFCoordinatorV1Address

	if got := sdb.GetCode(addr); !bytes.Equal(got, params.RabbitVRFCoordinatorV1Code) {
		t.Fatalf("coordinator code mismatch:\n got %x\nwant %x", got, params.RabbitVRFCoordinatorV1Code)
	}
	if got := sdb.GetNonce(addr); got != 1 {
		t.Fatalf("coordinator nonce = %d, want 1", got)
	}
	if got := sdb.GetBalance(addr); !got.IsZero() {
		t.Fatalf("coordinator balance = %s, want 0", got)
	}
}

func TestApplyRabbitVRFCoordinatorV1ResetsStatePreservesBalance(t *testing.T) {
	sdb := mkState(nil)

	addr := params.RabbitVRFCoordinatorV1Address
	slot := common.HexToHash("0x01")
	value := common.HexToHash("0xdeadbeef")

	sdb.SetBalance(addr, uint256.NewInt(123456789), tracing.BalanceChangeUnspecified)
	sdb.SetNonce(addr, 77, tracing.NonceChangeNewContract)
	sdb.SetCode(addr, []byte{0x60, 0x00}, tracing.CodeChangeUnspecified)
	sdb.SetState(addr, slot, value)

	misc.ApplyRabbitVRFCoordinatorV1(sdb)

	if got := sdb.GetBalance(addr); got.Cmp(uint256.NewInt(123456789)) != 0 {
		t.Fatalf("pre-fork balance not preserved: got %s", got)
	}
	if got := sdb.GetNonce(addr); got != 1 {
		t.Fatalf("old nonce survived transition: got %d, want 1", got)
	}
	if got := sdb.GetState(addr, slot); got != (common.Hash{}) {
		t.Fatalf("old storage survived transition: got %s", got)
	}
	if got := sdb.GetCode(addr); !bytes.Equal(got, params.RabbitVRFCoordinatorV1Code) {
		t.Fatal("canonical code not installed")
	}
}

func TestRabbitVRFCoordinatorV1OnlySystem(t *testing.T) {
	sdb := mkState(nil)
	misc.ApplyRabbitVRFCoordinatorV1(sdb)

	addr := params.RabbitVRFCoordinatorV1Address
	input := crypto.Keccak256([]byte("systemObservePrice()"))[:4]

	_, _, err := amsterdamCoreEVM(sdb).Call(
		common.Address{0xca},
		addr,
		input,
		vm.NewGasBudget(1_000_000, 0),
		new(uint256.Int),
	)
	if err == nil {
		t.Fatal("non-system caller unexpectedly accepted")
	}

	_, _, err = amsterdamCoreEVM(sdb).Call(
		params.SystemAddress,
		addr,
		input,
		vm.NewGasBudget(1_000_000, 0),
		new(uint256.Int),
	)
	if err != nil {
		t.Fatalf("system caller rejected: %v", err)
	}
}

func rabbitVRFTestConfig(activation uint64) *params.ChainConfig {
	config := *params.TestChainConfig
	config.LQC = &params.LQCConfig{
		EpochLength:      1,
		VRFProtocolBlock: activation,
	}
	return &config
}

func TestRabbitVRFCoordinatorV1PreExecutionActivationBoundary(t *testing.T) {
	config := rabbitVRFTestConfig(4)
	sdb := mkState(nil)
	evm := amsterdamCoreEVM(sdb)

	addr := params.RabbitVRFCoordinatorV1Address
	slot := common.HexToHash("0x1234")
	value := common.HexToHash("0xcafebabe")

	// Block 3: fork inactive.
	parent0 := &types.Header{
		Number: big.NewInt(2),
		Time:   20,
	}

	PreExecution(
		context.Background(),
		nil,
		parent0,
		config,
		evm,
		big.NewInt(3),
		30,
	)

	if got := sdb.GetCode(addr); len(got) != 0 {
		t.Fatalf("coordinator installed before fork: %x", got)
	}

	// Block 4: exact activation boundary.
	parent1 := &types.Header{
		Number: big.NewInt(3),
		Time:   30,
	}

	PreExecution(
		context.Background(),
		nil,
		parent1,
		config,
		evm,
		big.NewInt(4),
		40,
	)

	if got := sdb.GetCode(addr); !bytes.Equal(got, params.RabbitVRFCoordinatorV1Code) {
		t.Fatal("coordinator not installed at activation block")
	}

	// Simulate protocol storage written after installation.
	sdb.SetState(addr, slot, value)

	// Block 5: already active. The installer must NOT run again.
	parent2 := &types.Header{
		Number: big.NewInt(4),
		Time:   40,
	}

	PreExecution(
		context.Background(),
		nil,
		parent2,
		config,
		evm,
		big.NewInt(5),
		50,
	)

	if got := sdb.GetState(addr, slot); got != value {
		t.Fatalf("coordinator storage reset after activation: got %s want %s", got, value)
	}

	if got := sdb.GetCode(addr); !bytes.Equal(got, params.RabbitVRFCoordinatorV1Code) {
		t.Fatal("coordinator code changed after activation")
	}
}

func TestRabbitVRFCoordinatorV1ChainMakerActivationBoundary(t *testing.T) {
	config := rabbitVRFTestConfig(4)

	addr := params.RabbitVRFCoordinatorV1Address
	slot := common.HexToHash("0x5678")
	value := common.HexToHash("0xfeedface")

	genesis := &Genesis{
		Config: config,
	}

	_, blocks, _ := GenerateChainWithGenesis(
		genesis,
		ethash.NewFaker(),
		5,
		func(i int, b *BlockGen) {
			number := b.Number().Uint64()

			switch number {
			case 3:
				if got := b.statedb.GetCode(addr); len(got) != 0 {
					t.Fatalf("block 3: coordinator installed before fork: %x", got)
				}

				rabbitVRFTestSetPair(
					b.statedb,
					big.NewInt(0),
					1,
					1,
					0,
				)

			case 4:
				if got := b.statedb.GetCode(addr); !bytes.Equal(got, params.RabbitVRFCoordinatorV1Code) {
					t.Fatal("block 4: coordinator not installed at activation")
				}

				if got := b.statedb.GetState(
					addr,
					common.Hash{},
				); got != common.BigToHash(big.NewInt(1)) {
					t.Fatalf(
						"block 4: oracle observation missing: got %s want 0x1",
						got,
					)
				}

				b.statedb.SetState(addr, slot, value)

			case 5:
				if got := b.statedb.GetCode(addr); !bytes.Equal(got, params.RabbitVRFCoordinatorV1Code) {
					t.Fatal("block 5: coordinator code missing")
				}

				if got := b.statedb.GetState(
					addr,
					common.Hash{},
				); got != common.BigToHash(big.NewInt(1)) {
					t.Fatalf(
						"block 5: oracle observation changed unexpectedly: got %s want 0x1",
						got,
					)
				}

				if got := b.statedb.GetState(addr, slot); got != value {
					t.Fatalf("block 5: coordinator storage was reset: got %s want %s", got, value)
				}
			}
		},
	)

	if len(blocks) != 5 {
		t.Fatalf("generated %d blocks, want 5", len(blocks))
	}
}
