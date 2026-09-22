//go:build (!rabbit_workv1_engine_lab && !rabbit_workv1) || !rabbit_randomx

package lqc

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
)

func (l *LQC) registrySnapshotAtMaybeWorkV1Lab(
	chain consensus.ChainHeaderReader,
	number uint64,
	hash common.Hash,
) (*RegistrySnapshot, bool, error) {
	return nil, false, nil
}
