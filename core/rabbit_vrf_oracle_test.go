// Copyright 2026 The go-ethereum Authors
// This file is part of the go-ethereum library.

package core

import (
	"context"
	"math"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/misc"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/core/vm/program"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
)

var rabbitVRFTestPairAddress = common.HexToAddress("0x8b9f4581b71964049ac6be03b22000132438b385")

func rabbitVRFTestEVM(sdb *state.StateDB, timestamp uint64) *vm.EVM {
	ctx := vm.BlockContext{
		CanTransfer: CanTransfer,
		Transfer:    Transfer,
		GetHash: func(uint64) common.Hash {
			return common.Hash{}
		},
		BlockNumber: big.NewInt(1),
		Time:        timestamp,
		Difficulty:  big.NewInt(1),
		BaseFee:     big.NewInt(0),
		BlobBaseFee: big.NewInt(0),
		GasLimit:    60_000_000,
	}
	return vm.NewEVM(
		ctx,
		sdb,
		params.TestChainConfig,
		vm.Config{NoBaseFee: true},
	)
}

func rabbitVRFTestPairCode() []byte {
	cumulativeSelector :=
		crypto.Keccak256([]byte("price0CumulativeLast()"))[:4]

	reservesSelector :=
		crypto.Keccak256([]byte("getReserves()"))[:4]

	p := program.New()

	// Jump over handlers into selector dispatch.
	p.Op(vm.PUSH2)
	dispatchPatch := p.Size()
	p.Append([]byte{0x00, 0x00})
	p.Op(vm.JUMP)

	// price0CumulativeLast()
	_, cumulativePC := p.Jumpdest()
	p.Push(0)
	p.Op(vm.SLOAD)
	p.Push(0)
	p.Op(vm.MSTORE)
	p.Return(0, 32)

	// getReserves()
	_, reservesPC := p.Jumpdest()

	p.Push(1)
	p.Op(vm.SLOAD)
	p.Push(0)
	p.Op(vm.MSTORE)

	p.Push(2)
	p.Op(vm.SLOAD)
	p.Push(32)
	p.Op(vm.MSTORE)

	p.Push(3)
	p.Op(vm.SLOAD)
	p.Push(64)
	p.Op(vm.MSTORE)

	p.Return(0, 96)

	// ABI selector dispatcher.
	_, dispatchPC := p.Jumpdest()

	p.Push(0)
	p.Op(vm.CALLDATALOAD)
	p.Push(224)
	p.Op(vm.SHR)

	p.Op(vm.DUP1)
	p.Push(cumulativeSelector)
	p.Op(vm.EQ)
	p.Push(cumulativePC)
	p.Op(vm.JUMPI)

	p.Push(reservesSelector)
	p.Op(vm.EQ)
	p.Push(reservesPC)
	p.Op(vm.JUMPI)

	p.Push(0)
	p.Push(0)
	p.Op(vm.REVERT)

	code := append([]byte(nil), p.Bytes()...)

	if dispatchPC > math.MaxUint16 {
		panic("test pair dispatcher exceeds PUSH2")
	}

	code[dispatchPatch] = byte(dispatchPC >> 8)
	code[dispatchPatch+1] = byte(dispatchPC)

	return code
}

func rabbitVRFTestSetPair(
	sdb *state.StateDB,
	cumulative *big.Int,
	reserve0 uint64,
	reserve1 uint64,
	timestampLast uint32,
) {
	addr := rabbitVRFTestPairAddress

	sdb.CreateAccount(addr)
	sdb.SetCode(
		addr,
		rabbitVRFTestPairCode(),
		tracing.CodeChangeUnspecified,
	)

	sdb.SetState(
		addr,
		common.BigToHash(big.NewInt(0)),
		common.BigToHash(cumulative),
	)
	sdb.SetState(
		addr,
		common.BigToHash(big.NewInt(1)),
		common.BigToHash(new(big.Int).SetUint64(reserve0)),
	)
	sdb.SetState(
		addr,
		common.BigToHash(big.NewInt(2)),
		common.BigToHash(new(big.Int).SetUint64(reserve1)),
	)
	sdb.SetState(
		addr,
		common.BigToHash(big.NewInt(3)),
		common.BigToHash(new(big.Int).SetUint64(uint64(timestampLast))),
	)
}

