package main

import (
	"bytes"
	"fmt"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

type workTicket struct {
	ID         common.Hash
	Controller string
}

type scoredTicket struct {
	ticket workTicket
	score  common.Hash
}

func simulateWorkTicketRule(opts options, attackerIdentities int, attackerWorkUnits uint64) roleMetrics {
	var tickets []workTicket
	for miner := 0; miner < opts.honestMiners; miner++ {
		controller := fmt.Sprintf("honest-%d", miner)
		for ticket := uint64(0); ticket < opts.ticketsPerWork; ticket++ {
			tickets = append(tickets, makeTicket(controller, 0, ticket))
		}
	}
	attackerTickets := attackerWorkUnits * opts.ticketsPerWork
	for ticket := uint64(0); ticket < attackerTickets; ticket++ {
		identity := uint64(0)
		if attackerIdentities > 0 {
			identity = ticket % uint64(attackerIdentities)
		}
		tickets = append(tickets, makeTicket("attacker", identity, ticket))
	}

	parent := crypto.Keccak256Hash(
		[]byte("RABBIT-CONTINUOUS-WORK-TICKET-CANONICAL-SEED-V2"),
	)
	counter := metricCounter{slots: opts.slots}
	for slot := uint64(1); slot <= opts.slots; slot++ {
		ordered := orderTickets(tickets, parent, slot)
		flags := make([]bool, len(ordered))
		for index := range ordered {
			flags[index] = ordered[index].Controller == "attacker"
		}
		countRoles(&counter, flags, opts)
		parent = nextHash(parent, slot)
	}
	return counter.metrics()
}

func orderTickets(tickets []workTicket, parent common.Hash, slot uint64) []workTicket {
	seed := crypto.Keccak256Hash(
		[]byte("RABBIT-LQC-WORK-SELECTION-V1"),
		parent.Bytes(),
		uint64Bytes(slot),
	)
	scored := make([]scoredTicket, len(tickets))
	for index, ticket := range tickets {
		scored[index] = scoredTicket{
			ticket: ticket,
			score:  crypto.Keccak256Hash(seed.Bytes(), ticket.ID.Bytes()),
		}
	}
	sort.Slice(scored, func(i, j int) bool {
		comparison := bytes.Compare(scored[i].score.Bytes(), scored[j].score.Bytes())
		if comparison != 0 {
			return comparison < 0
		}
		return bytes.Compare(scored[i].ticket.ID.Bytes(), scored[j].ticket.ID.Bytes()) < 0
	})
	ordered := make([]workTicket, len(scored))
	for index := range scored {
		ordered[index] = scored[index].ticket
	}
	return ordered
}

func makeTicket(controller string, identity, index uint64) workTicket {
	id := crypto.Keccak256Hash(
		[]byte("RABBIT-LQC-WORK-TICKET-V1"),
		[]byte(controller),
		uint64Bytes(identity),
		uint64Bytes(index),
	)
	return workTicket{ID: id, Controller: controller}
}
