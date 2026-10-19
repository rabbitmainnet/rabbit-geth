//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package lqc

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
)

// RabbitVRFDKGBridgeForTargetBlockV1 exposes the same historical, parent-anchored
// serving session used by canonical header verification. Preparation may already
// be working on the next target epoch and must not select block artifacts.
func (l *LQC) RabbitVRFDKGBridgeForTargetBlockV1(
	chain consensus.ChainHeaderReader,
	parentNumber uint64,
	parentHash common.Hash,
	blockNumber uint64,
	epochLength uint64,
) (RabbitVRFDKGBridgeV1, bool, error) {
	return l.rabbitVRFDKGBridgeForTargetBlockV1(chain, parentNumber, parentHash, blockNumber, epochLength)
}
