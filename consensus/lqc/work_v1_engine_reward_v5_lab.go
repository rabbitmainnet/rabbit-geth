//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package lqc

import (
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core/types"
)

// Historical blocks retain the original V4-only reward decoder.
func (l *LQC) workV1EngineLabRewardEnvelopeForHeader(
	chain consensus.ChainHeaderReader,
	header *types.Header,
) (LQCHeaderEnvelopeV4, error) {
	if chain == nil || header == nil || header.Number == nil ||
		!header.Number.IsUint64() || header.Number.Sign() <= 0 {
		return LQCHeaderEnvelopeV4{}, ErrInvalidLQCHeaderRuntimeV4
	}

	if l != nil && l.config != nil &&
		l.config.MiningRewardV5Block != 0 &&
		header.Number.Uint64() >= l.config.MiningRewardV5Block {
		config := chain.Config()
		if config != nil && config.IsRabbitVRF(header.Number) {
			envelope, err := ValidateLQCHeaderExtraV5(
				header.Number.Uint64(),
				MaxWorkTicketsPerBlockV1,
				header.Extra,
			)
			if err != nil {
				return LQCHeaderEnvelopeV4{}, err
			}
			return LQCHeaderEnvelopeV4{
				Version:                      LQCHeaderEnvelopeVersionV4,
				BlockNumber:                  envelope.BlockNumber,
				RegistryRoot:                 envelope.RegistryRoot,
				WorkStateRoot:                envelope.WorkStateRoot,
				CommitteeClaimRoot:           envelope.CommitteeClaimRoot,
				RegistryOperations:           envelope.RegistryOperations,
				WorkTickets:                  envelope.WorkTickets,
				CommitteeParticipationClaims: envelope.CommitteeParticipationClaims,
			}, nil
		}
	}

	return ValidateLQCHeaderExtraV4(
		header.Number.Uint64(),
		MaxWorkTicketsPerBlockV1,
		header.Extra,
	)
}
