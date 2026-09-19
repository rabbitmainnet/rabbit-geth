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
	"github.com/holiman/uint256"
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

func rabbitVRFTestQuoteProtocolFee(
	t *testing.T,
	sdb *state.StateDB,
	timestamp uint64,
) (*big.Int, error) {
	t.Helper()

	selector := crypto.Keccak256(
		[]byte("quoteProtocolFee()"),
	)[:4]

	ret, _, err := rabbitVRFTestEVM(
		sdb,
		timestamp,
	).Call(
		common.Address{0x01},
		params.RabbitVRFCoordinatorV1Address,
		selector,
		vm.NewGasBudget(30_000_000, 30_000_000),
		common.U2560,
	)

	if err != nil {
		return nil, err
	}

	if len(ret) != 32 {
		t.Fatalf(
			"quoteProtocolFee returned %d bytes, want 32",
			len(ret),
		)
	}

	return new(big.Int).SetBytes(ret), nil
}

func TestRabbitVRFProtocolFeeWarmupAndStale(
	t *testing.T,
) {
	sdb := mkState(nil)
	misc.ApplyRabbitVRFCoordinatorV1(sdb)

	rabbitVRFTestSetPair(
		sdb,
		big.NewInt(0),
		1,
		2,
		0,
	)

	for _, timestamp := range []uint64{
		1000,
		1300,
		1600,
		1900,
		2200,
		2500,
	} {
		rabbitVRFTestObserve(
			t,
			sdb,
			timestamp,
		)
	}

	if fee, err := rabbitVRFTestQuoteProtocolFee(
		t,
		sdb,
		2500,
	); err == nil {
		t.Fatalf(
			"warm-up quote unexpectedly succeeded: %s",
			fee,
		)
	}

	rabbitVRFTestObserve(
		t,
		sdb,
		2800,
	)

	fee, err := rabbitVRFTestQuoteProtocolFee(
		t,
		sdb,
		2800,
	)
	if err != nil {
		t.Fatalf(
			"exact 1800-second quote failed: %v",
			err,
		)
	}

	if fee.Cmp(big.NewInt(20_000)) != 0 {
		t.Fatalf(
			"exact-window fee = %s, want 20000",
			fee,
		)
	}

	fee, err = rabbitVRFTestQuoteProtocolFee(
		t,
		sdb,
		4600,
	)
	if err != nil {
		t.Fatalf(
			"3600-second baseline-age boundary failed: %v",
			err,
		)
	}

	if fee.Cmp(big.NewInt(20_000)) != 0 {
		t.Fatalf(
			"boundary fee = %s, want 20000",
			fee,
		)
	}

	if fee, err := rabbitVRFTestQuoteProtocolFee(
		t,
		sdb,
		4601,
	); err == nil {
		t.Fatalf(
			"stale baseline quote unexpectedly succeeded: %s",
			fee,
		)
	}
}

func TestRabbitVRFProtocolFeeRecoveryNeedsNewWindow(
	t *testing.T,
) {
	sdb := mkState(nil)
	misc.ApplyRabbitVRFCoordinatorV1(sdb)

	rabbitVRFTestSetPair(
		sdb,
		big.NewInt(0),
		1,
		2,
		0,
	)

	for _, timestamp := range []uint64{
		1000,
		1300,
		1600,
		1900,
		2200,
		2500,
		2800,
	} {
		rabbitVRFTestObserve(
			t,
			sdb,
			timestamp,
		)
	}

	fee, err := rabbitVRFTestQuoteProtocolFee(
		t,
		sdb,
		2800,
	)
	if err != nil {
		t.Fatalf(
			"pre-outage quote failed: %v",
			err,
		)
	}

	if fee.Cmp(big.NewInt(20_000)) != 0 {
		t.Fatalf(
			"pre-outage fee = %s, want 20000",
			fee,
		)
	}

	if fee, err := rabbitVRFTestQuoteProtocolFee(
		t,
		sdb,
		6501,
	); err == nil {
		t.Fatalf(
			"stale latest quote unexpectedly succeeded: %s",
			fee,
		)
	}

	rabbitVRFTestObserve(
		t,
		sdb,
		6800,
	)

	if fee, err := rabbitVRFTestQuoteProtocolFee(
		t,
		sdb,
		6800,
	); err == nil {
		t.Fatalf(
			"single fresh observation restored billing: %s",
			fee,
		)
	}

	for _, timestamp := range []uint64{
		7100,
		7400,
		7700,
		8000,
		8300,
	} {
		rabbitVRFTestObserve(
			t,
			sdb,
			timestamp,
		)
	}

	if fee, err := rabbitVRFTestQuoteProtocolFee(
		t,
		sdb,
		8300,
	); err == nil {
		t.Fatalf(
			"1500-second recovery window unexpectedly succeeded: %s",
			fee,
		)
	}

	rabbitVRFTestObserve(
		t,
		sdb,
		8600,
	)

	fee, err = rabbitVRFTestQuoteProtocolFee(
		t,
		sdb,
		8600,
	)
	if err != nil {
		t.Fatalf(
			"recovered 1800-second window failed: %v",
			err,
		)
	}

	if fee.Cmp(big.NewInt(20_000)) != 0 {
		t.Fatalf(
			"recovered fee = %s, want 20000",
			fee,
		)
	}
}

