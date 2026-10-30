package eth

import (
	"bytes"
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
)

// vrfRewardCredits reports native settlement independently of balance deltas.
func (api *LQCRegistryAPI) vrfRewardCredits(number uint64, expectedHash common.Hash, participant common.Address) ([]VRFRewardCreditResult, error) {
	out := make([]VRFRewardCreditResult, 0)
	chain := api.eth.blockchain
	header := chain.GetHeaderByNumber(number)
	if header == nil || header.Hash() != expectedHash {
		return nil, errors.New("canonical VRF reward block unavailable")
	}
	reader, ok := api.eth.engine.(consensus.RabbitVRFExecutionFinalizationReader)
	if !ok {
		return nil, errors.New("VRF settlement reporting unavailable")
	}
	values, err := reader.RabbitVRFExecutionFinalizations(chain, header)
	if err != nil {
		return nil, err
	}
	if len(values) == 0 {
		return out, nil
	}
	stateDB, err := chain.StateAt(header)
	if err != nil {
		return nil, errors.New("historical VRF settlement state unavailable")
	}
	context := core.NewEVMBlockContext(header, chain, nil)
	evm := vm.NewEVM(context, stateDB, chain.Config(), *chain.GetVMConfig())

	for _, value := range values {
		member := false
		for _, address := range value.Participants {
			if address == participant {
				member = true
				break
			}
		}
		if !member && header.Coinbase != participant {
			continue
		}
		input := append(
			append([]byte{}, crypto.Keccak256([]byte("getRequest(bytes32)"))[:4]...),
			value.RequestID[:]...,
		)
		snapshot := stateDB.Snapshot()
		ret, _, callErr := evm.StaticCall(
			params.SystemAddress,
			params.RabbitVRFCoordinatorV1Address,
			input,
			vm.NewGasBudget(1_000_000, 1_000_000),
		)
		stateDB.RevertToSnapshot(snapshot)
		if callErr != nil || len(ret) != 11*32 {
			return nil, errors.New("VRF settlement request details unavailable")
		}
		word := func(i int) []byte { return ret[i*32 : (i+1)*32] }
		if new(big.Int).SetBytes(word(10)).Uint64() != 2 ||
			!bytes.Equal(word(8), value.Randomness[:]) ||
			!bytes.Equal(word(9), value.ProofHash[:]) ||
			new(big.Int).SetBytes(word(3)).Uint64() != value.Epoch ||
			new(big.Int).SetBytes(word(4)).Uint64() != value.Round {
			return nil, errors.New("VRF settlement confirmation mismatch")
		}
		fee := new(big.Int).SetBytes(word(6))
		if fee.Sign() <= 0 {
			return nil, errors.New("invalid VRF settlement fee")
		}
		producer, each, err := core.RabbitVRFRewardAmounts(
			fee, len(value.Participants),
			chain.Config().IsRabbitVRFEconomicsV2(header.Number),
		)
		if err != nil {
			return nil, err
		}
		add := func(role string, amount *big.Int) {
			if amount != nil && amount.Sign() > 0 {
				out = append(out, VRFRewardCreditResult{
					RequestID: value.RequestID,
					Role:      role,
					Amount:    (*hexutil.Big)(new(big.Int).Set(amount)),
				})
			}
		}
		if header.Coinbase == participant {
			add("PRODUCER", producer)
		}
		if member {
			add("THRESHOLD PARTICIPANT", each)
		}
	}
	current := chain.GetHeaderByNumber(number)
	if current == nil || current.Hash() != header.Hash() {
		return nil, errors.New("canonical VRF reward block changed; retry")
	}
	return out, nil
}
