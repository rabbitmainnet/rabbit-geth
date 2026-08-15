//go:build (!rabbit_workv1_engine_lab && !rabbit_workv1) || !rabbit_randomx

package lqc

import (
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core/types"
)

var ErrWorkV1EngineLabUnavailable = errors.New(
	"lqc Work V1 engine laboratory runtime unavailable",
)

func (l *LQC) WorkV1EngineLabRelayContext(
	chain consensus.ChainHeaderReader,
	parentNumber uint64,
	parentHash common.Hash,
	blockNumber uint64,
) (
	uint64,
	common.Hash,
	common.Hash,
	*big.Int,
	WorkRelayEligibilityCheckV1,
	error,
) {
	return 0, common.Hash{}, common.Hash{}, nil, nil,
		ErrWorkV1EngineLabUnavailable
}

func (l *LQC) prepareWorkV1EngineLabHook(
	chain consensus.ChainHeaderReader,
	header *types.Header,
) error {
	return nil
}

func (l *LQC) verifyCanonicalRegistryHeaderMaybeWorkV1Lab(
	chain consensus.ChainHeaderReader,
	header *types.Header,
) (HybridSelection, *RegistrySnapshot, error) {
	return l.verifyCanonicalRegistryHeader(chain, header)
}

func (l *LQC) selectionForHeaderMaybeWorkV1Lab(
	chain consensus.ChainHeaderReader,
	header *types.Header,
) HybridSelection {
	return l.selectionForHeader(chain, header)
}

func (l *LQC) prepareCanonicalRegistryExtraMaybeWorkV1Lab(
	chain consensus.ChainHeaderReader,
	header *types.Header,
) (HybridSelection, error) {
	return l.prepareCanonicalRegistryExtra(chain, header)
}
