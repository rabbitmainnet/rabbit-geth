//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
)

const rabbitVRFRequestStatusPendingV1 = uint8(1)

type rabbitVRFCanonicalRequestV1 struct {
	RequestID        common.Hash
	Requester        common.Address
	RequesterNonce   uint64
	RequestBlock     uint64
	Epoch            uint64
	Round            uint64
	CallbackGasLimit uint32
	FeePaid          *big.Int
	AppDataHash      common.Hash
	Randomness       common.Hash
	ProofHash        common.Hash
	Status           uint8
}

func (runtime *rabbitVRFDKGRuntime) canonicalPendingRequestV1(requestID common.Hash) (rabbitVRFCanonicalRequestV1, error) {
	var out rabbitVRFCanonicalRequestV1

	if runtime == nil {
		return out, errors.New("rabbit vrf canonical request runtime unavailable")
	}
	if requestID == (common.Hash{}) {
		return out, errors.New("zero rabbit vrf request id")
	}
	if runtime.canonicalRequestLookup != nil {
		return runtime.canonicalRequestLookup(requestID)
	}
	if runtime.backend == nil || runtime.backend.blockchain == nil {
		return out, errors.New("rabbit vrf canonical request runtime unavailable")
	}

	header := runtime.backend.blockchain.CurrentBlock()
	if header == nil {
		return out, errors.New("rabbit vrf canonical head unavailable")
	}

	stateDB, err := runtime.backend.blockchain.StateAt(header)
	if err != nil {
		return out, fmt.Errorf("load rabbit vrf canonical state: %w", err)
	}

	selector := crypto.Keccak256([]byte("getRequest(bytes32)"))[:4]
	input := make([]byte, 4+32)
	copy(input[:4], selector)
	copy(input[4:], requestID[:])

	blockContext := core.NewEVMBlockContext(
		header,
		runtime.backend.blockchain,
		nil,
	)
	evm := vm.NewEVM(
		blockContext,
		stateDB,
		runtime.backend.blockchain.Config(),
		*runtime.backend.blockchain.GetVMConfig(),
	)

	ret, _, err := evm.Call(
		params.SystemAddress,
		params.RabbitVRFCoordinatorV1Address,
		input,
		vm.NewGasBudget(1_000_000, 1_000_000),
		common.U2560,
	)
	if err != nil {
		return out, fmt.Errorf("rabbit vrf getRequest call failed: %w", err)
	}
	if len(ret) != 11*32 {
		return out, fmt.Errorf("rabbit vrf getRequest returned %d bytes, want %d", len(ret), 11*32)
	}

	word := func(index int) []byte {
		start := index * 32
		return ret[start : start+32]
	}

	out = rabbitVRFCanonicalRequestV1{
		RequestID:        requestID,
		Requester:        common.BytesToAddress(word(0)[12:]),
		RequesterNonce:   new(big.Int).SetBytes(word(1)).Uint64(),
		RequestBlock:     new(big.Int).SetBytes(word(2)).Uint64(),
		Epoch:            new(big.Int).SetBytes(word(3)).Uint64(),
		Round:            new(big.Int).SetBytes(word(4)).Uint64(),
		CallbackGasLimit: uint32(new(big.Int).SetBytes(word(5)).Uint64()),
		FeePaid:          new(big.Int).SetBytes(word(6)),
		AppDataHash:      common.BytesToHash(word(7)),
		Randomness:       common.BytesToHash(word(8)),
		ProofHash:        common.BytesToHash(word(9)),
		Status:           uint8(new(big.Int).SetBytes(word(10)).Uint64()),
	}

	if out.Requester == (common.Address{}) || out.RequestBlock == 0 {
		return rabbitVRFCanonicalRequestV1{}, errors.New("rabbit vrf request does not exist")
	}
	if out.Status != rabbitVRFRequestStatusPendingV1 {
		return rabbitVRFCanonicalRequestV1{}, fmt.Errorf("rabbit vrf request status %d is not pending", out.Status)
	}
	if out.Randomness != (common.Hash{}) || out.ProofHash != (common.Hash{}) {
		return rabbitVRFCanonicalRequestV1{}, errors.New("rabbit vrf pending request already has result")
	}

	return out, nil
}
