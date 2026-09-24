//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package lqc

import (
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core/types"
)

// maybeEnsureRabbitVRFDKGLifecycleV1 derives the canonical DKG bridge for the
// block being finalized and creates or resumes its immutable public lifecycle.
//
// This path deliberately performs no transport-key generation, credential
// access or P2P publication.
func (l *LQC) maybeEnsureRabbitVRFDKGLifecycleV1(
	chain consensus.ChainHeaderReader,
	header *types.Header,
) error {
	if l == nil ||
		chain == nil ||
		chain.Config() == nil ||
		header == nil ||
		header.Number == nil ||
		!header.Number.IsUint64() ||
		header.Number.Sign() <= 0 {
		return nil
	}

	blockNumber := header.Number.Uint64()

	bridge, ok, err :=
		l.RabbitVRFDKGBridgeContextV1(
			chain,
			blockNumber-1,
			header.ParentHash,
			blockNumber,
		)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}

	_, _, err =
		l.EnsureRabbitVRFDKGLifecycleV1(
			bridge,
		)

	return err
}
