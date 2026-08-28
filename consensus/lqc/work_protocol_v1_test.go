package lqc

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/holiman/uint256"
)

func workV1Hash(n byte) common.Hash {
	var h common.Hash
	h[31] = n
	return h
}

func workV1Ticket(
	address common.Address,
	hashByte byte,
	nonce uint64,
) VerifiedRandomXWorkTicketV1 {
	return VerifiedRandomXWorkTicketV1{
		Ticket: RandomXWorkTicketV1{
			Version:     RandomXWorkProtocolVersion,
			Epoch:       7,
			Participant: address,
			Nonce:       nonce,
		},
		Hash: workV1Hash(hashByte),
	}
}

func TestWorkProtocolV1RejectsMultipleSeatsForSameParticipant(t *testing.T) {
	a := common.HexToAddress("0x00000000000000000000000000000000000000a1")

	_, err := CanonicalWorkSeatsV1([]VerifiedRandomXWorkTicketV1{
		workV1Ticket(a, 3, 3),
		workV1Ticket(a, 1, 1),
		workV1Ticket(a, 2, 2),
	})
	if err != ErrDuplicateWorkParticipantV1 {
		t.Fatalf("error = %v, want %v", err, ErrDuplicateWorkParticipantV1)
	}
}

func TestWorkProtocolV1SeatRewardConservesEveryWei(t *testing.T) {
	a := common.HexToAddress("0x00000000000000000000000000000000000000a1")
	b := common.HexToAddress("0x00000000000000000000000000000000000000b1")
	c := common.HexToAddress("0x00000000000000000000000000000000000000c1")

	seats := []WorkSeatV1{
		{TicketHash: workV1Hash(1), Participant: a},
		{TicketHash: workV1Hash(2), Participant: b},
		{TicketHash: workV1Hash(3), Participant: c},
	}

	rewards, err := AggregateWorkSeatRewardsV1(uint256.NewInt(1001), seats)
	if err != nil {
		t.Fatal(err)
	}

	sum := uint256.NewInt(0)
	gotA := uint256.NewInt(0)
	gotB := uint256.NewInt(0)
	gotC := uint256.NewInt(0)

	for _, reward := range rewards {
		sum.Add(sum, reward.Amount)
		switch reward.Address {
		case a:
			gotA.Set(reward.Amount)
		case b:
			gotB.Set(reward.Amount)
		case c:
			gotC.Set(reward.Amount)
		}
	}

	if sum.Cmp(uint256.NewInt(1001)) != 0 {
		t.Fatalf("sum = %s, want 1001", sum)
	}
	if gotA.Cmp(uint256.NewInt(335)) != 0 {
		t.Fatalf("A = %s, want 335", gotA)
	}
	if gotB.Cmp(uint256.NewInt(333)) != 0 {
		t.Fatalf("B = %s, want 333", gotB)
	}
	if gotC.Cmp(uint256.NewInt(333)) != 0 {
		t.Fatalf("C = %s, want 333", gotC)
	}
}

func TestWorkProtocolV1RewardRejectsDuplicateParticipant(t *testing.T) {
	one := common.HexToAddress("0x0000000000000000000000000000000000000101")

	singleSeats := []WorkSeatV1{
		{TicketHash: workV1Hash(1), Participant: one},
		{TicketHash: workV1Hash(2), Participant: one},
		{TicketHash: workV1Hash(3), Participant: one},
		{TicketHash: workV1Hash(4), Participant: one},
	}
	_, err := AggregateWorkSeatRewardsV1(
		uint256.NewInt(1200),
		singleSeats,
	)
	if err != ErrDuplicateWorkParticipantV1 {
		t.Fatalf("error = %v, want %v", err, ErrDuplicateWorkParticipantV1)
	}
}

func TestWorkProtocolV1HundredWalletsCreateHundredSeats(t *testing.T) {
	tickets := make([]VerifiedRandomXWorkTicketV1, 0, 100)
	for i := 0; i < 100; i++ {
		tickets = append(tickets, workV1Ticket(
			common.BytesToAddress([]byte{byte(i + 1)}),
			byte(i+1),
			1,
		))
	}
	seats, err := CanonicalWorkSeatsV1(tickets)
	if err != nil {
		t.Fatal(err)
	}
	if len(seats) != 100 {
		t.Fatalf("seats=%d want=100", len(seats))
	}
}

func TestWorkProtocolV1RejectsDuplicateProofHash(t *testing.T) {
	a := common.HexToAddress("0x00000000000000000000000000000000000000a1")
	b := common.HexToAddress("0x00000000000000000000000000000000000000b1")

	_, err := CanonicalWorkSeatsV1([]VerifiedRandomXWorkTicketV1{
		workV1Ticket(a, 7, 1),
		workV1Ticket(b, 7, 2),
	})
	if err != ErrDuplicateRandomXWorkHash {
		t.Fatalf("error = %v, want %v", err, ErrDuplicateRandomXWorkHash)
	}
}
