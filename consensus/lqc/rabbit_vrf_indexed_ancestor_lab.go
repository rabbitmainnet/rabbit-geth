//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package lqc

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core/types"
)

func rabbitVRFIndexedAncestorV1(
	chain consensus.ChainHeaderReader,
	fromNumber uint64,
	fromHash common.Hash,
	targetNumber uint64,
) *types.Header {
	if chain == nil || fromHash == (common.Hash{}) || targetNumber > fromNumber {
		return nil
	}
	indexed, ok := chain.(interface {
		GetAncestor(common.Hash, uint64, uint64, *uint64) (common.Hash, uint64)
	})
	if !ok {
		return nil
	}
	distance := fromNumber - targetNumber
	budget := distance
	hash, number := indexed.GetAncestor(fromHash, fromNumber, distance, &budget)
	if hash == (common.Hash{}) || number != targetNumber {
		return nil
	}
	header := chain.GetHeader(hash, number)
	if header == nil || header.Number == nil ||
		!header.Number.IsUint64() || header.Number.Uint64() != targetNumber ||
		header.Hash() != hash {
		return nil
	}
	return header
}
