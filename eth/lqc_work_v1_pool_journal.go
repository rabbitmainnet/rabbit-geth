package eth

import (
	"errors"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/rlp"
)

const lqcWorkV1PoolJournalVersion = uint8(1)

var errLQCWorkV1PoolJournal = errors.New(
	"invalid lqc Work V1 pool journal",
)

var lqcWorkV1PoolJournalPrefix = []byte(
	"rabbit/lqcw/v1/verified-pending/",
)

type lqcWorkV1PoolJournalRecord struct {
	Version    uint8
	Genesis    common.Hash
	Epoch      uint64
	Candidates []lqc.WorkCommitCandidateV1
}

type lqcWorkV1PoolJournal struct {
	db      ethdb.KeyValueStore
	genesis common.Hash
	key     []byte
}

func newLQCWorkV1PoolJournal(
	database ethdb.KeyValueStore,
	genesis common.Hash,
) (*lqcWorkV1PoolJournal, error) {
	if database == nil || genesis == (common.Hash{}) {
		return nil, errLQCWorkV1PoolJournal
	}
	key := make(
		[]byte,
		0,
		len(lqcWorkV1PoolJournalPrefix)+len(genesis),
	)
	key = append(key, lqcWorkV1PoolJournalPrefix...)
	key = append(key, genesis[:]...)
	return &lqcWorkV1PoolJournal{
		db:      database,
		genesis: genesis,
		key:     key,
	}, nil
}

func cloneLQCWorkV1PoolJournalCandidates(
	input []lqc.WorkCommitCandidateV1,
) []lqc.WorkCommitCandidateV1 {
	out := make(
		[]lqc.WorkCommitCandidateV1,
		0,
		len(input),
	)
	for _, candidate := range input {
		cloned := candidate
		cloned.Signed.Signature = append(
			[]byte(nil),
			candidate.Signed.Signature...,
		)
		out = append(out, cloned)
	}
	return out
}

func (j *lqcWorkV1PoolJournal) LoadWorkCommitPoolV1(
	epoch uint64,
) ([]lqc.WorkCommitCandidateV1, error) {
	if j == nil || j.db == nil || epoch == 0 {
		return nil, errLQCWorkV1PoolJournal
	}

	exists, err := j.db.Has(j.key)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, nil
	}
	blob, err := j.db.Get(j.key)
	if err != nil {
		return nil, err
	}

	var record lqcWorkV1PoolJournalRecord
	if err := rlp.DecodeBytes(blob, &record); err != nil {
		return nil, err
	}
	if record.Version != lqcWorkV1PoolJournalVersion ||
		record.Genesis != j.genesis ||
		record.Epoch == 0 {
		return nil, errLQCWorkV1PoolJournal
	}
	if record.Epoch != epoch {
		return nil, nil
	}
	return cloneLQCWorkV1PoolJournalCandidates(
		record.Candidates,
	), nil
}

func (j *lqcWorkV1PoolJournal) StoreWorkCommitPoolV1(
	epoch uint64,
	candidates []lqc.WorkCommitCandidateV1,
) error {
	if j == nil || j.db == nil || epoch == 0 {
		return errLQCWorkV1PoolJournal
	}

	blob, err := rlp.EncodeToBytes(lqcWorkV1PoolJournalRecord{
		Version:    lqcWorkV1PoolJournalVersion,
		Genesis:    j.genesis,
		Epoch:      epoch,
		Candidates: cloneLQCWorkV1PoolJournalCandidates(candidates),
	})
	if err != nil {
		return err
	}
	if err := j.db.Put(j.key, blob); err != nil {
		return err
	}
	return j.db.SyncKeyValue()
}