func rabbitVRFTestCall(
	t *testing.T,
	sdb *state.StateDB,
	timestamp uint64,
	caller common.Address,
	signature string,
	argument []byte,
) []byte {
	t.Helper()

	selector := crypto.Keccak256([]byte(signature))[:4]
	input := make([]byte, 4+len(argument))
	copy(input[:4], selector)
	copy(input[4:], argument)

	ret, _, err := rabbitVRFTestEVM(sdb, timestamp).Call(
		caller,
		params.RabbitVRFCoordinatorV1Address,
		input,
		vm.NewGasBudget(30_000_000, 30_000_000),
		common.U2560,
	)
	if err != nil {
		t.Fatalf("%s call failed: %v", signature, err)
	}
	return ret
}

func rabbitVRFTestObserve(
	t *testing.T,
	sdb *state.StateDB,
	timestamp uint64,
) {
	t.Helper()

	rabbitVRFTestCall(
		t,
		sdb,
		timestamp,
		params.SystemAddress,
		"systemObservePrice()",
		nil,
	)
}

func rabbitVRFTestUintView(
	t *testing.T,
	sdb *state.StateDB,
	timestamp uint64,
	signature string,
) uint64 {
	t.Helper()

	ret := rabbitVRFTestCall(
		t,
		sdb,
		timestamp,
		common.Address{0x01},
		signature,
		nil,
	)

	if len(ret) != 32 {
		t.Fatalf("%s returned %d bytes, want 32", signature, len(ret))
	}

	return new(big.Int).SetBytes(ret).Uint64()
}

func rabbitVRFTestLatest(
	t *testing.T,
	sdb *state.StateDB,
	timestamp uint64,
) (bool, uint64, *big.Int) {
	t.Helper()

	ret := rabbitVRFTestCall(
		t,
		sdb,
		timestamp,
		common.Address{0x01},
		"latestObservation()",
		nil,
	)

	if len(ret) != 96 {
		t.Fatalf(
			"latestObservation returned %d bytes, want 96",
			len(ret),
		)
	}

	initialized := new(big.Int).SetBytes(ret[0:32]).Sign() != 0
	observedAt := new(big.Int).SetBytes(ret[32:64]).Uint64()
	cumulative := new(big.Int).SetBytes(ret[64:96])

	return initialized, observedAt, cumulative
}

func rabbitVRFTestObservationAt(
	t *testing.T,
	sdb *state.StateDB,
	timestamp uint64,
	slot byte,
) (uint64, *big.Int) {
	t.Helper()

	arg := make([]byte, 32)
	arg[31] = slot

	ret := rabbitVRFTestCall(
		t,
		sdb,
		timestamp,
		common.Address{0x01},
		"observationAt(uint8)",
		arg,
	)

	if len(ret) != 64 {
		t.Fatalf(
			"observationAt returned %d bytes, want 64",
			len(ret),
		)
	}

	observedAt := new(big.Int).SetBytes(ret[0:32]).Uint64()
	cumulative := new(big.Int).SetBytes(ret[32:64])

	return observedAt, cumulative
}

