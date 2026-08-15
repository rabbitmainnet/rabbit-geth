package lqc

import (
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rlp"
)

var ErrInvalidWorkSelectionBeaconV1 = errors.New(
	"invalid lqc work selection beacon v1",
)

var workSelectionBeaconDomainV1 = []byte(
	"RABBIT-LQC-WORK-SELECTION-BEACON-V1",
)

type WorkSelectionBeaconHasherV1 func(
	datasetKey common.Hash,
	input []byte,
) (common.Hash, error)

type workSelectionBeaconPayloadV1 struct {
	Domain        []byte
	Version       uint8
	ChainID       *big.Int
	SourceEpoch   uint64
	SelectionRoot common.Hash
	DatasetKey    common.Hash
}

// WorkSelectionBeaconInputV1 derives the only RandomX input used after a work
// epoch is fully closed. There is no separately relayed beacon proof and no
// producer-chosen nonce: every node recomputes the same entropy locally.
//
// This removes beacon-level proof omission. It does NOT solve censorship or
// strategic withholding of work tickets before SelectionRoot becomes final.
func WorkSelectionBeaconInputV1(
	chainID *big.Int,
	sourceEpoch uint64,
	selectionRoot common.Hash,
	datasetKey common.Hash,
) ([]byte, error) {
	if chainID == nil ||
		chainID.Sign() <= 0 ||
		sourceEpoch == 0 ||
		selectionRoot == (common.Hash{}) ||
		datasetKey == (common.Hash{}) {
		return nil, ErrInvalidWorkSelectionBeaconV1
	}

	return rlp.EncodeToBytes(workSelectionBeaconPayloadV1{
		Domain:        workSelectionBeaconDomainV1,
		Version:       RandomXWorkProtocolVersion,
		ChainID:       new(big.Int).Set(chainID),
		SourceEpoch:   sourceEpoch,
		SelectionRoot: selectionRoot,
		DatasetKey:    datasetKey,
	})
}

// DeriveWorkSelectionEntropyV1 performs exactly one deterministic RandomX
// evaluation. Extra repeated RandomX rounds would add verifier latency without
// providing VDF-style non-parallelizability.
func DeriveWorkSelectionEntropyV1(
	chainID *big.Int,
	sourceEpoch uint64,
	selectionRoot common.Hash,
	datasetKey common.Hash,
	hasher WorkSelectionBeaconHasherV1,
) (common.Hash, error) {
	if hasher == nil {
		return common.Hash{}, ErrInvalidWorkSelectionBeaconV1
	}
	input, err := WorkSelectionBeaconInputV1(
		chainID,
		sourceEpoch,
		selectionRoot,
		datasetKey,
	)
	if err != nil {
		return common.Hash{}, err
	}
	entropy, err := hasher(datasetKey, input)
	if err != nil {
		return common.Hash{}, err
	}
	if entropy == (common.Hash{}) {
		return common.Hash{}, ErrInvalidWorkSelectionBeaconV1
	}
	return entropy, nil
}

// DeterministicWorkSelectionSeedV1 is the complete inactive foundation path:
// closed work root -> local RandomX entropy -> seat selection seed.
func DeterministicWorkSelectionSeedV1(
	chainID *big.Int,
	sourceEpoch uint64,
	selectionRoot common.Hash,
	datasetKey common.Hash,
	blockNumber uint64,
	hasher WorkSelectionBeaconHasherV1,
) (common.Hash, common.Hash, error) {
	entropy, err := DeriveWorkSelectionEntropyV1(
		chainID,
		sourceEpoch,
		selectionRoot,
		datasetKey,
		hasher,
	)
	if err != nil {
		return common.Hash{}, common.Hash{}, err
	}
	seed, err := WorkSelectionSeedV1(
		chainID,
		sourceEpoch,
		selectionRoot,
		entropy,
		blockNumber,
	)
	if err != nil {
		return common.Hash{}, common.Hash{}, err
	}
	return entropy, seed, nil
}
