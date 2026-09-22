//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

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
	if chain == nil || !l.consensusLivenessV3Active(number) {
		return nil, false, nil
	}

	if _, err := l.workV1EngineLabRuntimeAt(chain, number, hash); err != nil {
		return nil, true, err
	}

	snapshot, ok := l.cachedRegistrySnapshot(number, hash)
	if !ok {
		return nil, true, ErrRegistrySnapshotChainMismatch
	}
	return snapshot, true, nil
}
