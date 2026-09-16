package lqc

import (
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
)

func compactClaimV1(position uint8) CompactCommitteeParticipationV1 {
	return CompactCommitteeParticipationV1{
		Position:  position,
		Signature: make([]byte, crypto.SignatureLength),
	}
}

func TestCommitteeParticipationClaimGroupsV1CanonicalWindow(t *testing.T) {
	input := []CommitteeParticipationClaimGroupV1{
		{TargetBlock: 99, Participations: []CompactCommitteeParticipationV1{compactClaimV1(7), compactClaimV1(1)}},
		{TargetBlock: 93, Participations: []CompactCommitteeParticipationV1{compactClaimV1(3)}},
	}
	canonical, err := CanonicalCommitteeParticipationClaimGroupsV1(101, input)
	if err != nil {
		t.Fatal(err)
	}
	if canonical[0].TargetBlock != 93 || canonical[1].TargetBlock != 99 ||
		canonical[1].Participations[0].Position != 1 ||
		canonical[1].Participations[1].Position != 7 {
		t.Fatalf("unexpected canonical claims: %+v", canonical)
	}
	if err := ValidateCanonicalCommitteeParticipationClaimGroupsV1(101, input); !errors.Is(err, ErrNonCanonicalCommitteeClaimsV1) {
		t.Fatalf("unordered error=%v", err)
	}
	if err := ValidateCanonicalCommitteeParticipationClaimGroupsV1(101, canonical); err != nil {
		t.Fatalf("canonical claims rejected: %v", err)
	}

	tests := []struct {
		name      string
		inclusion uint64
		groups    []CommitteeParticipationClaimGroupV1
		want      error
	}{
		{name: "current block", inclusion: 101, groups: []CommitteeParticipationClaimGroupV1{{TargetBlock: 101, Participations: []CompactCommitteeParticipationV1{compactClaimV1(0)}}}, want: ErrInvalidCommitteeParticipationClaimV1},
		{name: "expired", inclusion: 101, groups: []CommitteeParticipationClaimGroupV1{{TargetBlock: 92, Participations: []CompactCommitteeParticipationV1{compactClaimV1(0)}}}, want: ErrExpiredCommitteeParticipationClaimV1},
		{name: "duplicate target group", inclusion: 101, groups: []CommitteeParticipationClaimGroupV1{{TargetBlock: 100, Participations: []CompactCommitteeParticipationV1{compactClaimV1(0)}}, {TargetBlock: 100, Participations: []CompactCommitteeParticipationV1{compactClaimV1(1)}}}, want: ErrDuplicateCommitteeParticipationClaimV1},
		{name: "duplicate position", inclusion: 101, groups: []CommitteeParticipationClaimGroupV1{{TargetBlock: 100, Participations: []CompactCommitteeParticipationV1{compactClaimV1(0), compactClaimV1(0)}}}, want: ErrDuplicateCommitteeParticipationClaimV1},
		{name: "position above 127", inclusion: 101, groups: []CommitteeParticipationClaimGroupV1{{TargetBlock: 100, Participations: []CompactCommitteeParticipationV1{compactClaimV1(128)}}}, want: ErrInvalidCommitteeParticipationClaimV1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := CanonicalCommitteeParticipationClaimGroupsV1(test.inclusion, test.groups)
			if !errors.Is(err, test.want) {
				t.Fatalf("error=%v want=%v", err, test.want)
			}
		})
	}
	tooMany := make([]CompactCommitteeParticipationV1, MaxCommitteeParticipationsV1+1)
	for index := range tooMany {
		tooMany[index] = compactClaimV1(uint8(index))
	}
	_, err = CanonicalCommitteeParticipationClaimGroupsV1(101, []CommitteeParticipationClaimGroupV1{{
		TargetBlock:    100,
		Participations: tooMany,
	}})
	if !errors.Is(err, ErrTooManyCommitteeParticipationsV1) {
		t.Fatalf("global claim limit error=%v", err)
	}
}

type delayedCommitteeHeaderSizeEnvelopeV1 struct {
	Version                      uint8
	BlockNumber                  uint64
	RegistryRoot                 common.Hash
	WorkStateRoot                common.Hash
	CommitteeClaimRoot           common.Hash
	RegistryOperations           []RegistryOperation
	WorkTickets                  []SignedRandomXWorkTicketV1
	CommitteeParticipationClaims []CommitteeParticipationClaimGroupV1
}

func TestDelayedCommitteeParticipationV1CombinedHeaderBudget(t *testing.T) {
	groups := make([]CommitteeParticipationClaimGroupV1, CommitteeParticipationClaimWindowV1)
	proofIndex := 0
	for groupIndex := range groups {
		groups[groupIndex].TargetBlock = ^uint64(0) - CommitteeParticipationClaimWindowV1 + uint64(groupIndex)
		groups[groupIndex].Participations = make([]CompactCommitteeParticipationV1, MaxCommitteeParticipationsV1/len(groups))
		for index := range groups[groupIndex].Participations {
			groups[groupIndex].Participations[index] = compactClaimV1(uint8(proofIndex % MaxCommitteeParticipationsV1))
			proofIndex++
		}
	}
	tickets := make([]SignedRandomXWorkTicketV1, MaxWorkTicketsPerBlockV1)
	for index := range tickets {
		tickets[index] = signedHeaderWorkTicketV3(t, ^uint64(0)-uint64(index), byte(index+1), ^uint64(0)-uint64(index))
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
	envelope := delayedCommitteeHeaderSizeEnvelopeV1{
		Version:                      4,
		BlockNumber:                  ^uint64(0),
		RegistryRoot:                 crypto.Keccak256Hash([]byte("registry")),
		WorkStateRoot:                crypto.Keccak256Hash([]byte("work")),
		CommitteeClaimRoot:           crypto.Keccak256Hash([]byte("claims")),
		WorkTickets:                  tickets,
		CommitteeParticipationClaims: groups,
	}
	maxOperations := -1
	maxSize := 0
	for count := 0; count <= len(operations); count++ {
		envelope.RegistryOperations = operations[:count]
		payload, err := rlp.EncodeToBytes(envelope)
		if err != nil {
			t.Fatal(err)
		}
		size := len(registryHeaderMagic) + len(payload) + ProducerSealLength
		if size <= MaxRegistryHeaderExtraSize {
			maxOperations = count
			maxSize = size
		}
	}
	if maxOperations < 0 {
		t.Fatal("delayed 128-proof batch plus maximum admission tickets does not fit")
	}
	t.Logf("delayed combined header: claims=%d groups=%d tickets=%d maxRegistryOperations=%d size=%d limit=%d remaining=%d", proofIndex, len(groups), len(tickets), maxOperations, maxSize, MaxRegistryHeaderExtraSize, MaxRegistryHeaderExtraSize-maxSize)
}