func rabbitVRFTestObserveStoredCumulative(
	t *testing.T,
	sdb *state.StateDB,
	timestamp uint64,
	cumulative *big.Int,
) {
	t.Helper()

	if timestamp > math.MaxUint32 {
		t.Fatalf(
			"test timestamp %d exceeds uint32",
			timestamp,
		)
	}

	rabbitVRFTestSetPair(
		sdb,
		cumulative,
		1,
		1,
		uint32(timestamp),
	)

	rabbitVRFTestObserve(
		t,
		sdb,
		timestamp,
	)
}

func TestRabbitVRFProtocolFeeUsesNewestValidBaseline(
	t *testing.T,
) {
	sdb := mkState(nil)
	misc.ApplyRabbitVRFCoordinatorV1(sdb)

	q112 := new(big.Int).Lsh(
		big.NewInt(1),
		112,
	)

	entries := []struct {
		timestamp  uint64
		multiplier int64
	}{
		{1000, 0},
		{1300, 10},
		{1600, 16},
		{1900, 22},
		{2200, 28},
		{2500, 34},
		{2800, 40},
		{3100, 46},
	}

	for _, entry := range entries {
		cumulative := new(big.Int).Mul(
			new(big.Int).Set(q112),
			big.NewInt(entry.multiplier),
		)

		rabbitVRFTestObserveStoredCumulative(
			t,
			sdb,
			entry.timestamp,
			cumulative,
		)
	}

	fee, err := rabbitVRFTestQuoteProtocolFee(
		t,
		sdb,
		3100,
	)
	if err != nil {
		t.Fatalf(
			"newest-baseline quote failed: %v",
			err,
		)
	}

	if fee.Cmp(big.NewInt(200)) != 0 {
		t.Fatalf(
			"newest-baseline fee = %s, want 200",
			fee,
		)
	}
}

func TestRabbitVRFProtocolFeeRoundsUp(
	t *testing.T,
) {
	sdb := mkState(nil)
	misc.ApplyRabbitVRFCoordinatorV1(sdb)

	rabbitVRFTestSetPair(
		sdb,
		big.NewInt(0),
		3,
		1,
		0,
	)

	rabbitVRFTestObserve(
		t,
		sdb,
		1000,
	)

	rabbitVRFTestObserve(
		t,
		sdb,
		2800,
	)

	fee, err := rabbitVRFTestQuoteProtocolFee(
		t,
		sdb,
		2800,
	)
	if err != nil {
		t.Fatalf(
			"rounding quote failed: %v",
			err,
		)
	}

	q112 := new(big.Int).Lsh(
		big.NewInt(1),
		112,
	)

	priceX112 := new(big.Int).Div(
		new(big.Int).Set(q112),
		big.NewInt(3),
	)

	numerator := new(big.Int).Mul(
		big.NewInt(10_000),
		priceX112,
	)

	want := new(big.Int).Add(
		numerator,
		new(big.Int).Sub(
			new(big.Int).Set(q112),
			big.NewInt(1),
		),
	)
	want.Div(want, q112)

	if fee.Cmp(want) != 0 {
		t.Fatalf(
			"rounded fee = %s, want %s",
			fee,
			want,
		)
	}

	if fee.Cmp(big.NewInt(3334)) != 0 {
		t.Fatalf(
			"rounded fee = %s, want 3334",
			fee,
		)
	}
}

