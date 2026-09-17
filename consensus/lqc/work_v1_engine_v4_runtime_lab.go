//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package lqc

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
)

func (l *LQC) workV1EngineLabRememberClaimLedger(
	headerHash common.Hash,
	ledger *CommitteeClaimLedgerV1,
) error {
	if headerHash == (common.Hash{}) || ledger == nil {
		return ErrWorkV1EngineLabUnavailable
	}
	if err := ledger.Validate(); err != nil {
		return err
	}
	state, err := workV1EngineLabRuntimeFor(l)
	if err != nil {
		return err
	}
	state.mu.Lock()
	state.claimLedgers[headerHash] = ledger.clone()
	state.mu.Unlock()

	l.persistWorkV1EngineCheckpointIfReady(headerHash)
	return nil
}

func (l *LQC) workV1EngineLabCachedClaimLedger(
	hash common.Hash,
) (*CommitteeClaimLedgerV1, bool, error) {
	state, err := workV1EngineLabRuntimeFor(l)
	if err != nil {
		return nil, false, err
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	ledger, ok := state.claimLedgers[hash]
	if !ok {
		return nil, false, nil
	}
	return ledger.clone(), true, nil
}

func (l *LQC) workV1EngineLabParentClaimLedger(
	blockNumber uint64,
	parentHash common.Hash,
) (*CommitteeClaimLedgerV1, error) {
	if !l.consensusLivenessV3Active(blockNumber) || parentHash == (common.Hash{}) {
		return nil, ErrInvalidLQCHeaderRuntimeV4
	}
	if l.config != nil && blockNumber == l.config.ConsensusLivenessV3Block {
		return NewCommitteeClaimLedgerV1(), nil
	}
	ledger, ok, err := l.workV1EngineLabCachedClaimLedger(parentHash)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrWorkV1EngineLabParentMissing
	}
	return ledger, nil
}

func workV1EngineLabCommitteeSeatsV1(
	selection HybridSelection,
	seats []WorkSeatV1,
) ([]WorkSeatV1, error) {
	byParticipant := make(map[common.Address]WorkSeatV1, len(seats))
	for _, seat := range seats {
		byParticipant[seat.Participant] = seat
	}
	committee := make([]WorkSeatV1, len(selection.Committee))
	for index, participant := range selection.Committee {
		seat, ok := byParticipant[participant.Address]
		if !ok {
			return nil, ErrWorkV1EngineLabSelectionUnavailable
		}
		committee[index] = seat
	}
	return committee, nil
}

func (l *LQC) workV1EngineLabClaimResolver(
	chain consensus.ChainHeaderReader,
	fromNumber uint64,
	fromHash common.Hash,
) CommitteeClaimVerificationContextResolverV1 {
	return func(targetBlock uint64) (CommitteeParticipationVerificationContextV1, error) {
		if chain == nil || chain.Config() == nil || chain.Config().ChainID == nil ||
			targetBlock == 0 || targetBlock > fromNumber {
			return CommitteeParticipationVerificationContextV1{},
				ErrInvalidCommitteeParticipationVerificationV1
		}
		header := workV1EngineLabAncestorHeader(
			chain,
			fromNumber,
			fromHash,
			targetBlock,
		)
		if header == nil {
			return CommitteeParticipationVerificationContextV1{},
				ErrWorkV1EngineLabParentMissing
		}
		parentRuntime, err := l.workV1EngineLabRuntimeAt(
			chain,
			targetBlock-1,
			header.ParentHash,
		)
		if err != nil {
			return CommitteeParticipationVerificationContextV1{}, err
		}
		parentRegistry, err := l.registrySnapshotAt(
			chain,
			targetBlock-1,
			header.ParentHash,
		)
		if err != nil {
			return CommitteeParticipationVerificationContextV1{}, err
		}
		selection, _, err := l.workV1EngineLabSelectionForHeader(
			chain,
			parentRuntime,
			parentRegistry,
			header,
		)
		if err != nil {
			return CommitteeParticipationVerificationContextV1{}, err
		}
		committee, err := workV1EngineLabCommitteeSeatsV1(
			selection,
			parentRuntime.Work.SelectionSeats,
		)
		if err != nil {
			return CommitteeParticipationVerificationContextV1{}, err
		}
		sourceEpoch, hasSource, err := WorkSelectionSourceEpochV1(
			targetBlock,
			parentRuntime.Work.EpochLength,
		)
		if err != nil || !hasSource {
			return CommitteeParticipationVerificationContextV1{},
				ErrInvalidCommitteeParticipationVerificationV1
		}
		datasetNumber, err := WorkDatasetAnchorBlockV1(
			sourceEpoch,
			parentRuntime.Work.EpochLength,
		)
		if err != nil {
			return CommitteeParticipationVerificationContextV1{}, err
		}
		dataset := workV1EngineLabAncestorHeader(
			chain,
			parentRuntime.Work.Number,
			parentRuntime.Work.Hash,
			datasetNumber,
		)
		if dataset == nil {
			return CommitteeParticipationVerificationContextV1{},
				ErrWorkV1EngineLabParentMissing
		}
		datasetKey, err := RandomXWorkDatasetKeyV1(
			chain.Config().ChainID,
			sourceEpoch,
			dataset.Hash(),
		)
		if err != nil {
			return CommitteeParticipationVerificationContextV1{}, err
		}
		state, err := workV1EngineLabRuntimeFor(l)
		if err != nil {
			return CommitteeParticipationVerificationContextV1{}, err
		}
		return CommitteeParticipationVerificationContextV1{
			ChainID:       new(big.Int).Set(chain.Config().ChainID),
			DatasetKey:    datasetKey,
			ParentHash:    header.ParentHash,
			SelectionRoot: parentRuntime.Work.SelectionRoot,
			Committee:     committee,
			Hasher:        CommitteeParticipationHasherV1(state.hasher),
		}, nil
	}
}

// WorkV1EngineLabCommitteeContext returns the branch-bound verification
// context for a canonical target block. Relay code uses this same resolver as
// consensus, so it cannot accept a claim for a different committee or fork.
func (l *LQC) WorkV1EngineLabCommitteeContext(
	chain consensus.ChainHeaderReader,
	blockNumber uint64,
	blockHash common.Hash,
) (CommitteeParticipationVerificationContextV1, error) {
	if chain == nil || blockNumber == 0 || blockHash == (common.Hash{}) ||
		!l.consensusLivenessV3Active(blockNumber) {
		return CommitteeParticipationVerificationContextV1{},
			ErrInvalidCommitteeParticipationVerificationV1
	}
	header := chain.GetHeader(blockHash, blockNumber)
	if header == nil {
		return CommitteeParticipationVerificationContextV1{},
			ErrWorkV1EngineLabParentMissing
	}
	return l.workV1EngineLabClaimResolver(
		chain,
		blockNumber,
		blockHash,
	)(blockNumber)
}

func (l *LQC) workV1EngineLabV4Context(
	chain consensus.ChainHeaderReader,
	work LQCHeaderWorkRuntimeContextV1,
	parentHash common.Hash,
) (LQCHeaderV4RuntimeContextV1, error) {
	ledger, err := l.workV1EngineLabParentClaimLedger(
		work.BlockNumber,
		parentHash,
	)
	if err != nil {
		return LQCHeaderV4RuntimeContextV1{}, err
	}
	return LQCHeaderV4RuntimeContextV1{
		Work:         work,
		ParentClaims: ledger,
		ResolveClaims: l.workV1EngineLabClaimResolver(
			chain,
			work.BlockNumber-1,
			parentHash,
		),
	}, nil
}
