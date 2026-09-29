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