func TestRabbitVRFProtocolFeeUint256WrapAnd512BitMulDiv(
	t *testing.T,
) {
	sdb := mkState(nil)
	misc.ApplyRabbitVRFCoordinatorV1(sdb)

	rabbitVRFTestObserveStoredCumulative(
		t,
		sdb,
		1000,
		big.NewInt(1),
	)

	rabbitVRFTestObserveStoredCumulative(
		t,
		sdb,
		2800,
		big.NewInt(0),
	)

	fee, err := rabbitVRFTestQuoteProtocolFee(
		t,
		sdb,
		2800,
	)
	if err != nil {
		t.Fatalf(
			"512-bit wrapped quote failed: %v",
			err,
		)
	}

	two256 := new(big.Int).Lsh(
		big.NewInt(1),
		256,
	)

	delta := new(big.Int).Sub(
		two256,
		big.NewInt(1),
	)

	numerator := new(big.Int).Mul(
		big.NewInt(10_000),
		delta,
	)

	q112 := new(big.Int).Lsh(
		big.NewInt(1),
		112,
	)

	denominator := new(big.Int).Mul(
		big.NewInt(1800),
		q112,
	)

	want := new(big.Int).Add(
		numerator,
		new(big.Int).Sub(
			new(big.Int).Set(denominator),
			big.NewInt(1),
		),
	)

	want.Div(
		want,
		denominator,
	)

	if numerator.BitLen() <= 256 {
		t.Fatalf(
			"test did not exercise 512-bit multiplication: bitlen=%d",
			numerator.BitLen(),
		)
	}

	if fee.Cmp(want) != 0 {
		t.Fatalf(
			"512-bit wrapped fee = %s, want %s",
			fee,
			want,
		)
	}
}

func TestRabbitVRFRequestViewsEmptyState(
	t *testing.T,
) {
	sdb := mkState(nil)
	misc.ApplyRabbitVRFCoordinatorV1(sdb)

	requester := common.HexToAddress(
		"0x1111111111111111111111111111111111111111",
	)

	requesterArg := make([]byte, 32)
	copy(
		requesterArg[12:],
		requester.Bytes(),
	)

	nonceRet := rabbitVRFTestCall(
		t,
		sdb,
		1000,
		common.Address{0x01},
		"nextRequestNonce(address)",
		requesterArg,
	)

	if len(nonceRet) != 32 {
		t.Fatalf(
			"nextRequestNonce returned %d bytes, want 32",
			len(nonceRet),
		)
	}

	if got := new(big.Int).SetBytes(nonceRet); got.Sign() != 0 {
		t.Fatalf(
			"initial nextRequestNonce = %s, want 0",
			got,
		)
	}

	requestID := crypto.Keccak256Hash(
		[]byte("rabbit-vrf-missing-request"),
	)

	requestRet := rabbitVRFTestCall(
		t,
		sdb,
		1000,
		common.Address{0x01},
		"getRequest(bytes32)",
		requestID[:],
	)

	const wantRequestBytes = 11 * 32

	if len(requestRet) != wantRequestBytes {
		t.Fatalf(
			"getRequest returned %d bytes, want %d",
			len(requestRet),
			wantRequestBytes,
		)
	}

	for index, value := range requestRet {
		if value != 0 {
			t.Fatalf(
				"empty getRequest byte %d = 0x%02x, want 0x00",
				index,
				value,
			)
		}
	}
}

