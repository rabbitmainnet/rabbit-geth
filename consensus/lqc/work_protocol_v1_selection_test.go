package lqc

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

func selectionV1TicketHash(index int) common.Hash {
	return crypto.Keccak256Hash(
		[]byte("RABBIT-WORK-SELECTION-TEST"),
		[]byte(fmt.Sprintf("%08d", index)),
	)
}

func selectionV1SeedForTest(block uint64) common.Hash {
	seed, err := WorkSelectionSeedV1(
		big.NewInt(928),
		7,
		crypto.Keccak256Hash([]byte("selection-root")),
		crypto.Keccak256Hash([]byte("selection-entropy")),
		block,
	)
	if err != nil {
		panic(err)
	}
	return seed
}

func TestWorkSelectionV1PreservesRepeatedParticipantSeats(t *testing.T) {
	a := common.HexToAddress("0x00000000000000000000000000000000000000a1")
	seats := []WorkSeatV1{
		{TicketHash: selectionV1TicketHash(1), Participant: a},
		{TicketHash: selectionV1TicketHash(2), Participant: a},
		{TicketHash: selectionV1TicketHash(3), Participant: a},
		{TicketHash: selectionV1TicketHash(4), Participant: a},
		{TicketHash: selectionV1TicketHash(5), Participant: a},
	}

	sel, err := BuildWorkSelectionV1(seats, selectionV1SeedForTest(100), 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(sel.Ordered) != 5 ||
		sel.Producer == nil ||
		len(sel.Fallbacks) != 2 ||
		len(sel.Committee) != 2 {
		t.Fatalf("unexpected selection: %+v", sel)
	}
	for _, seat := range sel.Ordered {
		if seat.Participant != a {
			t.Fatalf("participant changed: %s", seat.Participant)
		}
	}
}

func TestWorkSelectionV1ParticipantDoesNotAffectSeatOrder(t *testing.T) {
	a := common.HexToAddress("0x00000000000000000000000000000000000000a1")
	b := common.HexToAddress("0x00000000000000000000000000000000000000b1")

	left := make([]WorkSeatV1, 64)
	right := make([]WorkSeatV1, 64)

	for i := 0; i < 64; i++ {
		hash := selectionV1TicketHash(i + 1)
		left[i] = WorkSeatV1{TicketHash: hash, Participant: a}
		right[i] = WorkSeatV1{TicketHash: hash, Participant: b}
	}

	orderedLeft, err := DeterministicallyOrderWorkSeatsV1(left, selectionV1SeedForTest(321))
	if err != nil {
		t.Fatal(err)
	}
	orderedRight, err := DeterministicallyOrderWorkSeatsV1(right, selectionV1SeedForTest(321))
	if err != nil {
		t.Fatal(err)
	}

	if len(orderedLeft) != len(orderedRight) {
		t.Fatal("order length changed")
	}
	for i := range orderedLeft {
		if orderedLeft[i].TicketHash != orderedRight[i].TicketHash {
			t.Fatalf(
				"ticket order changed at %d: %s != %s",
				i,
				orderedLeft[i].TicketHash,
				orderedRight[i].TicketHash,
			)
		}
	}
}

func TestWorkSelectionV1OneVsFiveThousandIdentitiesNoFreePower(t *testing.T) {
	// 80 honest seats + 20 attacker seats. The exact same 20 ticket hashes
	// represent the attacker's fixed work in both scenarios.
	const honestSeats = 80
	const attackerSeats = 20

	honest := make([]WorkSeatV1, 0, honestSeats)
	for i := 0; i < honestSeats; i++ {
		honest = append(honest, WorkSeatV1{
			TicketHash: selectionV1TicketHash(i + 1),
			Participant: common.BigToAddress(
				big.NewInt(int64(10_000 + i)),
			),
		})
	}

	oneAddress := common.HexToAddress(
		"0x0000000000000000000000000000000000000a01",
	)
	one := append([]WorkSeatV1(nil), honest...)
	split := append([]WorkSeatV1(nil), honest...)

	splitController := make(map[common.Address]struct{}, 5000)
	splitAddresses := make([]common.Address, 0, 5000)
	for i := 0; i < 5000; i++ {
		address := common.BigToAddress(
			big.NewInt(int64(100_000 + i)),
		)
		splitController[address] = struct{}{}
		splitAddresses = append(splitAddresses, address)
	}

	oneController := map[common.Address]struct{}{
		oneAddress: {},
	}

	for i := 0; i < attackerSeats; i++ {
		hash := selectionV1TicketHash(honestSeats + i + 1)

		one = append(one, WorkSeatV1{
			TicketHash:  hash,
			Participant: oneAddress,
		})
		split = append(split, WorkSeatV1{
			TicketHash:  hash,
			Participant: splitAddresses[i],
		})
	}

	type totals struct {
		producer  uint64
		fallbacks uint64
		committee uint64
	}

	countControlled := func(
		sel WorkSelectionV1,
		controller map[common.Address]struct{},
	) totals {
		var out totals
		if sel.Producer != nil {
			if _, ok := controller[sel.Producer.Participant]; ok {
				out.producer++
			}
		}
		for _, seat := range sel.Fallbacks {
			if _, ok := controller[seat.Participant]; ok {
				out.fallbacks++
			}
		}
		for _, seat := range sel.Committee {
			if _, ok := controller[seat.Participant]; ok {
				out.committee++
			}
		}
		return out
	}

	var oneTotal, splitTotal totals

	// Across many canonical block seeds, role counts must be exactly identical
	// because the attacker's WORK SEAT hashes are identical; only addresses
	// differ. The other 4,980 split identities have no seats and add no power.
	for block := uint64(1); block <= 1000; block++ {
		left, err := BuildWorkSelectionV1(
			one, selectionV1SeedForTest(block), 5, 32,
		)
		if err != nil {
			t.Fatal(err)
		}
		right, err := BuildWorkSelectionV1(
			split, selectionV1SeedForTest(block), 5, 32,
		)
		if err != nil {
			t.Fatal(err)
		}

		l := countControlled(left, oneController)
		r := countControlled(right, splitController)

		oneTotal.producer += l.producer
		oneTotal.fallbacks += l.fallbacks
		oneTotal.committee += l.committee

		splitTotal.producer += r.producer
		splitTotal.fallbacks += r.fallbacks
		splitTotal.committee += r.committee
	}

	if oneTotal != splitTotal {
		t.Fatalf(
			"identity split changed power: one=%+v split=%+v",
			oneTotal,
			splitTotal,
		)
	}

	if oneTotal.producer == 0 ||
		oneTotal.fallbacks == 0 ||
		oneTotal.committee == 0 {
		t.Fatalf("test did not exercise all roles: %+v", oneTotal)
	}
}

func TestWorkSelectionV1RejectsDuplicateTicketHash(t *testing.T) {
	a := common.HexToAddress("0x00000000000000000000000000000000000000a1")
	b := common.HexToAddress("0x00000000000000000000000000000000000000b1")
	hash := selectionV1TicketHash(1)

	_, err := BuildWorkSelectionV1(
		[]WorkSeatV1{
			{TicketHash: hash, Participant: a},
			{TicketHash: hash, Participant: b},
		},
		selectionV1SeedForTest(1),
		1,
		1,
	)
	if err != ErrDuplicateRandomXWorkHash {
		t.Fatalf("error = %v, want %v", err, ErrDuplicateRandomXWorkHash)
	}
}
