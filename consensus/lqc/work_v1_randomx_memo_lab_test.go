//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package lqc

import (
	"crypto/ecdsa"
	"errors"
	"math/big"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

func TestWorkV1RandomXMemoConcurrentIdentityAndClose(t *testing.T) {
	var calls atomic.Int32
	var closes atomic.Int32
	inner := func(epoch common.Hash, input []byte) (common.Hash, error) {
		calls.Add(1)
		return crypto.Keccak256Hash(epoch[:], input), nil
	}
	hash, close := newWorkV1MemoizedRandomXHasherV1(inner, func() { closes.Add(1) }, 8)
	epoch := common.HexToHash("0x01")
	expected := crypto.Keccak256Hash(epoch[:], []byte("proof"))
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := hash(epoch, []byte("proof"))
			if err != nil || got != expected {
				t.Error("memo changed computed hash")
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("same proof recalculated %d times", calls.Load())
	}
	if _, err := hash(epoch, []byte("changed proof")); err != nil {
		t.Fatal(err)
	}
	if _, err := hash(common.HexToHash("0x02"), []byte("proof")); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 3 {
		t.Fatal("different input or dataset reused cached hash")
	}
	close()
	close()
	if closes.Load() != 1 {
		t.Fatal("underlying hasher closed more than once")
	}
	if _, err := hash(epoch, []byte("proof")); !errors.Is(err, errWorkV1RandomXMemoClosed) {
		t.Fatal("closed memo accepted proof")
	}
}
func TestWorkV1RandomXMemoBoundAndFailures(t *testing.T) {
	calls := 0
	fail := true
	hash, close := newWorkV1MemoizedRandomXHasherV1(func(epoch common.Hash, input []byte) (common.Hash, error) {
		calls++
		if fail {
			return common.Hash{}, errors.New("failed RandomX")
		}
		return crypto.Keccak256Hash(input), nil
	}, nil, 2)
	defer close()
	epoch := common.HexToHash("0x01")
	for i := 0; i < 2; i++ {
		if _, err := hash(epoch, []byte("a")); err == nil {
			t.Fatal("failed evaluation accepted")
		}
	}
	if calls != 2 {
		t.Fatal("failure cached")
	}
	fail = false
	for _, input := range []string{"a", "b", "a", "c", "b"} {
		if _, err := hash(epoch, []byte(input)); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 6 {
		t.Fatalf("LRU bound incorrect: calls=%d", calls)
	}
}

func TestWorkV1RandomXMemoPreservesCommitteeSignatureChecks(t *testing.T) {
	chainID := big.NewInt(9280)
	datasetKey := crypto.Keccak256Hash([]byte("shared-epoch-dataset"))
	parentHash := crypto.Keccak256Hash([]byte("target-parent"))
	selectionRoot := crypto.Keccak256Hash([]byte("target-selection"))
	committee := make([]WorkSeatV1, 3)
	keys := make([]*ecdsa.PrivateKey, len(committee))
	for index := range committee {
		key, err := crypto.GenerateKey()
		if err != nil {
			t.Fatal(err)
		}
		keys[index] = key
		committee[index] = WorkSeatV1{
			TicketHash:  crypto.Keccak256Hash([]byte{byte(index + 1)}),
			Participant: crypto.PubkeyToAddress(key.PublicKey),
		}
	}
	hashCalls := 0
	hasher := func(key common.Hash, input []byte) (common.Hash, error) {
		if key != datasetKey {
			t.Fatalf("dataset key=%s want=%s", key, datasetKey)
		}
		hashCalls++
		return crypto.Keccak256Hash(key.Bytes(), input), nil
	}
	positions := []uint8{0, 2}
	group := CommitteeParticipationClaimGroupV1{
		TargetBlock:    100,
		Participations: make([]CompactCommitteeParticipationV1, len(positions)),
	}
	for index, position := range positions {
		seat := committee[position]
		proof := CommitteeParticipationV1{
			Version:       CommitteeParticipationVersionV1,
			BlockNumber:   group.TargetBlock,
			ParentHash:    parentHash,
			SelectionRoot: selectionRoot,
			TicketHash:    seat.TicketHash,
			Participant:   seat.Participant,
		}
		input, err := CommitteeParticipationInputV1(chainID, proof)
		if err != nil {
			t.Fatal(err)
		}
		proofHash, err := hasher(datasetKey, input)
		if err != nil {
			t.Fatal(err)
		}
		signingHash, err := CommitteeParticipationSigningHashV1(chainID, proof, proofHash)
		if err != nil {
			t.Fatal(err)
		}
		signature, err := crypto.Sign(signingHash[:], keys[position])
		if err != nil {
			t.Fatal(err)
		}
		group.Participations[index] = CompactCommitteeParticipationV1{
			Position:  position,
			Signature: signature,
		}
	}
	hashCalls = 0
	memoHash, closeMemo := newWorkV1MemoizedRandomXHasherV1(hasher, nil, 8)
	defer closeMemo()
	hasher = memoHash
	verified, err := VerifyCommitteeParticipationClaimGroupV1(
		CommitteeParticipationVerificationContextV1{
			ChainID:       chainID,
			DatasetKey:    datasetKey,
			ParentHash:    parentHash,
			SelectionRoot: selectionRoot,
			Committee:     committee,
			Hasher:        hasher,
		},
		group,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(verified) != len(positions) || hashCalls != len(positions) {
		t.Fatalf("verified=%d hashCalls=%d want=%d", len(verified), hashCalls, len(positions))
	}

	// A second valid verification reuses only the RandomX output.
	if _, err := VerifyCommitteeParticipationClaimGroupV1(CommitteeParticipationVerificationContextV1{
		ChainID: chainID, DatasetKey: datasetKey, ParentHash: parentHash,
		SelectionRoot: selectionRoot, Committee: committee, Hasher: hasher,
	}, group); err != nil {
		t.Fatal(err)
	}
	if hashCalls != len(positions) {
		t.Fatal("valid replay recalculated identical RandomX inputs")
	}
	tampered := group
	tampered.Participations = append([]CompactCommitteeParticipationV1(nil), group.Participations...)
	tampered.Participations[0] = cloneCompactCommitteeParticipationV1(tampered.Participations[0])
	tampered.Participations[0].Signature[0] ^= 0x01
	if _, err := VerifyCommitteeParticipationClaimGroupV1(
		CommitteeParticipationVerificationContextV1{
			ChainID: chainID, DatasetKey: datasetKey, ParentHash: parentHash,
			SelectionRoot: selectionRoot, Committee: committee, Hasher: hasher,
		},
		tampered,
	); err == nil {
		t.Fatal("tampered committee signature accepted")
	}
}
