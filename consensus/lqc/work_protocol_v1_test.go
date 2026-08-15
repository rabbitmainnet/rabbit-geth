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

func TestWorkProtocolV1RetainsMultipleSeatsForSameParticipant(t *testing.T) {
	a := common.HexToAddress("0x00000000000000000000000000000000000000a1")

	seats, err := CanonicalWorkSeatsV1([]VerifiedRandomXWorkTicketV1{
		workV1Ticket(a, 3, 3),
		workV1Ticket(a, 1, 1),
		workV1Ticket(a, 2, 2),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(seats) != 3 {
		t.Fatalf("seats = %d, want 3", len(seats))
	}
	for _, seat := range seats {
		if seat.Participant != a {
			t.Fatalf("unexpected participant %s", seat.Participant)
		}
	}
}

func TestWorkProtocolV1SeatRewardConservesEveryWei(t *testing.T) {
	a := common.HexToAddress("0x00000000000000000000000000000000000000a1")
	b := common.HexToAddress("0x00000000000000000000000000000000000000b1")

	seats := []WorkSeatV1{
		{TicketHash: workV1Hash(1), Participant: a},
		{TicketHash: workV1Hash(2), Participant: a},
		{TicketHash: workV1Hash(3), Participant: b},
	}

	rewards, err := AggregateWorkSeatRewardsV1(uint256.NewInt(1001), seats)
	if err != nil {
		t.Fatal(err)
	}

	sum := uint256.NewInt(0)
	gotA := uint256.NewInt(0)
	gotB := uint256.NewInt(0)

	for _, reward := range rewards {
		sum.Add(sum, reward.Amount)
		switch reward.Address {
		case a:
			gotA.Set(reward.Amount)
		case b:
			gotB.Set(reward.Amount)
		}
	}

	if sum.Cmp(uint256.NewInt(1001)) != 0 {
		t.Fatalf("sum = %s, want 1001", sum)
	}
	if gotA.Cmp(uint256.NewInt(668)) != 0 {
		t.Fatalf("A = %s, want 668", gotA)
	}
	if gotB.Cmp(uint256.NewInt(333)) != 0 {
		t.Fatalf("B = %s, want 333", gotB)
	}
}

func TestWorkProtocolV1AddressSplitGetsNoRewardBonus(t *testing.T) {
	one := common.HexToAddress("0x0000000000000000000000000000000000000101")

	singleSeats := []WorkSeatV1{
		{TicketHash: workV1Hash(1), Participant: one},
		{TicketHash: workV1Hash(2), Participant: one},
		{TicketHash: workV1Hash(3), Participant: one},
		{TicketHash: workV1Hash(4), Participant: one},
	}
	singleRewards, err := AggregateWorkSeatRewardsV1(
		uint256.NewInt(1200),
		singleSeats,
	)
	if err != nil {
		t.Fatal(err)
	}

	splitAddresses := []common.Address{
		common.HexToAddress("0x0000000000000000000000000000000000000201"),
		common.HexToAddress("0x0000000000000000000000000000000000000202"),
		common.HexToAddress("0x0000000000000000000000000000000000000203"),
		common.HexToAddress("0x0000000000000000000000000000000000000204"),
	}
	splitSeats := make([]WorkSeatV1, 0, len(splitAddresses))
	for index, address := range splitAddresses {
		splitSeats = append(splitSeats, WorkSeatV1{
			TicketHash:  workV1Hash(byte(index + 1)),
			Participant: address,
		})
	}
	splitRewards, err := AggregateWorkSeatRewardsV1(
		uint256.NewInt(1200),
		splitSeats,
	)
	if err != nil {
		t.Fatal(err)
	}

	singleController := map[common.Address]struct{}{one: {}}
	splitController := make(map[common.Address]struct{}, len(splitAddresses))
	for _, address := range splitAddresses {
		splitController[address] = struct{}{}
	}

	singleTotal := WorkControllerRewardTotalV1(singleRewards, singleController)
	splitTotal := WorkControllerRewardTotalV1(splitRewards, splitController)

	if singleTotal.Cmp(uint256.NewInt(1200)) != 0 {
		t.Fatalf("single total = %s, want 1200", singleTotal)
	}
	if splitTotal.Cmp(singleTotal) != 0 {
		t.Fatalf(
			"split total = %s, single total = %s",
			splitTotal,
			singleTotal,
		)
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
