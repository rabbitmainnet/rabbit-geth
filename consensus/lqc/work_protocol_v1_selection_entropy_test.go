package lqc

import (
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

func TestCanonicalWorkSelectionEntropyV1LowestProofWins(t *testing.T) {
	a := common.HexToHash("0x10")
	b := common.HexToHash("0x20")
	c := common.HexToHash("0x30")

	got, err := CanonicalWorkSelectionEntropyV1([]common.Hash{c, a, b})
	if err != nil {
		t.Fatal(err)
	}
	if got != a {
		t.Fatalf("entropy=%s want=%s", got, a)
	}

	same, err := CanonicalWorkSelectionEntropyV1([]common.Hash{b, c, a})
	if err != nil {
		t.Fatal(err)
	}
	if same != got {
		t.Fatal("arrival order changed entropy")
	}

	withHigher, err := CanonicalWorkSelectionEntropyV1([]common.Hash{a, b})
	if err != nil {
		t.Fatal(err)
	}
	if withHigher != a {
		t.Fatal("higher proof changed entropy")
	}

	if _, err := CanonicalWorkSelectionEntropyV1([]common.Hash{a, a}); !errors.Is(err, ErrDuplicateWorkSelectionEntropyProofV1) {
		t.Fatalf("duplicate error=%v", err)
	}
}

func TestWorkSelectionSeedV1BindsContext(t *testing.T) {
	chain := big.NewInt(928)
	root := crypto.Keccak256Hash([]byte("root"))
	ent := crypto.Keccak256Hash([]byte("entropy"))

	base, err := WorkSelectionSeedV1(chain, 7, root, ent, 1000)
	if err != nil {
		t.Fatal(err)
	}

	other, err := WorkSelectionSeedV1(chain, 7, root, ent, 1001)
	if err != nil {
		t.Fatal(err)
	}
	if base == other {
		t.Fatal("block number did not bind seed")
	}

	other, err = WorkSelectionSeedV1(
		chain, 7, root, crypto.Keccak256Hash([]byte("other")), 1000,
	)
	if err != nil {
		t.Fatal(err)
	}
	if base == other {
		t.Fatal("entropy did not bind seed")
	}

	_, err = WorkSelectionSeedV1(chain, 7, root, common.Hash{}, 1000)
	if !errors.Is(err, ErrInvalidWorkSelectionEntropyV1) {
		t.Fatalf("missing entropy error=%v", err)
	}
}
