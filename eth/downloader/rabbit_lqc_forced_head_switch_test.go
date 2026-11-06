package downloader

import (
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/types"
)

// Verify that a conflicting head cannot silently replace the current
// skeleton through BeaconExtend, but can be requested by a forced sync.
func TestLQCForcedConflictingHeadSwitchV1(t *testing.T) {
	db := rawdb.NewMemoryDatabase()
	defer db.Close()

	genesis := &types.Header{
		Number: big.NewInt(0),
		Extra:  []byte("rabbit-genesis"),
	}
	a := &types.Header{
		Number:     big.NewInt(4),
		ParentHash: genesis.Hash(),
		Extra:      []byte("fork-A"),
	}
	b := types.CopyHeader(a)
	b.Extra = []byte("fork-B")

	rawdb.WriteHeader(db, genesis)
	rawdb.WriteCanonicalHash(db, genesis.Hash(), 0)
	rawdb.WriteBody(db, genesis.Hash(), 0, &types.Body{})
	rawdb.WriteReceipts(db, genesis.Hash(), 0, types.Receipts{})

	sk := newSkeleton(
		db,
		newPeerSet(),
		func(string) {},
		newHookedBackfiller(),
		&lqcHashTestReader{},
	)
	defer sk.Terminate()

	if err := sk.Sync(a, nil, true); err != nil {
		t.Fatalf("initial forced head: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for {
		stored := rawdb.ReadSkeletonHeader(db, 4)
		if stored != nil && stored.Hash() == a.Hash() {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("initial head not stored")
		}
		time.Sleep(10 * time.Millisecond)
	}

	if err := sk.Sync(b, nil, false); err == nil {
		t.Fatal("incompatible extension unexpectedly accepted")
	}

	if err := sk.Sync(b, nil, true); err != nil {
		t.Fatalf("forced replacement rejected: %v", err)
	}

	deadline = time.Now().Add(3 * time.Second)
	for {
		stored := rawdb.ReadSkeletonHeader(db, 4)
		if stored != nil && stored.Hash() == b.Hash() {
			t.Log("FORCED_HEAD_REPLACEMENT_CONFIRMED")
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("forced replacement did not install fork B")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
