package lqc

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
)

type compactCommitteeParticipationSizeEntryV1 struct {
	Position  uint8
	Signature []byte
}

type compactCommitteeParticipationSizeEnvelopeV1 struct {
	Version        uint8
	BlockNumber    uint64
	ParentHash     common.Hash
	SelectionRoot  common.Hash
	Participations []compactCommitteeParticipationSizeEntryV1
}

func TestCommitteeParticipationV1HeaderSizeBudget(t *testing.T) {
	selected, full := committeeParticipationBatchFixtureV1(
		MaxCommitteeParticipationsV1,
	)
	if len(selected) != MaxCommitteeParticipationsV1 {
		t.Fatalf("selected=%d want=%d", len(selected), MaxCommitteeParticipationsV1)
	}
	for index := range full {
		full[index].Signature = make([]byte, crypto.SignatureLength)
	}

	fullPayload, err := rlp.EncodeToBytes(full)
	if err != nil {
		t.Fatal(err)
	}
	fullExtraSize := len(registryHeaderMagic) + len(fullPayload) + ProducerSealLength
	if fullExtraSize <= MaxRegistryHeaderExtraSize {
		t.Fatalf("expanded representation unexpectedly fits: %d <= %d", fullExtraSize, MaxRegistryHeaderExtraSize)
	}

	compact := compactCommitteeParticipationSizeEnvelopeV1{
		Version:        CommitteeParticipationVersionV1,
		BlockNumber:    full[0].BlockNumber,
		ParentHash:     full[0].ParentHash,
		SelectionRoot:  full[0].SelectionRoot,
		Participations: make([]compactCommitteeParticipationSizeEntryV1, len(full)),
	}
	for index := range full {
		compact.Participations[index] = compactCommitteeParticipationSizeEntryV1{
			Position:  uint8(index),
			Signature: append([]byte(nil), full[index].Signature...),
		}
	}
	compactPayload, err := rlp.EncodeToBytes(compact)
	if err != nil {
		t.Fatal(err)
	}
	compactExtraSize := len(registryHeaderMagic) + len(compactPayload) + ProducerSealLength
	if compactExtraSize > MaxRegistryHeaderExtraSize {
		t.Fatalf("compact representation exceeds header: %d > %d", compactExtraSize, MaxRegistryHeaderExtraSize)
	}

	t.Logf(
		"committee participation sizes: expanded=%d compact=%d limit=%d remaining=%d",
		fullExtraSize,
		compactExtraSize,
		MaxRegistryHeaderExtraSize,
		MaxRegistryHeaderExtraSize-compactExtraSize,
	)
}