func rabbitVRFTestQuoteRequestFee(
	t *testing.T,
	sdb *state.StateDB,
	timestamp uint64,
	callbackGasLimit uint32,
) (*big.Int, []byte, error) {
	t.Helper()

	argument := make([]byte, 32)
	new(big.Int).
		SetUint64(uint64(callbackGasLimit)).
		FillBytes(argument)

	selector := crypto.Keccak256(
		[]byte("quoteRequestFee(uint32)"),
	)[:4]

	input := make([]byte, 36)
	copy(input[:4], selector)
	copy(input[4:], argument)

	ret, _, err := rabbitVRFTestEVM(
		sdb,
		timestamp,
	).Call(
		common.Address{0x01},
		params.RabbitVRFCoordinatorV1Address,
		input,
		vm.NewGasBudget(30_000_000, 30_000_000),
		common.U2560,
	)

	if err != nil {
		return nil, ret, err
	}

	if len(ret) != 32 {
		t.Fatalf(
			"quoteRequestFee returned %d bytes, want 32",
			len(ret),
		)
	}

	return new(big.Int).SetBytes(ret), ret, nil
}

func TestRabbitVRFRequestFeeQuote(
	t *testing.T,
) {
	sdb := mkState(nil)
	misc.ApplyRabbitVRFCoordinatorV1(sdb)

	rabbitVRFTestSetPair(
		sdb,
		big.NewInt(0),
		1,
		2,
		0,
	)

	for _, timestamp := range []uint64{
		1000,
		1300,
		1600,
		1900,
		2200,
		2500,
		2800,
	} {
		rabbitVRFTestObserve(
			t,
			sdb,
			timestamp,
		)
	}

	protocolFee, err := rabbitVRFTestQuoteProtocolFee(
		t,
		sdb,
		2800,
	)
	if err != nil {
		t.Fatalf(
			"quoteProtocolFee failed: %v",
			err,
		)
	}

	requestFee, _, err := rabbitVRFTestQuoteRequestFee(
		t,
		sdb,
		2800,
		0,
	)
	if err != nil {
		t.Fatalf(
			"quoteRequestFee(0) failed: %v",
			err,
		)
	}

	if requestFee.Cmp(protocolFee) != 0 {
		t.Fatalf(
			"request fee = %s, protocol fee = %s",
			requestFee,
			protocolFee,
		)
	}

	if requestFee.Cmp(big.NewInt(20_000)) != 0 {
		t.Fatalf(
			"request fee = %s, want 20000",
			requestFee,
		)
	}

	fee, revertData, err := rabbitVRFTestQuoteRequestFee(
		t,
		sdb,
		2800,
		1,
	)

	if err == nil {
		t.Fatalf(
			"quoteRequestFee(1) unexpectedly succeeded: %s",
			fee,
		)
	}

	wantSelector := crypto.Keccak256(
		[]byte("CallbackFundingUnavailable()"),
	)[:4]

	if string(revertData) != string(wantSelector) {
		t.Fatalf(
			"callback rejection data = %x, want %x",
			revertData,
			wantSelector,
		)
	}
}

func rabbitVRFTestRequestRandomness(
	sdb *state.StateDB,
	timestamp uint64,
	caller common.Address,
	callbackGasLimit uint32,
	appDataHash common.Hash,
	value uint64,
) ([]byte, error) {
	argument := make([]byte, 64)

	new(big.Int).
		SetUint64(uint64(callbackGasLimit)).
		FillBytes(argument[:32])

	copy(
		argument[32:64],
		appDataHash[:],
	)

	selector := crypto.Keccak256(
		[]byte("requestRandomness(uint32,bytes32)"),
	)[:4]

	input := make([]byte, 4+len(argument))
	copy(input[:4], selector)
	copy(input[4:], argument)

	ret, _, err := rabbitVRFTestEVM(
		sdb,
		timestamp,
	).Call(
		caller,
		params.RabbitVRFCoordinatorV1Address,
		input,
		vm.NewGasBudget(30_000_000, 30_000_000),
		uint256.NewInt(value),
	)

	return ret, err
}

func rabbitVRFTestPrepareRequestPricing(
	t *testing.T,
	sdb *state.StateDB,
) {
	t.Helper()

	rabbitVRFTestSetPair(
		sdb,
		big.NewInt(0),
		1,
		2,
		0,
	)

	for _, timestamp := range []uint64{
		1000,
		1300,
		1600,
		1900,
		2200,
		2500,
		2800,
	} {
		rabbitVRFTestObserve(
			t,
			sdb,
			timestamp,
		)
	}
}

