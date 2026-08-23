//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/crypto"
)

func workV1EngineProviderCandidateLab(
	epoch uint64,
	nonce uint64,
	label string,
) lqc.WorkCommitCandidateV1 {
	signature := make([]byte, 65)
	copy(signature, crypto.Keccak256([]byte("sig-"+label)))
	signature[64] = byte(nonce & 1)

	return lqc.WorkCommitCandidateV1{
		Signed: lqc.SignedRandomXWorkTicketV1{
			Ticket: lqc.RandomXWorkTicketV1{
				Version: lqc.RandomXWorkProtocolVersion,
				Epoch:   epoch,
				Participant: common.BytesToAddress(
					crypto.Keccak256(
						[]byte("participant-" + label),
					)[12:],
				),
				Nonce: nonce,
			},
			Signature: signature,
		},
		ProofHash: crypto.Keccak256Hash(
			[]byte("proof-" + label),
		),
	}
}

func TestWorkV1EnginePoolProviderLabSameBlockIsIdempotent(
	t *testing.T,
) {
	a := workV1EngineProviderCandidateLab(1, 1, "a")
	b := workV1EngineProviderCandidateLab(1, 2, "b")

	pendingCalls := 0
	provider := &workV1EnginePoolProviderLab{
		pending: func() ([]lqc.WorkCommitCandidateV1, error) {
			pendingCalls++
			return []lqc.WorkCommitCandidateV1{a, b}, nil
		},
		includedAt: func(
			uint64,
		) ([]lqc.SignedRandomXWorkTicketV1, bool) {
			return nil, false
		},
		removeIncluded: func([]common.Hash) uint64 {
			return 0
		},
	}

	first, err := provider.provide(129, 1)
	if err != nil {
		t.Fatal(err)
	}
	second, err := provider.provide(129, 1)
	if err != nil {
		t.Fatal(err)
	}

	if pendingCalls != 1 {
		t.Fatalf("pending calls=%d want=1", pendingCalls)
	}
	if len(first) != 2 || len(second) != 2 {
		t.Fatalf(
			"ticket lengths first=%d second=%d",
			len(first),
			len(second),
		)
	}
	for i := range first {
		if !workV1EnginePoolSignedEqualLab(
			first[i],
			second[i],
		) {
			t.Fatalf("reservation changed at index %d", i)
		}
	}
}

