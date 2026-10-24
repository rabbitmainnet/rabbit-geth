package core

import (
	"bytes"
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/misc"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/holiman/uint256"
)

func TestRabbitVRFEconomicsV2StateMigration(t *testing.T) {
	config := rabbitVRFTestConfig(4)
	config.LQC.VRFEconomicsV2Block = 312000
	sdb := mkState(nil)
	misc.ApplyRabbitVRFCoordinatorV1(sdb)
	evm := amsterdamCoreEVM(sdb)
	addr := params.RabbitVRFCoordinatorV1Address

	balance := uint256.NewInt(123456789)
	sdb.SetBalance(addr, balance, tracing.BalanceChangeUnspecified)
	sdb.SetNonce(addr, 77, tracing.NonceChangeNewContract)

	// Sentinel storage checks detect accidental account recreation.
	slots := map[common.Hash]common.Hash{
		common.HexToHash("0x1234"):                         common.HexToHash("0xcafebabe"),
		crypto.Keccak256Hash([]byte("migration-sentinel")): common.HexToHash("0xabcdef"),
	}
	for key, value := range slots {
		sdb.SetState(addr, key, value)
	}

	check := func(wantCode []byte, wantPrice int64) {
		t.Helper()
		if !bytes.Equal(sdb.GetCode(addr), wantCode) {
			t.Fatal("unexpected coordinator code")
		}
		if sdb.GetBalance(addr).Cmp(balance) != 0 {
			t.Fatal("coordinator balance changed")
		}
		if sdb.GetNonce(addr) != 77 {
			t.Fatal("coordinator nonce changed")
		}
		for key, value := range slots {
			if sdb.GetState(addr, key) != value {
				t.Fatal("sentinel storage changed")
			}
		}
		input := crypto.Keccak256(
			[]byte("VRF_BASE_FEE_TRUSD_BASE_UNITS()"),
		)[:4]
		ret, _, err := evm.Call(
			common.Address{0xca}, addr, input,
			vm.NewGasBudget(1_000_000, 0), new(uint256.Int),
		)
		if err != nil {
			t.Fatal(err)
		}
		if len(ret) != 32 || new(big.Int).SetBytes(ret).Cmp(big.NewInt(wantPrice)) != 0 {
			t.Fatalf("unexpected base price: %x", ret)
		}
	}

	for _, height := range []int64{311999, 312000, 312001} {
		evm.Context.BlockNumber = big.NewInt(height)
		PreExecution(
			context.Background(), nil,
			&types.Header{Number: big.NewInt(height - 1), Time: uint64(height-1) * 10},
			config, evm, big.NewInt(height), uint64(height)*10,
		)
		if height < 312000 {
			check(params.RabbitVRFCoordinatorV1Code, 1000)
		} else {
			check(params.RabbitVRFCoordinatorV2Code, 5000)
		}
	}
}
