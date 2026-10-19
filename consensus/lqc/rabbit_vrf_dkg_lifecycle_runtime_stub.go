//go:build (!rabbit_workv1_engine_lab && !rabbit_workv1) || !rabbit_randomx

package lqc

import (
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core/types"
)

func (l *LQC) maybeEnsureRabbitVRFDKGLifecycleV1(
	chain consensus.ChainHeaderReader,
	header *types.Header,
) error {
	return nil
}
