package main

import (
	"fmt"
	"math/big"
)

func vrfRewardLines(activity rewardActivityRPC, number uint64, balance *big.Int, unit string) []string {
	lines := make([]string, 0)
	for _, credit := range activity.VRFCredits {
		amount := rewardRPCAmount(credit.Amount)
		if amount.Sign() <= 0 {
			continue
		}
		lines = append(lines, fmt.Sprintf(
			"VRF REWARD | #%d | +%s %s | %s | Request: %s | Balance: %s %s\n",
			number, formatRAB(amount), unit, credit.Role,
			credit.RequestID.Hex(), formatRAB(balance), unit,
		))
	}
	return lines
}
