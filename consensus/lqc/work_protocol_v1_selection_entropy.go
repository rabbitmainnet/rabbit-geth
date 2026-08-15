package lqc

import (
	"bytes"
	"errors"
	"math/big"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
)

var (
	ErrInvalidWorkSelectionEntropyV1        = errors.New("invalid lqc work selection entropy v1")
	ErrDuplicateWorkSelectionEntropyProofV1 = errors.New("duplicate lqc work selection entropy proof v1")
)

var workSelectionSeedDomainV1 = []byte("RABBIT-LQC-WORK-SELECTION-SEED-V1")

type workSelectionSeedPayloadV1 struct {
	Domain        []byte
	Version       uint8
	ChainID       *big.Int
	SourceEpoch   uint64
	SelectionRoot common.Hash
	Entropy       common.Hash
	BlockNumber   uint64
}

// Input hashes MUST already be verified RandomX beacon proofs.
// The lowest verified proof is canonical. Replacing it requires a strictly
// lower valid proof; free mutable-header variants do not change this entropy.
func CanonicalWorkSelectionEntropyV1(hashes []common.Hash) (common.Hash, error) {
	if len(hashes) == 0 {
		return common.Hash{}, ErrInvalidWorkSelectionEntropyV1
	}
	proofs := append([]common.Hash(nil), hashes...)
	sort.Slice(proofs, func(i, j int) bool {
		return bytes.Compare(proofs[i][:], proofs[j][:]) < 0
	})
	for i, proof := range proofs {
		if proof == (common.Hash{}) {
			return common.Hash{}, ErrInvalidWorkSelectionEntropyV1
		}
		if i > 0 && proof == proofs[i-1] {
			return common.Hash{}, ErrDuplicateWorkSelectionEntropyProofV1
		}
	}
	return proofs[0], nil
}

// WorkSelectionSeedV1 is intentionally parentHash-free.
func WorkSelectionSeedV1(
	chainID *big.Int,
	sourceEpoch uint64,
	selectionRoot common.Hash,
	entropy common.Hash,
	blockNumber uint64,
) (common.Hash, error) {
	if chainID == nil || chainID.Sign() <= 0 || sourceEpoch == 0 ||
		selectionRoot == (common.Hash{}) || entropy == (common.Hash{}) || blockNumber == 0 {
		return common.Hash{}, ErrInvalidWorkSelectionEntropyV1
	}
	blob, err := rlp.EncodeToBytes(workSelectionSeedPayloadV1{
		Domain:        workSelectionSeedDomainV1,
		Version:       RandomXWorkProtocolVersion,
		ChainID:       new(big.Int).Set(chainID),
		SourceEpoch:   sourceEpoch,
		SelectionRoot: selectionRoot,
		Entropy:       entropy,
		BlockNumber:   blockNumber,
	})
	if err != nil {
		return common.Hash{}, err
	}
	return crypto.Keccak256Hash(blob), nil
}
