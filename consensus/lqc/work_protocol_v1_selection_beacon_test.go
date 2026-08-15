package lqc

import (
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

func fakeSelectionBeaconHasherV1(
	datasetKey common.Hash,
	input []byte,
) (common.Hash, error) {
	if datasetKey == (common.Hash{}) || len(input) == 0 {
		return common.Hash{}, ErrInvalidWorkSelectionBeaconV1
	}
	return crypto.Keccak256Hash(datasetKey.Bytes(), input), nil
}

func selectionBeaconContextV1() (*big.Int, uint64, common.Hash, common.Hash) {
	return big.NewInt(928),
		7,
		crypto.Keccak256Hash([]byte("closed-work-root")),
		crypto.Keccak256Hash([]byte("randomx-dataset-key"))
}

func TestWorkSelectionBeaconV1DeterministicOneHash(t *testing.T) {
	chainID, epoch, root, key := selectionBeaconContextV1()

	calls := 0
	hasher := func(datasetKey common.Hash, input []byte) (common.Hash, error) {
		calls++
		return crypto.Keccak256Hash(datasetKey.Bytes(), input), nil
	}

	first, err := DeriveWorkSelectionEntropyV1(chainID, epoch, root, key, hasher)
	if err != nil {
		t.Fatal(err)
	}
	second, err := DeriveWorkSelectionEntropyV1(chainID, epoch, root, key, hasher)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("same closed root produced different entropy")
	}
	if calls != 2 {
		t.Fatalf("hasher calls=%d want=2", calls)
	}
}

func TestWorkSelectionBeaconV1ClosedRootChangesEntropy(t *testing.T) {
	chainID, epoch, root, key := selectionBeaconContextV1()

	left, err := DeriveWorkSelectionEntropyV1(
		chainID, epoch, root, key, fakeSelectionBeaconHasherV1,
	)
	if err != nil {
		t.Fatal(err)
	}
	right, err := DeriveWorkSelectionEntropyV1(
		chainID,
		epoch,
		crypto.Keccak256Hash([]byte("different-root")),
		key,
		fakeSelectionBeaconHasherV1,
	)
	if err != nil {
		t.Fatal(err)
	}
	if left == right {
		t.Fatal("different closed roots produced same entropy")
	}
}

func TestDeterministicWorkSelectionSeedV1(t *testing.T) {
	chainID, epoch, root, key := selectionBeaconContextV1()

	entropyA, seedA, err := DeterministicWorkSelectionSeedV1(
		chainID, epoch, root, key, 1000, fakeSelectionBeaconHasherV1,
	)
	if err != nil {
		t.Fatal(err)
	}
	entropyB, seedB, err := DeterministicWorkSelectionSeedV1(
		chainID, epoch, root, key, 1000, fakeSelectionBeaconHasherV1,
	)
	if err != nil {
		t.Fatal(err)
	}
	if entropyA != entropyB || seedA != seedB {
		t.Fatal("deterministic beacon/seed path diverged")
	}

	_, seedNext, err := DeterministicWorkSelectionSeedV1(
		chainID, epoch, root, key, 1001, fakeSelectionBeaconHasherV1,
	)
	if err != nil {
		t.Fatal(err)
	}
	if seedNext == seedA {
		t.Fatal("block number did not bind final selection seed")
	}
}

func TestWorkSelectionBeaconV1RejectsInvalidContext(t *testing.T) {
	chainID, epoch, root, key := selectionBeaconContextV1()

	if _, err := DeriveWorkSelectionEntropyV1(
		chainID, epoch, root, key, nil,
	); !errors.Is(err, ErrInvalidWorkSelectionBeaconV1) {
		t.Fatalf("nil hasher error=%v", err)
	}
	if _, err := DeriveWorkSelectionEntropyV1(
		chainID, epoch, common.Hash{}, key, fakeSelectionBeaconHasherV1,
	); !errors.Is(err, ErrInvalidWorkSelectionBeaconV1) {
		t.Fatalf("zero root error=%v", err)
	}
}