func TestRabbitVRFPriceObservationCadenceAndCounterfactual(
	t *testing.T,
) {
	sdb := mkState(nil)
	misc.ApplyRabbitVRFCoordinatorV1(sdb)

	stored := big.NewInt(7)

	rabbitVRFTestSetPair(
		sdb,
		stored,
		2,
		5,
		100,
	)

	priceX112 := new(big.Int).Lsh(big.NewInt(5), 112)
	priceX112.Div(priceX112, big.NewInt(2))

	rabbitVRFTestObserve(t, sdb, 400)

	if got := rabbitVRFTestUintView(
		t,
		sdb,
		400,
		"observationCount()",
	); got != 1 {
		t.Fatalf("count after first observation = %d, want 1", got)
	}

	if got := rabbitVRFTestUintView(
		t,
		sdb,
		400,
		"observationIndex()",
	); got != 0 {
		t.Fatalf("index after first observation = %d, want 0", got)
	}

	initialized, observedAt, cumulative :=
		rabbitVRFTestLatest(t, sdb, 400)

	if !initialized {
		t.Fatal("first observation not initialized")
	}
	if observedAt != 400 {
		t.Fatalf("first observedAt = %d, want 400", observedAt)
	}

	want400 := new(big.Int).Mul(
		new(big.Int).Set(priceX112),
		big.NewInt(300),
	)
	want400.Add(want400, stored)

	if cumulative.Cmp(want400) != 0 {
		t.Fatalf(
			"first cumulative = %s, want %s",
			cumulative,
			want400,
		)
	}

	// 299 seconds is below the frozen 300-second cadence.
	rabbitVRFTestObserve(t, sdb, 699)

	if got := rabbitVRFTestUintView(
		t,
		sdb,
		699,
		"observationCount()",
	); got != 1 {
		t.Fatalf("count at +299s = %d, want 1", got)
	}

	_, observedAt, _ = rabbitVRFTestLatest(t, sdb, 699)

	if observedAt != 400 {
		t.Fatalf(
			"latest changed before cadence: got %d want 400",
			observedAt,
		)
	}

	// Exactly 300 seconds must append the next observation.
	rabbitVRFTestObserve(t, sdb, 700)

	if got := rabbitVRFTestUintView(
		t,
		sdb,
		700,
		"observationCount()",
	); got != 2 {
		t.Fatalf("count at +300s = %d, want 2", got)
	}

	if got := rabbitVRFTestUintView(
		t,
		sdb,
		700,
		"observationIndex()",
	); got != 1 {
		t.Fatalf("index at +300s = %d, want 1", got)
	}

	_, observedAt, cumulative =
		rabbitVRFTestLatest(t, sdb, 700)

	if observedAt != 700 {
		t.Fatalf("second observedAt = %d, want 700", observedAt)
	}

	want700 := new(big.Int).Mul(
		new(big.Int).Set(priceX112),
		big.NewInt(600),
	)
	want700.Add(want700, stored)

	if cumulative.Cmp(want700) != 0 {
		t.Fatalf(
			"second cumulative = %s, want %s",
			cumulative,
			want700,
		)
	}
}

func TestRabbitVRFPriceObservationRejectsZeroReserve(
	t *testing.T,
) {
	sdb := mkState(nil)
	misc.ApplyRabbitVRFCoordinatorV1(sdb)

	rabbitVRFTestSetPair(
		sdb,
		big.NewInt(123),
		0,
		5,
		100,
	)

	rabbitVRFTestObserve(t, sdb, 400)

	if got := rabbitVRFTestUintView(
		t,
		sdb,
		400,
		"observationCount()",
	); got != 0 {
		t.Fatalf("zero-reserve count = %d, want 0", got)
	}

	initialized, observedAt, cumulative :=
		rabbitVRFTestLatest(t, sdb, 400)

	if initialized || observedAt != 0 || cumulative.Sign() != 0 {
		t.Fatalf(
			"zero reserve mutated oracle: initialized=%v time=%d cumulative=%s",
			initialized,
			observedAt,
			cumulative,
		)
	}
}

func rabbitVRFTestPairReturnCode(
	cumulativeData []byte,
	reservesData []byte,
) []byte {
	cumulativeSelector :=
		crypto.Keccak256([]byte("price0CumulativeLast()"))[:4]

	reservesSelector :=
		crypto.Keccak256([]byte("getReserves()"))[:4]

	p := program.New()

	p.Op(vm.PUSH2)
	dispatchPatch := p.Size()
	p.Append([]byte{0x00, 0x00})
	p.Op(vm.JUMP)

	_, cumulativePC := p.Jumpdest()
	p.ReturnData(cumulativeData)

	_, reservesPC := p.Jumpdest()
	p.ReturnData(reservesData)

	_, dispatchPC := p.Jumpdest()

	p.Push(0)
	p.Op(vm.CALLDATALOAD)
	p.Push(224)
	p.Op(vm.SHR)

	p.Op(vm.DUP1)
	p.Push(cumulativeSelector)
	p.Op(vm.EQ)
	p.Push(cumulativePC)
	p.Op(vm.JUMPI)

	p.Push(reservesSelector)
	p.Op(vm.EQ)
	p.Push(reservesPC)
	p.Op(vm.JUMPI)

	p.Push(0)
	p.Push(0)
	p.Op(vm.REVERT)

	code := append([]byte(nil), p.Bytes()...)

	if dispatchPC > math.MaxUint16 {
		panic("test pair dispatcher exceeds PUSH2")
	}

	code[dispatchPatch] = byte(dispatchPC >> 8)
	code[dispatchPatch+1] = byte(dispatchPC)

	return code
}

