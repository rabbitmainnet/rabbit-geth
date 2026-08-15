package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rpc"
)

type stateSnapshot struct {
	Accounts   map[common.Address]accountState
	IndexCount uint64
}

type rawAccountState struct {
	address  common.Address
	balance  *hexutil.Big
	locked   *hexutil.Bytes
	original *hexutil.Bytes
	stage    *hexutil.Bytes
}

type traceAccount struct {
	Balance *hexutil.Big `json:"balance,omitempty"`
}

type prestateDiff struct {
	Pre  map[common.Address]traceAccount `json:"pre"`
	Post map[common.Address]traceAccount `json:"post"`
}

type traceEnvelope struct {
	Result json.RawMessage `json:"result"`
	Error  string          `json:"error,omitempty"`
}

func fetchState(ctx context.Context, client *rpc.Client, blockHash common.Hash, addresses []common.Address) (stateSnapshot, error) {
	blockReference := rpc.BlockNumberOrHashWithHash(blockHash, false)
	holders := make([]rawAccountState, 0, len(addresses))
	batch := make([]rpc.BatchElem, 0, len(addresses)*4+1)
	for _, address := range addresses {
		holder := rawAccountState{
			address:  address,
			balance:  new(hexutil.Big),
			locked:   new(hexutil.Bytes),
			original: new(hexutil.Bytes),
			stage:    new(hexutil.Bytes),
		}
		holders = append(holders, holder)
		batch = append(batch,
			rpc.BatchElem{
				Method: "eth_getBalance",
				Args:   []interface{}{address, blockReference},
				Result: holder.balance,
			},
			rpc.BatchElem{
				Method: "eth_getStorageAt",
				Args:   []interface{}{vestingSystemAddress, lockedBalanceSlot(address).Hex(), blockReference},
				Result: holder.locked,
			},
			rpc.BatchElem{
				Method: "eth_getStorageAt",
				Args:   []interface{}{vestingSystemAddress, originalLockedBalanceSlot(address).Hex(), blockReference},
				Result: holder.original,
			},
			rpc.BatchElem{
				Method: "eth_getStorageAt",
				Args:   []interface{}{vestingSystemAddress, releasedStageSlot(address).Hex(), blockReference},
				Result: holder.stage,
			},
		)
	}
	indexCount := new(hexutil.Bytes)
	batch = append(batch, rpc.BatchElem{
		Method: "eth_getStorageAt",
		Args:   []interface{}{vestingSystemAddress, vestingIndexCountSlot().Hex(), blockReference},
		Result: indexCount,
	})
	if err := client.BatchCallContext(ctx, batch); err != nil {
		return stateSnapshot{}, fmt.Errorf("state batch at %s: %w", blockHash.Hex(), err)
	}
	for _, element := range batch {
		if element.Error != nil {
			return stateSnapshot{}, fmt.Errorf("%s at %s: %w", element.Method, blockHash.Hex(), element.Error)
		}
	}
	result := stateSnapshot{
		Accounts:   make(map[common.Address]accountState, len(holders)),
		IndexCount: new(big.Int).SetBytes(*indexCount).Uint64(),
	}
	for _, holder := range holders {
		stage := uint8(0)
		if len(*holder.stage) > 0 {
			stage = (*holder.stage)[len(*holder.stage)-1]
		}
		result.Accounts[holder.address] = accountState{
			Balance:  cloneBig((*big.Int)(holder.balance)),
			Locked:   new(big.Int).SetBytes(*holder.locked),
			Original: new(big.Int).SetBytes(*holder.original),
			Stage:    stage,
		}
	}
	return result, nil
}

func fetchIndexedAddresses(ctx context.Context, client *rpc.Client, blockHash common.Hash, count uint64) ([]common.Address, error) {
	if count == 0 {
		return nil, nil
	}
	blockReference := rpc.BlockNumberOrHashWithHash(blockHash, false)
	values := make([]*hexutil.Bytes, count)
	batch := make([]rpc.BatchElem, count)
	for i := uint64(0); i < count; i++ {
		values[i] = new(hexutil.Bytes)
		batch[i] = rpc.BatchElem{
			Method: "eth_getStorageAt",
			Args:   []interface{}{vestingSystemAddress, vestingIndexSlot(i).Hex(), blockReference},
			Result: values[i],
		}
	}
	if err := client.BatchCallContext(ctx, batch); err != nil {
		return nil, fmt.Errorf("vesting index batch: %w", err)
	}
	addresses := make([]common.Address, 0, count)
	for i, element := range batch {
		if element.Error != nil {
			return nil, fmt.Errorf("vesting index %d: %w", i, element.Error)
		}
		addresses = append(addresses, common.BytesToAddress(*values[i]))
	}
	return addresses, nil
}

func transactionBalanceDeltas(ctx context.Context, client *rpc.Client, block uint64) (map[common.Address]*big.Int, error) {
	var traces []traceEnvelope
	config := map[string]interface{}{
		"tracer": "prestateTracer",
		"tracerConfig": map[string]interface{}{
			"diffMode":       true,
			"disableCode":    true,
			"disableStorage": true,
		},
		"timeout": "60s",
	}
	if err := client.CallContext(ctx, &traces, "debug_traceBlockByNumber", hexutil.EncodeUint64(block), config); err != nil {
		return nil, fmt.Errorf("debug_traceBlockByNumber(%d): %w", block, err)
	}
	deltas := make(map[common.Address]*big.Int)
	for index, envelope := range traces {
		if envelope.Error != "" {
			return nil, fmt.Errorf("transaction trace %d in block %d: %s", index, block, envelope.Error)
		}
		var diff prestateDiff
		if err := json.Unmarshal(envelope.Result, &diff); err != nil {
			return nil, fmt.Errorf("decode transaction trace %d in block %d: %w", index, block, err)
		}
		addresses := make(map[common.Address]struct{}, len(diff.Pre)+len(diff.Post))
		for address := range diff.Pre {
			addresses[address] = struct{}{}
		}
		for address := range diff.Post {
			addresses[address] = struct{}{}
		}
		for address := range addresses {
			pre, existedBefore := diff.Pre[address]
			post, existsAfter := diff.Post[address]
			before := new(big.Int)
			if existedBefore && pre.Balance != nil {
				before.Set((*big.Int)(pre.Balance))
			}
			after := new(big.Int)
			switch {
			case existsAfter && post.Balance != nil:
				after.Set((*big.Int)(post.Balance))
			case existsAfter && existedBefore:
				after.Set(before)
			}
			delta := new(big.Int).Sub(after, before)
			if delta.Sign() == 0 {
				continue
			}
			if existing := deltas[address]; existing != nil {
				existing.Add(existing, delta)
			} else {
				deltas[address] = delta
			}
		}
	}
	return deltas, nil
}
