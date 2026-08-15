package eth

import (
	"bytes"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
)

func TestLQCWorkV1PoolJournalPersistsPendingAcrossRestart(
	t *testing.T,
) {
	database := rawdb.NewMemoryDatabase()
	defer database.Close()

	config := testWorkV1TransportConfig()
	journal, err := newLQCWorkV1PoolJournal(
		database,
		config.Genesis,
	)
	if err != nil {
		t.Fatal(err)
	}
	config.PoolPersistence = journal
	first, err := newLQCWorkV1Transport(config)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.currentContext(); err != nil {
		t.Fatal(err)
	}

	candidate := signedWorkV1TransportCandidate(
		t,
		testWorkV1ChallengeAnchor(),
		17,
	)
	if err := first.pool.AddVerifiedV1(candidate); err != nil {
		t.Fatal(err)
	}
	first.Close()

	restartedJournal, err := newLQCWorkV1PoolJournal(
		database,
		config.Genesis,
	)
	if err != nil {
		t.Fatal(err)
	}
	restartConfig := testWorkV1TransportConfig()
	restartConfig.PoolPersistence = restartedJournal
	restarted, err := newLQCWorkV1Transport(restartConfig)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.currentContext(); err != nil {
		t.Fatal(err)
	}
	pending, err := restarted.pool.PendingV1()
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 ||
		pending[0].ProofHash != candidate.ProofHash ||
		pending[0].Signed.Ticket != candidate.Signed.Ticket ||
		!bytes.Equal(
			pending[0].Signed.Signature,
			candidate.Signed.Signature,
		) {
		t.Fatal("verified pending candidate was not restored exactly")
	}

	if removed := restarted.pool.RemoveIncludedV1(
		[]common.Hash{candidate.ProofHash},
	); removed != 1 {
		t.Fatalf("removed=%d want=1", removed)
	}
	restarted.Close()

	afterRemovalJournal, err := newLQCWorkV1PoolJournal(
		database,
		config.Genesis,
	)
	if err != nil {
		t.Fatal(err)
	}
	afterRemovalConfig := testWorkV1TransportConfig()
	afterRemovalConfig.PoolPersistence = afterRemovalJournal
	afterRemoval, err := newLQCWorkV1Transport(afterRemovalConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer afterRemoval.Close()
	if _, err := afterRemoval.currentContext(); err != nil {
		t.Fatal(err)
	}
	if afterRemoval.pool.LenV1() != 0 {
		t.Fatal("canonically removed candidate returned after restart")
	}

	if err := afterRemoval.pool.ResetCommitEpochV1(8); err != nil {
		t.Fatal(err)
	}
	if afterRemoval.pool.LenV1() != 0 {
		t.Fatal("old epoch pending leaked into the new epoch")
	}
}