func rabbitVRFTestAssertOracleEmpty(
	t *testing.T,
	sdb *state.StateDB,
	timestamp uint64,
) {
	t.Helper()

	if got := rabbitVRFTestUintView(
		t,
		sdb,
		timestamp,
		"observationCount()",
	); got != 0 {
		t.Fatalf("observation count = %d, want 0", got)
	}

	initialized, observedAt, cumulative :=
		rabbitVRFTestLatest(t, sdb, timestamp)

	if initialized || observedAt != 0 || cumulative.Sign() != 0 {
		t.Fatalf(
			"oracle mutated: initialized=%v time=%d cumulative=%s",
			initialized,
			observedAt,
			cumulative,
		)
	}
}

func TestRabbitVRFPriceObservationRejectsMalformedPairData(
	t *testing.T,
) {
	validCumulative := make([]byte, 32)

	validReserves := make([]byte, 96)
	validReserves[31] = 1
	validReserves[63] = 1
	validReserves[95] = 100

	badReserveWidth := append([]byte(nil), validReserves...)
	badReserveWidth[0] = 1

	badTimestampWidth := append([]byte(nil), validReserves...)
	badTimestampWidth[64] = 1

	tests := []struct {
		name           string
		cumulativeData []byte
		reservesData   []byte
	}{
		{
			name:           "short cumulative",
			cumulativeData: make([]byte, 31),
			reservesData:   validReserves,
		},
		{
			name:           "long cumulative",
			cumulativeData: make([]byte, 33),
			reservesData:   validReserves,
		},
		{
			name:           "short reserves",
			cumulativeData: validCumulative,
			reservesData:   make([]byte, 95),
		},
		{
			name:           "long reserves",
			cumulativeData: validCumulative,
			reservesData:   make([]byte, 97),
		},
		{
			name:           "reserve exceeds uint112",
			cumulativeData: validCumulative,
			reservesData:   badReserveWidth,
		},
		{
			name:           "timestamp exceeds uint32",
			cumulativeData: validCumulative,
			reservesData:   badTimestampWidth,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sdb := mkState(nil)
			misc.ApplyRabbitVRFCoordinatorV1(sdb)

			sdb.CreateAccount(rabbitVRFTestPairAddress)
			sdb.SetCode(
				rabbitVRFTestPairAddress,
				rabbitVRFTestPairReturnCode(
					tc.cumulativeData,
					tc.reservesData,
				),
				tracing.CodeChangeUnspecified,
			)

			rabbitVRFTestObserve(t, sdb, 400)
			rabbitVRFTestAssertOracleEmpty(t, sdb, 400)
		})
	}
}

func TestRabbitVRFPriceObservationRejectsPairCallFailure(
	t *testing.T,
) {
	sdb := mkState(nil)
	misc.ApplyRabbitVRFCoordinatorV1(sdb)

	sdb.CreateAccount(rabbitVRFTestPairAddress)

	revertCode := program.New().
		Push(0).
		Push(0).
		Op(vm.REVERT).
		Bytes()

	sdb.SetCode(
		rabbitVRFTestPairAddress,
		revertCode,
		tracing.CodeChangeUnspecified,
	)

	rabbitVRFTestObserve(t, sdb, 400)
	rabbitVRFTestAssertOracleEmpty(t, sdb, 400)
}

func TestRabbitVRFPriceObservationUint32Wrap(
	t *testing.T,
) {
	sdb := mkState(nil)
	misc.ApplyRabbitVRFCoordinatorV1(sdb)

	stored := big.NewInt(100)
	timestampLast := uint32(math.MaxUint32 - 100)
	current := uint64(math.MaxUint32) + 200

	rabbitVRFTestSetPair(
		sdb,
		stored,
		1,
		1,
		timestampLast,
	)

	rabbitVRFTestObserve(t, sdb, current)

	initialized, observedAt, cumulative :=
		rabbitVRFTestLatest(t, sdb, current)

	if !initialized {
		t.Fatal("wrapped observation not initialized")
	}
	if observedAt != current {
		t.Fatalf(
			"wrapped observedAt = %d, want %d",
			observedAt,
			current,
		)
	}

	// uint32(current) - timestampLast wraps to exactly 300.
	want := new(big.Int).Lsh(big.NewInt(1), 112)
	want.Mul(want, big.NewInt(300))
	want.Add(want, stored)

	if cumulative.Cmp(want) != 0 {
		t.Fatalf(
			"wrapped cumulative = %s, want %s",
			cumulative,
			want,
		)
	}
}

