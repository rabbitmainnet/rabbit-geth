package core

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core/types"
)

func rabbitVRFValidatedFinalizationsForExecution(
	reader consensus.RabbitVRFValidatedFinalizationReader,
	verifyHeader func(consensus.ChainHeaderReader, *types.Header) error,
	chain consensus.ChainHeaderReader,
	header *types.Header,
	blockHash common.Hash,
) ([]consensus.RabbitVRFValidatedFinalization, error) {
	finalizations, found, err := reader.RabbitVRFValidatedFinalizations(blockHash)
	if err != nil {
		return nil, err
	}
	if !found {
		if err := verifyHeader(chain, header); err != nil {
			return nil, fmt.Errorf(
				"verify Rabbit VRF header before execution: %w",
				err,
			)
		}
		finalizations, found, err =
			reader.RabbitVRFValidatedFinalizations(blockHash)
		if err != nil {
			return nil, err
		}
	}
	if !found {
		return nil, fmt.Errorf(
			"Rabbit VRF validated finalizations unavailable",
		)
	}
	return finalizations, nil
}

func rabbitVRFFinalizationsForEngineExecution(
	engine consensus.Engine,
	chain consensus.ChainHeaderReader,
	header *types.Header,
	blockHash common.Hash,
) ([]consensus.RabbitVRFValidatedFinalization, error) {
	if reader, ok := engine.(consensus.RabbitVRFExecutionFinalizationReader); ok {
		if _, err := rabbitVRFValidatedFinalizationsForExecution(
			reader,
			engine.VerifyHeader,
			chain,
			header,
			blockHash,
		); err != nil {
			return nil, err
		}
		return reader.RabbitVRFExecutionFinalizations(chain, header)
	}

	reader, ok := engine.(consensus.RabbitVRFValidatedFinalizationReader)
	if !ok {
		return nil, fmt.Errorf(
			"Rabbit VRF validated finalization reader unavailable",
		)
	}

	finalizations, err := rabbitVRFValidatedFinalizationsForExecution(
		reader,
		engine.VerifyHeader,
		chain,
		header,
		blockHash,
	)
	if err != nil {
		return nil, err
	}
	if len(finalizations) != 0 {
		return nil, fmt.Errorf(
			"Rabbit VRF economic finalizations require execution reader",
		)
	}
	return finalizations, nil
}
