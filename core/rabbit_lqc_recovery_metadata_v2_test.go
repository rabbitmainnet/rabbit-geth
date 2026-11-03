package core

import (
	"encoding/binary"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
)

func TestLQCRecoveryMetadataV2Validation(t *testing.T) {
	target := common.HexToHash("0x1234")
	other := common.HexToHash("0x5678")

	for _, tc := range []struct {
		name      string
		v1        bool
		v2        []byte
		wantError bool
		wantHash  common.Hash
	}{
		{"empty", false, nil, false, common.Hash{}},
		{"legacy", true, nil, false, target},
		{"orphan", false, makeMetadataV2(target, 5), true, common.Hash{}},
		{"valid", true, makeMetadataV2(target, 5), false, target},
		{"wrong hash", true, makeMetadataV2(other, 5), true, common.Hash{}},
		{"zero height", true, makeMetadataV2(target, 0), true, common.Hash{}},
		{"short metadata", true, []byte{1, 2}, true, common.Hash{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := rawdb.NewMemoryDatabase()
			defer db.Close()

			if tc.v1 {
				if err := db.Put(lqcLocalRecoveryKeyV1, target.Bytes()); err != nil {
					t.Fatal(err)
				}
			}
			if tc.v2 != nil {
				if err := db.Put(lqcLocalRecoveryMetadataKeyV2, tc.v2); err != nil {
					t.Fatal(err)
				}
			}
			got, err := ReadLQCLocalStateRecoveryV1(db)
			if (err != nil) != tc.wantError || got != tc.wantHash {
				t.Fatalf("got=%s err=%v want=%s error=%v",
					got, err, tc.wantHash, tc.wantError)
			}
		})
	}

	db := rawdb.NewMemoryDatabase()
	defer db.Close()
	if err := db.Put(lqcLocalRecoveryKeyV1, target.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := db.Put(lqcLocalRecoveryMetadataKeyV2, makeMetadataV2(target, 5)); err != nil {
		t.Fatal(err)
	}
	rawdb.WriteHeaderNumber(db, target, 6)
	if _, err := ReadLQCLocalStateRecoveryV1(db); err == nil {
		t.Fatal("conflicting stored height accepted")
	}
}

func makeMetadataV2(hash common.Hash, number uint64) []byte {
	b := make([]byte, common.HashLength+8)
	copy(b, hash[:])
	binary.BigEndian.PutUint64(b[common.HashLength:], number)
	return b
}

func TestLQCRecoveryMetadataV2Lifecycle(t *testing.T) {
	db := rawdb.NewMemoryDatabase()
	defer db.Close()

	target := common.HexToHash("0xabcd")
	rawdb.WriteHeadBlockHash(db, target)
	rawdb.WriteHeaderNumber(db, target, 123)

	prepared, err := PrepareLQCLocalStateRecoveryV1(db)
	if err != nil || prepared != target {
		t.Fatalf("prepare: target=%s err=%v", prepared, err)
	}

	metadata, err := db.Get(lqcLocalRecoveryMetadataKeyV2)
	if err != nil || len(metadata) != common.HashLength+8 {
		t.Fatalf("V2 metadata missing: %v", err)
	}

	if binary.BigEndian.Uint64(metadata[common.HashLength:]) != 123 {
		t.Fatal("incorrect V2 block number")
	}

	read, err := ReadLQCLocalStateRecoveryV1(db)
	if err != nil || read != target {
		t.Fatalf("read: hash=%s err=%v", read, err)
	}

	bc := &BlockChain{db: db}
	if err := bc.clearLQCLocalRecoveryV2(); err != nil {
		t.Fatal(err)
	}

	for _, key := range [][]byte{
		lqcLocalRecoveryKeyV1,
		lqcLocalRecoveryMetadataKeyV2,
	} {
		exists, err := db.Has(key)
		if err != nil || exists {
			t.Fatalf("marker not cleared: %q err=%v", key, err)
		}
	}

	read, err = ReadLQCLocalStateRecoveryV1(db)
	if err != nil || read != (common.Hash{}) {
		t.Fatalf("unexpected pending recovery: %s %v", read, err)
	}
}
