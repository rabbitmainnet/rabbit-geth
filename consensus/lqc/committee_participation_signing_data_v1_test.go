package lqc

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

func TestCommitteeParticipationSigningDataMatchesConsensusHashV1(t *testing.T) {
	proof := CommitteeParticipationV1{
		Version:       CommitteeParticipationVersionV1,
		BlockNumber:   100,
		ParentHash:    common.HexToHash("0x01"),
		SelectionRoot: common.HexToHash("0x02"),
		TicketHash:    common.HexToHash("0x03"),
		Participant:   common.HexToAddress("0x1000000000000000000000000000000000000001"),
	}
	proofHash := common.HexToHash("0x04")
	data, err := CommitteeParticipationSigningDataV1(big.NewInt(928), proof, proofHash)
	if err != nil {
		t.Fatal(err)
	}
	want, err := CommitteeParticipationSigningHashV1(big.NewInt(928), proof, proofHash)
	if err != nil {
		t.Fatal(err)
	}
	if got := crypto.Keccak256Hash(data); got != want {
		t.Fatalf("wallet signing hash=%s consensus hash=%s", got, want)
	}
}
