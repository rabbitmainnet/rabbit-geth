package lqc

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

func TestCommitteeParticipationV1BindsSelectedWorkSeatAndBlock(t *testing.T) {
	chainID := big.NewInt(9280)
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	participant := crypto.PubkeyToAddress(key.PublicKey)
	selected := WorkSeatV1{
		TicketHash:  crypto.Keccak256Hash([]byte("selected-ticket")),
		Participant: participant,
	}
	proof := CommitteeParticipationV1{
		Version:       CommitteeParticipationVersionV1,
		BlockNumber:   100,
		ParentHash:    crypto.Keccak256Hash([]byte("parent")),
		SelectionRoot: crypto.Keccak256Hash([]byte("selection")),
		TicketHash:    selected.TicketHash,
		Participant:   participant,
	}
	input, err := CommitteeParticipationInputV1(chainID, proof)
	if err != nil {
		t.Fatal(err)
	}
	proofHash := crypto.Keccak256Hash([]byte("test-randomx"), input)
	signingHash, err := CommitteeParticipationSigningHashV1(chainID, proof, proofHash)
	if err != nil {
		t.Fatal(err)
	}
	proof.Signature, err = crypto.Sign(signingHash[:], key)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyCommitteeParticipationV1(chainID, selected, proof, proofHash); err != nil {
		t.Fatalf("valid participation rejected: %v", err)
	}

	tests := []struct {
		name     string
		selected WorkSeatV1
		proof    CommitteeParticipationV1
		hash     common.Hash
	}{
		{name: "wrong ticket", selected: WorkSeatV1{TicketHash: crypto.Keccak256Hash([]byte("other")), Participant: participant}, proof: proof, hash: proofHash},
		{name: "wrong block", selected: selected, proof: func() CommitteeParticipationV1 { changed := proof; changed.BlockNumber++; return changed }(), hash: proofHash},
		{name: "wrong proof hash", selected: selected, proof: proof, hash: crypto.Keccak256Hash([]byte("wrong"))},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := VerifyCommitteeParticipationV1(chainID, test.selected, test.proof, test.hash); err == nil {
				t.Fatal("modified participation accepted")
			}
		})
	}
}