func TestRabbitVRFRequestRandomnessStateAndReverts(
	t *testing.T,
) {
	caller := common.HexToAddress(
		"0x1111111111111111111111111111111111111111",
	)

	appDataHash := crypto.Keccak256Hash(
		[]byte("rabbit-vrf-request-test"),
	)

	t.Run("callback_rejected_without_state", func(t *testing.T) {
		sdb := mkState(nil)
		misc.ApplyRabbitVRFCoordinatorV1(sdb)

		sdb.SetBalance(
			caller,
			uint256.NewInt(1_000_000),
			tracing.BalanceChangeUnspecified,
		)

		beforeCaller := sdb.GetBalance(caller).Uint64()
		beforeCoordinator := sdb.GetBalance(
			params.RabbitVRFCoordinatorV1Address,
		).Uint64()

		ret, err := rabbitVRFTestRequestRandomness(
			sdb,
			1000,
			caller,
			1,
			appDataHash,
			0,
		)

		if err == nil {
			t.Fatal(
				"callback request unexpectedly succeeded",
			)
		}

		wantSelector := crypto.Keccak256(
			[]byte("CallbackFundingUnavailable()"),
		)[:4]

		if string(ret) != string(wantSelector) {
			t.Fatalf(
				"callback revert data = %x, want %x",
				ret,
				wantSelector,
			)
		}

		nonceArg := make([]byte, 32)
		copy(
			nonceArg[12:],
			caller.Bytes(),
		)

		nonceRet := rabbitVRFTestCall(
			t,
			sdb,
			1000,
			common.Address{0x01},
			"nextRequestNonce(address)",
			nonceArg,
		)

		if new(big.Int).SetBytes(nonceRet).Sign() != 0 {
			t.Fatal(
				"callback revert changed requester nonce",
			)
		}

		if got := sdb.GetBalance(caller).Uint64(); got != beforeCaller {
			t.Fatalf(
				"callback revert caller balance = %d, want %d",
				got,
				beforeCaller,
			)
		}

		if got := sdb.GetBalance(
			params.RabbitVRFCoordinatorV1Address,
		).Uint64(); got != beforeCoordinator {
			t.Fatalf(
				"callback revert coordinator balance = %d, want %d",
				got,
				beforeCoordinator,
			)
		}
	})

	t.Run("wrong_fee_reverts_without_state", func(t *testing.T) {
		sdb := mkState(nil)
		misc.ApplyRabbitVRFCoordinatorV1(sdb)

		rabbitVRFTestPrepareRequestPricing(
			t,
			sdb,
		)

		sdb.SetBalance(
			caller,
			uint256.NewInt(1_000_000),
			tracing.BalanceChangeUnspecified,
		)

		beforeCaller := sdb.GetBalance(caller).Uint64()

		ret, err := rabbitVRFTestRequestRandomness(
			sdb,
			2800,
			caller,
			0,
			appDataHash,
			19_999,
		)

		if err == nil {
			t.Fatal(
				"wrong-fee request unexpectedly succeeded",
			)
		}

		wantSelector := crypto.Keccak256(
			[]byte("IncorrectRequestFee(uint256,uint256)"),
		)[:4]

		if len(ret) < 4 ||
			string(ret[:4]) != string(wantSelector) {
			t.Fatalf(
				"wrong-fee revert data = %x, want selector %x",
				ret,
				wantSelector,
			)
		}

		nonceArg := make([]byte, 32)
		copy(
			nonceArg[12:],
			caller.Bytes(),
		)

		nonceRet := rabbitVRFTestCall(
			t,
			sdb,
			2800,
			common.Address{0x01},
			"nextRequestNonce(address)",
			nonceArg,
		)

		if new(big.Int).SetBytes(nonceRet).Sign() != 0 {
			t.Fatal(
				"wrong-fee revert changed requester nonce",
			)
		}

		if got := sdb.GetBalance(caller).Uint64(); got != beforeCaller {
			t.Fatalf(
				"wrong-fee revert caller balance = %d, want %d",
				got,
				beforeCaller,
			)
		}

		if got := sdb.GetBalance(
			params.RabbitVRFCoordinatorV1Address,
		).Uint64(); got != 0 {
			t.Fatalf(
				"wrong-fee revert coordinator balance = %d, want 0",
				got,
			)
		}
	})

	t.Run("valid_request", func(t *testing.T) {
		sdb := mkState(nil)
		misc.ApplyRabbitVRFCoordinatorV1(sdb)

		rabbitVRFTestPrepareRequestPricing(
			t,
			sdb,
		)

		sdb.SetBalance(
			caller,
			uint256.NewInt(1_000_000),
			tracing.BalanceChangeUnspecified,
		)

		ret, err := rabbitVRFTestRequestRandomness(
			sdb,
			2800,
			caller,
			0,
			appDataHash,
			20_000,
		)
		if err != nil {
			t.Fatalf(
				"valid request failed: %v",
				err,
			)
		}

		if len(ret) != 32 {
			t.Fatalf(
				"request returned %d bytes, want 32",
				len(ret),
			)
		}

		encoded := make([]byte, 7*32)

		requestDomain := crypto.Keccak256Hash(
			[]byte("RABBIT_VRF_REQUEST_V1"),
		)
		copy(
			encoded[0:32],
			requestDomain[:],
		)

		params.TestChainConfig.ChainID.FillBytes(
			encoded[32:64],
		)

		copy(
			encoded[64+12:96],
			params.RabbitVRFCoordinatorV1Address.Bytes(),
		)

		copy(
			encoded[96+12:128],
			caller.Bytes(),
		)

		copy(
			encoded[192:224],
			appDataHash[:],
		)

		wantRequestID := crypto.Keccak256Hash(
			encoded,
		)

		gotRequestID := common.BytesToHash(ret)

		if gotRequestID != wantRequestID {
			t.Fatalf(
				"requestId = %s, want %s",
				gotRequestID,
				wantRequestID,
			)
		}

		nonceArg := make([]byte, 32)
		copy(
			nonceArg[12:],
			caller.Bytes(),
		)

		nonceRet := rabbitVRFTestCall(
			t,
			sdb,
			2800,
			common.Address{0x01},
			"nextRequestNonce(address)",
			nonceArg,
		)

		if got := new(big.Int).SetBytes(nonceRet).Uint64(); got != 1 {
			t.Fatalf(
				"next requester nonce = %d, want 1",
				got,
			)
		}

		requestRet := rabbitVRFTestCall(
			t,
			sdb,
			2800,
			common.Address{0x01},
			"getRequest(bytes32)",
			wantRequestID[:],
		)

		if len(requestRet) != 11*32 {
			t.Fatalf(
				"getRequest returned %d bytes, want %d",
				len(requestRet),
				11*32,
			)
		}

		word := func(index int) []byte {
			start := index * 32
			return requestRet[start : start+32]
		}

		if got := common.BytesToAddress(word(0)[12:]); got != caller {
			t.Fatalf(
				"stored requester = %s, want %s",
				got,
				caller,
			)
		}

		if got := new(big.Int).SetBytes(word(1)).Uint64(); got != 0 {
			t.Fatalf(
				"stored requester nonce = %d, want 0",
				got,
			)
		}

		if got := new(big.Int).SetBytes(word(2)).Uint64(); got != 1 {
			t.Fatalf(
				"stored request block = %d, want 1",
				got,
			)
		}

		for _, index := range []int{3, 4, 5} {
			if new(big.Int).SetBytes(word(index)).Sign() != 0 {
				t.Fatalf(
					"stored word %d is non-zero: %x",
					index,
					word(index),
				)
			}
		}

		if got := new(big.Int).SetBytes(word(6)).Uint64(); got != 20_000 {
			t.Fatalf(
				"stored feePaid = %d, want 20000",
				got,
			)
		}

		if got := common.BytesToHash(word(7)); got != appDataHash {
			t.Fatalf(
				"stored appDataHash = %s, want %s",
				got,
				appDataHash,
			)
		}

		if got := common.BytesToHash(word(8)); got != (common.Hash{}) {
			t.Fatalf(
				"stored randomness = %s, want zero",
				got,
			)
		}

		if got := common.BytesToHash(word(9)); got != (common.Hash{}) {
			t.Fatalf(
				"stored proofHash = %s, want zero",
				got,
			)
		}

		if got := new(big.Int).SetBytes(word(10)).Uint64(); got != 1 {
			t.Fatalf(
				"stored status = %d, want PENDING(1)",
				got,
			)
		}

		if got := sdb.GetBalance(caller).Uint64(); got != 980_000 {
			t.Fatalf(
				"caller balance = %d, want 980000",
				got,
			)
		}

		if got := sdb.GetBalance(
			params.RabbitVRFCoordinatorV1Address,
		).Uint64(); got != 20_000 {
			t.Fatalf(
				"coordinator balance = %d, want 20000",
				got,
			)
		}
	})
}

