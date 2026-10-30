package core

import "math/big"

// RabbitVRFRewardAmounts exposes the existing settlement calculation
// for read-only reporting. It does not execute or change settlement.
func RabbitVRFRewardAmounts(fee *big.Int, participants int, economicsV2 bool) (*big.Int, *big.Int, error) {
	split := rabbitVRFSplitProtocolFeeV1
	if economicsV2 {
		split = rabbitVRFSplitProtocolFeeV2
	}
	producer, each, _, err := split(fee, participants)
	return producer, each, err
}
