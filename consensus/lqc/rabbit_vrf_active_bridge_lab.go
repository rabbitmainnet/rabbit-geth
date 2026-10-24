//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package lqc

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
)

// RabbitVRFActiveBridgeContextV1 uses exactly the block verifier's resolver.
func (l *LQC) RabbitVRFActiveBridgeContextV1(chain consensus.ChainHeaderReader, parentNumber uint64, parentHash common.Hash, blockNumber uint64) (RabbitVRFDKGBridgeV1, bool, error) {
	if chain == nil || chain.Config() == nil || chain.Config().LQC == nil {
		return RabbitVRFDKGBridgeV1{}, false, ErrWorkV1EngineLabUnavailable
	}
	epochLength, err := l.rabbitVRFCanonicalWorkEpochLengthV1(chain, parentNumber, parentHash)
	if err != nil {
		return RabbitVRFDKGBridgeV1{}, false, err
	}
	return l.rabbitVRFDKGBridgeForTargetBlockV1(chain, parentNumber, parentHash, blockNumber, epochLength)
}

// Both admission and execution use the parent Work state, never legacy LCQ epochs.
func (l *LQC) rabbitVRFCanonicalWorkEpochLengthV1(chain consensus.ChainHeaderReader, parentNumber uint64, parentHash common.Hash) (uint64, error) {
	if l == nil || chain == nil {
		return 0, ErrWorkV1EngineLabUnavailable
	}
	parent, err := l.workV1EngineLabRuntimeAt(chain, parentNumber, parentHash)
	if err != nil {
		return 0, err
	}
	if parent == nil || parent.Work == nil || parent.Work.EpochLength == 0 {
		return 0, ErrWorkV1EngineLabUnavailable
	}
	return parent.Work.EpochLength, nil
}