func TestWorkV1EnginePoolProviderLabRemovesOnlyCanonicalIncluded(
	t *testing.T,
) {
	a := workV1EngineProviderCandidateLab(1, 1, "a")
	b := workV1EngineProviderCandidateLab(1, 2, "b")

	removed := make(map[common.Hash]bool)
	provider := &workV1EnginePoolProviderLab{
		pending: func() ([]lqc.WorkCommitCandidateV1, error) {
			out := make([]lqc.WorkCommitCandidateV1, 0, 2)
			if !removed[a.ProofHash] {
				out = append(out, a)
			}
			if !removed[b.ProofHash] {
				out = append(out, b)
			}
			return out, nil
		},
		includedAt: func(
			blockNumber uint64,
		) ([]lqc.SignedRandomXWorkTicketV1, bool) {
			if blockNumber != 129 {
				return nil, false
			}
			return []lqc.SignedRandomXWorkTicketV1{
				a.Signed,
			}, true
		},
		removeIncluded: func(
			hashes []common.Hash,
		) uint64 {
			var count uint64
			for _, hash := range hashes {
				if !removed[hash] {
					removed[hash] = true
					count++
				}
			}
			return count
		},
	}

	first, err := provider.provide(129, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 2 {
		t.Fatalf("first tickets=%d want=2", len(first))
	}

	second, err := provider.provide(130, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !removed[a.ProofHash] {
		t.Fatal("canonical included candidate A was not removed")
	}
	if removed[b.ProofHash] {
		t.Fatal("unconfirmed candidate B was incorrectly removed")
	}
	if len(second) != 1 ||
		!workV1EnginePoolSignedEqualLab(
			second[0],
			b.Signed,
		) {
		t.Fatal("unconfirmed candidate B did not remain pending")
	}
}

func TestWorkV1EnginePoolProviderLabAbandonedBlockKeepsTickets(
	t *testing.T,
) {
	a := workV1EngineProviderCandidateLab(1, 1, "a")
	removeCalls := 0

	provider := &workV1EnginePoolProviderLab{
		pending: func() ([]lqc.WorkCommitCandidateV1, error) {
			return []lqc.WorkCommitCandidateV1{a}, nil
		},
		includedAt: func(
			uint64,
		) ([]lqc.SignedRandomXWorkTicketV1, bool) {
			// The reserved payload never became canonical.
			return nil, false
		},
		removeIncluded: func([]common.Hash) uint64 {
			removeCalls++
			return 0
		},
	}

	if _, err := provider.provide(129, 1); err != nil {
		t.Fatal(err)
	}
	next, err := provider.provide(130, 1)
	if err != nil {
		t.Fatal(err)
	}

	if removeCalls != 0 {
		t.Fatalf("remove calls=%d want=0", removeCalls)
	}
	if len(next) != 1 ||
		!workV1EnginePoolSignedEqualLab(
			next[0],
			a.Signed,
		) {
		t.Fatal("abandoned ticket did not remain available")
	}
}

func TestWorkV1EnginePoolProviderLabReadmitsRemovedAfterReorg(
	t *testing.T,
) {
	a := workV1EngineProviderCandidateLab(1, 1, "a")
	removed := false
	reorged := false
	readmitCalls := 0

	provider := &workV1EnginePoolProviderLab{
		pending: func() ([]lqc.WorkCommitCandidateV1, error) {
			if removed {
				return nil, nil
			}
			return []lqc.WorkCommitCandidateV1{a}, nil
		},
		includedAt: func(
			blockNumber uint64,
		) ([]lqc.SignedRandomXWorkTicketV1, bool) {
			if blockNumber != 129 {
				return nil, false
			}
			if reorged {
				// A replacement block is canonical at the same height,
				// but it does not contain candidate A.
				return nil, true
			}
			return []lqc.SignedRandomXWorkTicketV1{
				a.Signed,
			}, true
		},
		removeIncluded: func(
			hashes []common.Hash,
		) uint64 {
			if len(hashes) == 1 &&
				hashes[0] == a.ProofHash &&
				!removed {
				removed = true
				return 1
			}
			return 0
		},
		readmitRemoved: func(
			candidate lqc.WorkCommitCandidateV1,
		) bool {
			if candidate.ProofHash != a.ProofHash ||
				!workV1EnginePoolSignedEqualLab(
					candidate.Signed,
					a.Signed,
				) {
				t.Fatal("provider tried to readmit the wrong ticket")
			}
			readmitCalls++
			removed = false
			return true
		},
	}

	first, err := provider.provide(129, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 1 {
		t.Fatalf("first tickets=%d want=1", len(first))
	}

	second, err := provider.provide(130, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !removed || len(second) != 0 {
		t.Fatal("canonical ticket was not quarantined after removal")
	}
	if readmitCalls != 0 {
		t.Fatal("canonical ticket was readmitted before a reorg")
	}

	third, err := provider.provide(131, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !removed || len(third) != 0 || readmitCalls != 0 {
		t.Fatal("canonical removal did not remain quarantined")
	}

	reorged = true
	fourth, err := provider.provide(132, 1)
	if err != nil {
		t.Fatal(err)
	}
	if removed || readmitCalls != 1 {
		t.Fatal("reorged ticket was not readmitted exactly once")
	}
	if len(fourth) != 1 ||
		!workV1EnginePoolSignedEqualLab(fourth[0], a.Signed) {
		t.Fatal("readmitted ticket did not return to pending")
	}
}

func TestWorkV1EnginePoolProviderLabDoesNotReadmitExpiredEpoch(
	t *testing.T,
) {
	a := workV1EngineProviderCandidateLab(1, 1, "a")
	removed := false
	reorged := false
	readmitCalls := 0

	provider := &workV1EnginePoolProviderLab{
		pending: func() ([]lqc.WorkCommitCandidateV1, error) {
			if removed {
				return nil, nil
			}
			return []lqc.WorkCommitCandidateV1{a}, nil
		},
		includedAt: func(
			blockNumber uint64,
		) ([]lqc.SignedRandomXWorkTicketV1, bool) {
			if blockNumber == 129 && !reorged {
				return []lqc.SignedRandomXWorkTicketV1{
					a.Signed,
				}, true
			}
			return nil, false
		},
		removeIncluded: func(
			hashes []common.Hash,
		) uint64 {
			if len(hashes) == 1 &&
				hashes[0] == a.ProofHash &&
				!removed {
				removed = true
				return 1
			}
			return 0
		},
		readmitRemoved: func(
			lqc.WorkCommitCandidateV1,
		) bool {
			readmitCalls++
			return true
		},
	}

	if _, err := provider.provide(129, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.provide(130, 1); err != nil {
		t.Fatal(err)
	}
	if !removed {
		t.Fatal("canonical ticket was not removed")
	}

	reorged = true
	if _, err := provider.provide(257, 2); err != nil {
		t.Fatal(err)
	}
	if readmitCalls != 0 {
		t.Fatal("expired epoch ticket was incorrectly readmitted")
	}
}

func TestWorkV1EnginePoolProviderLabHardCapsEight(
	t *testing.T,
) {
	all := make([]lqc.WorkCommitCandidateV1, 0, 12)
	for i := uint64(0); i < 12; i++ {
		all = append(
			all,
			workV1EngineProviderCandidateLab(
				1,
				i+1,
				string(rune('a'+i)),
			),
		)
	}

	provider := &workV1EnginePoolProviderLab{
		pending: func() ([]lqc.WorkCommitCandidateV1, error) {
			return all, nil
		},
		includedAt: func(
			uint64,
		) ([]lqc.SignedRandomXWorkTicketV1, bool) {
			return nil, false
		},
		removeIncluded: func([]common.Hash) uint64 {
			return 0
		},
	}

	got, err := provider.provide(129, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != int(lqc.MaxWorkTicketsPerBlockV1) {
		t.Fatalf(
			"tickets=%d want=%d",
			len(got),
			lqc.MaxWorkTicketsPerBlockV1,
		)
	}
}

func TestWorkV1EnginePoolProviderLabRestartRemovesCanonicalIncluded(
	t *testing.T,
) {
	a := workV1EngineProviderCandidateLab(1, 1, "restart-a")
	b := workV1EngineProviderCandidateLab(1, 2, "restart-b")
	pool := map[common.Hash]lqc.WorkCommitCandidateV1{
		a.ProofHash: a,
		b.ProofHash: b,
	}

	// This is a fresh provider: it has no in-memory reservation from the
	// process that built block 129. Its persistent pool still contains both
	// candidates, while canonical block 129 already contains candidate A.
	provider := &workV1EnginePoolProviderLab{
		epochLength: 128,
		pending: func() ([]lqc.WorkCommitCandidateV1, error) {
			out := make([]lqc.WorkCommitCandidateV1, 0, len(pool))
			for _, candidate := range pool {
				out = append(out, candidate)
			}
			return out, nil
		},
		includedAt: func(
			blockNumber uint64,
		) ([]lqc.SignedRandomXWorkTicketV1, bool) {
			if blockNumber == 129 {
				return []lqc.SignedRandomXWorkTicketV1{a.Signed}, true
			}
			return nil, true
		},
		removeIncluded: func(hashes []common.Hash) uint64 {
			var removed uint64
			for _, hash := range hashes {
				if _, ok := pool[hash]; ok {
					delete(pool, hash)
					removed++
				}
			}
			return removed
		},
	}

	got, err := provider.provide(130, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := pool[a.ProofHash]; exists {
		t.Fatal("canonical ticket survived restart reconciliation")
	}
	if _, exists := pool[b.ProofHash]; !exists {
		t.Fatal("pending ticket was incorrectly removed after restart")
	}
	if len(got) != 1 ||
		!workV1EnginePoolSignedEqualLab(got[0], b.Signed) {
		t.Fatal("provider did not return only the still-pending ticket")
	}
}

func TestWorkV1EnginePoolProviderLabCanonicalReplayLookup(
	t *testing.T,
) {
	included := workV1EngineProviderCandidateLab(
		18,
		1,
		"canonical-replay",
	)
	other := workV1EngineProviderCandidateLab(
		18,
		2,
		"other-ticket",
	)

	provider := &workV1EnginePoolProviderLab{
		includedAt: func(
			blockNumber uint64,
		) ([]lqc.SignedRandomXWorkTicketV1, bool) {
			if blockNumber == 2306 {
				return []lqc.SignedRandomXWorkTicketV1{
					included.Signed,
				}, true
			}
			return nil, true
		},
		epochLength: 128,
	}

	if !provider.canonicalIncludes(2322, 18, included) {
		t.Fatal("canonical epoch-18 replay was not detected")
	}
	if provider.canonicalIncludes(2322, 18, other) {
		t.Fatal("different signed ticket was rejected as canonical")
	}
	if provider.canonicalIncludes(2322, 19, included) {
		t.Fatal("ticket from another commit epoch was accepted")
	}
}
