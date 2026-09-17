//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package lqc

import (
	"bytes"
	"encoding/gob"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/log"
)

const workV1EngineCheckpointVersionV1 uint8 = 1

var workV1EngineCheckpointPrefixV1 = []byte("rabbit-lqc-work-runtime-checkpoint-v1:")

type workV1EngineCheckpointV1 struct {
	Version        uint8
	Number         uint64
	Hash           common.Hash
	Runtime        *CanonicalWorkRuntimeStateV1
	HasClaimLedger bool
	ClaimLedger    *CommitteeClaimLedgerV1
}

func workV1EngineCheckpointKeyV1(hash common.Hash) []byte {
	key := make([]byte, 0, len(workV1EngineCheckpointPrefixV1)+len(hash))
	key = append(key, workV1EngineCheckpointPrefixV1...)
	key = append(key, hash[:]...)
	return key
}

func encodeWorkV1EngineCheckpointV1(checkpoint *workV1EngineCheckpointV1) ([]byte, error) {
	var out bytes.Buffer
	err := gob.NewEncoder(&out).Encode(checkpoint)
	return out.Bytes(), err
}

func decodeWorkV1EngineCheckpointV1(blob []byte) (*workV1EngineCheckpointV1, error) {
	var checkpoint workV1EngineCheckpointV1
	if err := gob.NewDecoder(bytes.NewReader(blob)).Decode(&checkpoint); err != nil {
		return nil, err
	}
	return &checkpoint, nil
}

func (l *LQC) persistWorkV1EngineCheckpointIfReady(headerHash common.Hash) {
	if l == nil || l.db == nil || headerHash == (common.Hash{}) {
		return
	}
	state, err := workV1EngineLabRuntimeFor(l)
	if err != nil {
		return
	}

	state.mu.Lock()
	runtime := state.runtimes[headerHash]
	if runtime == nil || runtime.Work == nil ||
		runtime.Work.Hash != headerHash ||
		runtime.Work.Number == 0 ||
		runtime.Work.Number%l.registryCheckpointInterval() != 0 {
		state.mu.Unlock()
		return
	}

	checkpoint := &workV1EngineCheckpointV1{
		Version: workV1EngineCheckpointVersionV1,
		Number:  runtime.Work.Number,
		Hash:    headerHash,
		Runtime: runtime,
	}
	if l.consensusLivenessV3Active(runtime.Work.Number) {
		ledger := state.claimLedgers[headerHash]
		if ledger == nil {
			state.mu.Unlock()
			return
		}
		checkpoint.HasClaimLedger = true
		checkpoint.ClaimLedger = ledger.clone()
	}
	state.mu.Unlock()

	blob, err := encodeWorkV1EngineCheckpointV1(checkpoint)
	if err != nil {
		log.Warn("Failed to encode LQC recovery checkpoint",
			"number", checkpoint.Number, "hash", checkpoint.Hash, "err", err)
		return
	}
	if err := l.db.Put(workV1EngineCheckpointKeyV1(headerHash), blob); err != nil {
		log.Warn("Failed to store LQC recovery checkpoint",
			"number", checkpoint.Number, "hash", checkpoint.Hash, "err", err)
		return
	}
	log.Debug("Stored LQC recovery checkpoint",
		"number", checkpoint.Number, "hash", checkpoint.Hash)
}

func (l *LQC) workV1EngineLabRestoreCheckpoint(
	chain consensus.ChainHeaderReader,
	number uint64,
	hash common.Hash,
) (bool, error) {
	if l == nil || l.db == nil || chain == nil ||
		chain.Config() == nil || chain.Config().ChainID == nil ||
		hash == (common.Hash{}) || number == 0 ||
		number%l.registryCheckpointInterval() != 0 {
		return false, nil
	}

	key := workV1EngineCheckpointKeyV1(hash)
	has, err := l.db.Has(key)
	if err != nil {
		return false, err
	}
	if !has {
		return false, nil
	}
	blob, err := l.db.Get(key)
	if err != nil {
		return false, err
	}
	checkpoint, err := decodeWorkV1EngineCheckpointV1(blob)
	if err != nil || checkpoint == nil {
		return false, nil
	}
	if checkpoint.Version != workV1EngineCheckpointVersionV1 ||
		checkpoint.Number != number ||
		checkpoint.Hash != hash ||
		checkpoint.Runtime == nil ||
		checkpoint.Runtime.Work == nil ||
		checkpoint.Runtime.Work.Number != number ||
		checkpoint.Runtime.Work.Hash != hash {
		return false, nil
	}
	if err := checkpoint.Runtime.Validate(chain.Config().ChainID); err != nil {
		return false, nil
	}

	header := chain.GetHeader(hash, number)
	if header == nil || header.Number == nil || !header.Number.IsUint64() ||
		header.Number.Uint64() != number || header.Hash() != hash {
		return false, nil
	}

	if l.consensusLivenessV3Active(number) {
		envelope, err := ValidateLQCHeaderExtraV4(
			number,
			MaxWorkTicketsPerBlockV1,
			header.Extra,
		)
		if err != nil {
			return false, nil
		}
		if envelope.WorkStateRoot != checkpoint.Runtime.StateRoot ||
			!checkpoint.HasClaimLedger ||
			checkpoint.ClaimLedger == nil {
			return false, nil
		}
		if err := checkpoint.ClaimLedger.Validate(); err != nil {
			return false, nil
		}
		claimRoot, err := checkpoint.ClaimLedger.Root()
		if err != nil || claimRoot != envelope.CommitteeClaimRoot {
			return false, nil
		}
	} else {
		envelope, err := DecodeLQCHeaderExtraV3(
			header.Extra,
			MaxWorkTicketsPerBlockV1,
		)
		if err != nil || envelope.BlockNumber != number ||
			envelope.WorkStateRoot != checkpoint.Runtime.StateRoot {
			return false, nil
		}
	}

	state, err := workV1EngineLabRuntimeFor(l)
	if err != nil {
		return false, err
	}
	state.mu.Lock()
	state.runtimes[hash] = checkpoint.Runtime
	if checkpoint.HasClaimLedger && checkpoint.ClaimLedger != nil {
		state.claimLedgers[hash] = checkpoint.ClaimLedger.clone()
	}
	state.mu.Unlock()

	log.Info("Restored canonical LQC recovery checkpoint",
		"number", number, "hash", hash)
	return true, nil
}