func TestRabbitVRFPriceObservationRingWrap(
	t *testing.T,
) {
	sdb := mkState(nil)
	misc.ApplyRabbitVRFCoordinatorV1(sdb)

	rabbitVRFTestSetPair(
		sdb,
		big.NewInt(0),
		1,
		1,
		0,
	)

	const first = uint64(1000)

	for i := uint64(0); i < 17; i++ {
		rabbitVRFTestObserve(
			t,
			sdb,
			first+i*300,
		)
	}

	if got := rabbitVRFTestUintView(
		t,
		sdb,
		first+16*300,
		"observationCount()",
	); got != 16 {
		t.Fatalf("ring count = %d, want 16", got)
	}

	if got := rabbitVRFTestUintView(
		t,
		sdb,
		first+16*300,
		"observationIndex()",
	); got != 0 {
		t.Fatalf("ring index after 17 observations = %d, want 0", got)
	}

	observedAt, cumulative :=
		rabbitVRFTestObservationAt(
			t,
			sdb,
			first+16*300,
			0,
		)

	wantTime := first + 16*300

	if observedAt != wantTime {
		t.Fatalf(
			"slot 0 time after wrap = %d, want %d",
			observedAt,
			wantTime,
		)
	}

	wantCumulative := new(big.Int).Lsh(
		new(big.Int).SetUint64(wantTime),
		112,
	)

	if cumulative.Cmp(wantCumulative) != 0 {
		t.Fatalf(
			"slot 0 cumulative after wrap = %s, want %s",
			cumulative,
			wantCumulative,
		)
	}
}

func TestRabbitVRFPriceObservationPreExecutionActivation(t *testing.T) {
	config := rabbitVRFTestConfig(2)
	sdb := mkState(nil)

	rabbitVRFTestSetPair(
		sdb,
		big.NewInt(0),
		1,
		1,
		0,
	)

	coordinator := params.RabbitVRFCoordinatorV1Address
	metadataSlot := common.Hash{}

	parent0 := &types.Header{
		Number: big.NewInt(0),
		Time:   0,
	}

	PreExecution(
		context.Background(),
		nil,
		parent0,
		config,
		rabbitVRFTestEVM(sdb, 100),
		big.NewInt(1),
		100,
	)

	if len(sdb.GetCode(coordinator)) != 0 {
		t.Fatal("coordinator installed before VRF activation")
	}

	parent1 := &types.Header{
		Number: big.NewInt(1),
		Time:   100,
	}

	PreExecution(
		context.Background(),
		nil,
		parent1,
		config,
		rabbitVRFTestEVM(sdb, 400),
		big.NewInt(2),
		400,
	)

	if len(sdb.GetCode(coordinator)) == 0 {
		t.Fatal("coordinator not installed at VRF activation")
	}

	if got := sdb.GetState(
		coordinator,
		metadataSlot,
	); got != common.BigToHash(big.NewInt(1)) {
		t.Fatalf(
			"activation metadata = %s, want 0x1",
			got,
		)
	}

	parent2 := &types.Header{
		Number: big.NewInt(2),
		Time:   400,
	}

	PreExecution(
		context.Background(),
		nil,
		parent2,
		config,
		rabbitVRFTestEVM(sdb, 699),
		big.NewInt(3),
		699,
	)

	if got := sdb.GetState(
		coordinator,
		metadataSlot,
	); got != common.BigToHash(big.NewInt(1)) {
		t.Fatalf(
			"+299s metadata = %s, want 0x1",
			got,
		)
	}

	parent3 := &types.Header{
		Number: big.NewInt(3),
		Time:   699,
	}

	PreExecution(
		context.Background(),
		nil,
		parent3,
		config,
		rabbitVRFTestEVM(sdb, 700),
		big.NewInt(4),
		700,
	)

	wantMetadata := new(big.Int).SetUint64(2 | (1 << 8))

	if got := sdb.GetState(
		coordinator,
		metadataSlot,
	); got != common.BigToHash(wantMetadata) {
		t.Fatalf(
			"+300s metadata = %s, want %s",
			got,
			common.BigToHash(wantMetadata),
		)
	}
}