func TestRabbitVRFRequestRandomnessConsecutiveAndEvents(
	t *testing.T,
) {
	sdb := mkState(nil)
	misc.ApplyRabbitVRFCoordinatorV1(sdb)

	rabbitVRFTestPrepareRequestPricing(
		t,
		sdb,
	)

	caller := common.HexToAddress(
		"0x2222222222222222222222222222222222222222",
	)

	appDataHash := crypto.Keccak256Hash(
		[]byte("rabbit-vrf-consecutive-request"),
	)

	sdb.SetBalance(
		caller,
		uint256.NewInt(1_000_000),
		tracing.BalanceChangeUnspecified,
	)

	expectedRequestID := func(
		requesterNonce uint64,
	) common.Hash {
		encoded := make([]byte, 7*32)

		requestDomain := crypto.Keccak256Hash(
			[]byte("RABBIT_VRF_REQUEST_V1"),
		)

		copy(
			encoded[0:32],
			requestDomain[:],
		)

		params.TestChainConfig.ChainID.FillBytes(
			encoded[32:64],
		)

		copy(
			encoded[64+12:96],
			params.RabbitVRFCoordinatorV1Address.Bytes(),
		)

		copy(
			encoded[96+12:128],
			caller.Bytes(),
		)

		new(big.Int).
			SetUint64(requesterNonce).
			FillBytes(encoded[128:160])

		copy(
			encoded[192:224],
			appDataHash[:],
		)

		return crypto.Keccak256Hash(
			encoded,
		)
	}

	firstRet, err := rabbitVRFTestRequestRandomness(
		sdb,
		2800,
		caller,
		0,
		appDataHash,
		20_000,
	)
	if err != nil {
		t.Fatalf(
			"first request failed: %v",
			err,
		)
	}

	secondRet, err := rabbitVRFTestRequestRandomness(
		sdb,
		2800,
		caller,
		0,
		appDataHash,
		20_000,
	)
	if err != nil {
		t.Fatalf(
			"second request failed: %v",
			err,
		)
	}

	firstID := common.BytesToHash(firstRet)
	secondID := common.BytesToHash(secondRet)

	wantFirstID := expectedRequestID(0)
	wantSecondID := expectedRequestID(1)

	if firstID != wantFirstID {
		t.Fatalf(
			"first requestId = %s, want %s",
			firstID,
			wantFirstID,
		)
	}

	if secondID != wantSecondID {
		t.Fatalf(
			"second requestId = %s, want %s",
			secondID,
			wantSecondID,
		)
	}

	if firstID == secondID {
		t.Fatal(
			"consecutive requests produced identical requestIds",
		)
	}

	nonceArg := make([]byte, 32)
	copy(
		nonceArg[12:],
		caller.Bytes(),
	)

	nonceRet := rabbitVRFTestCall(
		t,
		sdb,
		2800,
		common.Address{0x01},
		"nextRequestNonce(address)",
		nonceArg,
	)

	if got := new(big.Int).SetBytes(nonceRet).Uint64(); got != 2 {
		t.Fatalf(
			"next requester nonce = %d, want 2",
			got,
		)
	}

	for index, requestID := range []common.Hash{
		firstID,
		secondID,
	} {
		requestRet := rabbitVRFTestCall(
			t,
			sdb,
			2800,
			common.Address{0x01},
			"getRequest(bytes32)",
			requestID[:],
		)

		if len(requestRet) != 11*32 {
			t.Fatalf(
				"request %d returned %d bytes, want %d",
				index,
				len(requestRet),
				11*32,
			)
		}

		word := func(wordIndex int) []byte {
			start := wordIndex * 32
			return requestRet[start : start+32]
		}

		if got := new(big.Int).SetBytes(word(1)).Uint64(); got != uint64(index) {
			t.Fatalf(
				"request %d stored nonce = %d, want %d",
				index,
				got,
				index,
			)
		}

		if got := new(big.Int).SetBytes(word(6)).Uint64(); got != 20_000 {
			t.Fatalf(
				"request %d feePaid = %d, want 20000",
				index,
				got,
			)
		}

		if got := new(big.Int).SetBytes(word(10)).Uint64(); got != 1 {
			t.Fatalf(
				"request %d status = %d, want PENDING(1)",
				index,
				got,
			)
		}
	}

	logs := sdb.Logs()

	if len(logs) != 2 {
		t.Fatalf(
			"request logs = %d, want 2",
			len(logs),
		)
	}

	eventTopic := crypto.Keccak256Hash(
		[]byte(
			"RandomnessRequested(bytes32,address,uint64,uint32,bytes32,uint256)",
		),
	)

	requesterTopic := common.BytesToHash(
		common.LeftPadBytes(
			caller.Bytes(),
			32,
		),
	)

	for index, log := range logs {
		requestID := []common.Hash{
			firstID,
			secondID,
		}[index]

		if log.Address != params.RabbitVRFCoordinatorV1Address {
			t.Fatalf(
				"log %d address = %s, want %s",
				index,
				log.Address,
				params.RabbitVRFCoordinatorV1Address,
			)
		}

		if len(log.Topics) != 4 {
			t.Fatalf(
				"log %d topics = %d, want 4",
				index,
				len(log.Topics),
			)
		}

		if log.Topics[0] != eventTopic {
			t.Fatalf(
				"log %d topic0 = %s, want %s",
				index,
				log.Topics[0],
				eventTopic,
			)
		}

		if log.Topics[1] != requestID {
			t.Fatalf(
				"log %d requestId topic = %s, want %s",
				index,
				log.Topics[1],
				requestID,
			)
		}

		if log.Topics[2] != requesterTopic {
			t.Fatalf(
				"log %d requester topic = %s, want %s",
				index,
				log.Topics[2],
				requesterTopic,
			)
		}

		wantNonceTopic := common.BigToHash(
			new(big.Int).SetUint64(
				uint64(index),
			),
		)

		if log.Topics[3] != wantNonceTopic {
			t.Fatalf(
				"log %d nonce topic = %s, want %s",
				index,
				log.Topics[3],
				wantNonceTopic,
			)
		}

		if len(log.Data) != 96 {
			t.Fatalf(
				"log %d data length = %d, want 96",
				index,
				len(log.Data),
			)
		}

		if new(big.Int).SetBytes(
			log.Data[0:32],
		).Sign() != 0 {
			t.Fatalf(
				"log %d callbackGasLimit is non-zero",
				index,
			)
		}

		if got := common.BytesToHash(
			log.Data[32:64],
		); got != appDataHash {
			t.Fatalf(
				"log %d appDataHash = %s, want %s",
				index,
				got,
				appDataHash,
			)
		}

		if got := new(big.Int).SetBytes(
			log.Data[64:96],
		).Uint64(); got != 20_000 {
			t.Fatalf(
				"log %d feePaid = %d, want 20000",
				index,
				got,
			)
		}
	}

	if got := sdb.GetBalance(caller).Uint64(); got != 960_000 {
		t.Fatalf(
			"caller balance = %d, want 960000",
			got,
		)
	}

	if got := sdb.GetBalance(
		params.RabbitVRFCoordinatorV1Address,
	).Uint64(); got != 40_000 {
		t.Fatalf(
			"coordinator balance = %d, want 40000",
			got,
		)
	}
}
