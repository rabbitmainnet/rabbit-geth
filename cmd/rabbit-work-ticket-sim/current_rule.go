package main

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/crypto"
)

func simulateCurrentAddressRule(opts options, attackerIdentities int) roleMetrics {
	participants := make([]lqc.HybridParticipant, 0, opts.honestMiners+attackerIdentities)
	attacker := make(map[common.Address]bool, attackerIdentities)
	for index := 0; index < opts.honestMiners; index++ {
		participants = append(participants, participantFor("honest", index))
	}
	for index := 0; index < attackerIdentities; index++ {
		participant := participantFor("attacker", index)
		participants = append(participants, participant)
		attacker[participant.Address] = true
	}

	parent := crypto.Keccak256Hash([]byte("RABBIT-CURRENT-ADDRESS-RULE"), uint64Bytes(uint64(attackerIdentities)))
	counter := metricCounter{slots: opts.slots}
	for slot := uint64(1); slot <= opts.slots; slot++ {
		ordered := lqc.DeterministicallyOrderParticipants(participants, parent, slot)
		flags := make([]bool, len(ordered))
		for index := range ordered {
			flags[index] = attacker[ordered[index].Address]
		}
		countRoles(&counter, flags, opts)
		parent = nextHash(parent, slot)
	}
	return counter.metrics()
}

func participantFor(group string, index int) lqc.HybridParticipant {
	hash := crypto.Keccak256Hash([]byte(fmt.Sprintf("RABBIT-LQC-CURRENT-%s-%d", group, index)))
	address := common.BytesToAddress(hash[12:])
	return lqc.HybridParticipant{
		Address: address,
		Payout:  address,
		Bond:    new(big.Int),
		Status:  lqc.ParticipantActiveCandidate,
	}
}
