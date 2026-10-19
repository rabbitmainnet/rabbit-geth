//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package lqc

import (
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core/types"
)

// Reconstruct the registry with the same WorkSeat liveness rules used by
// canonical header validation. Every advertised registry root remains checked.
func (l *LQC) replayRegistryRootMismatchByWorkSeats(
	chain consensus.ChainHeaderReader,
	parent *RegistrySnapshot,
	header *types.Header,
	original error,
) (*RegistrySnapshot, error) {
	if chain == nil || chain.Config() == nil ||
		header == nil || header.Number == nil ||
		!l.consensusLivenessV3Active(header.Number.Uint64()) {
		return nil, original
	}

	envelope := LQCHeaderEnvelopeV3{
		Version: LQCHeaderEnvelopeVersionV3,
	}
	if chain.Config().IsRabbitVRF(header.Number) {
		decoded, err := DecodeLQCHeaderExtraV5(
			header.Extra, MaxWorkTicketsPerBlockV1,
		)
		if err != nil {
			return nil, err
		}
		envelope.BlockNumber = decoded.BlockNumber
		envelope.RegistryRoot = decoded.RegistryRoot
		envelope.WorkStateRoot = decoded.WorkStateRoot
		envelope.RegistryOperations = decoded.RegistryOperations
		envelope.WorkTickets = decoded.WorkTickets
	} else {
		decoded, err := DecodeLQCHeaderExtraV4(
			header.Extra, MaxWorkTicketsPerBlockV1,
		)
		if err != nil {
			return nil, err
		}
		envelope.BlockNumber = decoded.BlockNumber
		envelope.RegistryRoot = decoded.RegistryRoot
		envelope.WorkStateRoot = decoded.WorkStateRoot
		envelope.RegistryOperations = decoded.RegistryOperations
		envelope.WorkTickets = decoded.WorkTickets
	}

	l.rememberRegistrySnapshot(parent)
	runtime, err := l.workV1EngineLabRuntimeAt(
		chain, parent.Number, parent.Hash,
	)
	if err != nil {
		return nil, err
	}
	return l.workV1EngineLabReplayRegistryV3(
		chain, runtime, header, envelope,
	)
}
