package lqc

import (
	"crypto/ecdsa"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

func TestVerifyCommitteeParticipationClaimGroupV1UsesSharedDatasetKey(t *testing.T) {
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
