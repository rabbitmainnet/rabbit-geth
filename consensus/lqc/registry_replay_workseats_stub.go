//go:build (!rabbit_workv1_engine_lab && !rabbit_workv1) || !rabbit_randomx

package lqc

import (
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core/types"
)

func (l *LQC) replayRegistryRootMismatchByWorkSeats(
	chain consensus.ChainHeaderReader,
	parent *RegistrySnapshot,
	header *types.Header,
	original error,
) (*RegistrySnapshot, error) {
	return nil, original
}
