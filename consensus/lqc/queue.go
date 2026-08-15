package lqc

import "github.com/ethereum/go-ethereum/common"

func BuildQueue(committee []Participant) []common.Address {
	out := make([]common.Address, 0, len(committee))
	for _, p := range committee {
		out = append(out, p.Address)
	}
	return out
}

func QueuePosition(queue []common.Address, addr common.Address) int {
	for i, a := range queue {
		if a == addr {
			return i
		}
	}
	return -1
}
