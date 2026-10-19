package lqc

import (
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
)

func TestCompactCommitteeParticipationsV1RoundTrip(t *testing.T) {
	selected, proofs := committeeParticipationBatchFixtureV1(4)
	unordered := []CommitteeParticipationV1{proofs[3], proofs[1]}
	compact, err := CompactCommitteeParticipationsV1(selected, unordered)
	if err != nil {
		t.Fatal(err)
	}
	if len(compact) != 2 || compact[0].Position != 1 || compact[1].Position != 3 {
		t.Fatalf("unexpected compact order: %+v", compact)
	}
	expanded, err := ExpandCommitteeParticipationsV1(
		proofs[0].BlockNumber,
		proofs[0].ParentHash,
		proofs[0].SelectionRoot,
		selected,
		compact,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(expanded) != 2 ||
		expanded[0].Participant != selected[1].Participant ||
		expanded[0].TicketHash != selected[1].TicketHash ||
		expanded[1].Participant != selected[3].Participant ||
		expanded[1].TicketHash != selected[3].TicketHash {
		t.Fatalf("unexpected expanded proofs: %+v", expanded)
	}

	tests := []struct {
		name    string
		compact []CompactCommitteeParticipationV1
		want    error
	}{
		{name: "duplicate position", compact: []CompactCommitteeParticipationV1{compact[0], compact[0]}, want: ErrNonCanonicalCommitteeParticipationV1},
		{name: "descending positions", compact: []CompactCommitteeParticipationV1{compact[1], compact[0]}, want: ErrNonCanonicalCommitteeParticipationV1},
		{name: "outside committee", compact: []CompactCommitteeParticipationV1{{Position: 4, Signature: make([]byte, crypto.SignatureLength)}}, want: ErrCommitteeParticipantNotSelectedV1},
		{name: "invalid signature length", compact: []CompactCommitteeParticipationV1{{Position: 0, Signature: make([]byte, crypto.SignatureLength-1)}}, want: ErrInvalidCompactCommitteeParticipationV1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ExpandCommitteeParticipationsV1(
				proofs[0].BlockNumber,
				proofs[0].ParentHash,
				proofs[0].SelectionRoot,
				selected,
				test.compact,
			)
			if !errors.Is(err, test.want) {
				t.Fatalf("error=%v want=%v", err, test.want)
			}
		})
	}
}

type combinedCommitteeHeaderSizeEnvelopeV1 struct {
	Version                 uint8
	BlockNumber             uint64
	RegistryRoot            common.Hash
	WorkStateRoot           common.Hash
	ParentHash              common.Hash
	SelectionRoot           common.Hash
	RegistryOperations      []RegistryOperation
	WorkTickets             []SignedRandomXWorkTicketV1
	CommitteeParticipations []CompactCommitteeParticipationV1
}

func TestCommitteeParticipationV1CombinedHeaderBudget(t *testing.T) {
	selected, proofs := committeeParticipationBatchFixtureV1(MaxCommitteeParticipationsV1)
	compact, err := CompactCommitteeParticipationsV1(selected, proofs)
	if err != nil {
		t.Fatal(err)
	}
	tickets := make([]SignedRandomXWorkTicketV1, MaxWorkTicketsPerBlockV1)
	for index := range tickets {
		tickets[index] = signedHeaderWorkTicketV3(
			t,
			^uint64(0)-uint64(index),
			byte(index+1),
			^uint64(0)-uint64(index),
		)
	}
	operations := make([]RegistryOperation, MaxRegistryOperationsPerBlock)
	for index := range operations {
		operations[index] = RegistryOperation{
			Version:    RegistryProtocolVersion,
			Action:     RegistryActionHeartbeat,
			Address:    common.BigToAddress(big.NewInt(int64(index + 1))),
			Sequence:   ^uint64(0) - uint64(index),
			ValidUntil: ^uint64(0),
			ProofNonce: ^uint64(0) - uint64(index),
			Signature:  make([]byte, crypto.SignatureLength),
		}
	}

	base := combinedCommitteeHeaderSizeEnvelopeV1{
		Version:                 4,
		BlockNumber:             100,
		RegistryRoot:            crypto.Keccak256Hash([]byte("registry")),
		WorkStateRoot:           crypto.Keccak256Hash([]byte("work")),
		ParentHash:              proofs[0].ParentHash,
		SelectionRoot:           proofs[0].SelectionRoot,
		WorkTickets:             tickets,
		CommitteeParticipations: compact,
	}
	maxOperations := -1
	maxSize := 0
	for count := 0; count <= len(operations); count++ {
		base.RegistryOperations = operations[:count]
		payload, encodeErr := rlp.EncodeToBytes(base)
		if encodeErr != nil {
			t.Fatal(encodeErr)
		}
		size := len(registryHeaderMagic) + len(payload) + ProducerSealLength
		if size <= MaxRegistryHeaderExtraSize {
			maxOperations = count
			maxSize = size
		}
	}
	if maxOperations < 0 {
		t.Fatal("128 committee proofs plus maximum admission tickets do not fit")
	}
	t.Logf(
		"combined header: committee=%d tickets=%d maxRegistryOperations=%d size=%d limit=%d remaining=%d",
		len(compact),
		len(tickets),
		maxOperations,
		maxSize,
		MaxRegistryHeaderExtraSize,
		MaxRegistryHeaderExtraSize-maxSize,
	)
}
