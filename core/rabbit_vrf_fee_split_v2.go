package core

import (
	"fmt"
	"math/big"
)

// V2 is prepared independently. Historical V1 callers remain unchanged.
func rabbitVRFSplitProtocolFeeV2(
	fee *big.Int,
	participantCount int,
) (
	producer *big.Int,
	eachParticipant *big.Int,
	rabbitAllocation *big.Int,
	err error,
) {
	if fee == nil || fee.Sign() < 0 || participantCount <= 0 {
		return nil, nil, nil, fmt.Errorf(
			"invalid rabbit vrf settlement input",
		)
	}

	denominator := new(big.Int).SetUint64(
		rabbitVRFBPSDenominatorV1,
	)

	producer = new(big.Int).Mul(
		new(big.Int).Set(fee),
		new(big.Int).SetUint64(2000),
	)
	producer.Div(producer, denominator)

	participantPool := new(big.Int).Mul(
		new(big.Int).Set(fee),
		new(big.Int).SetUint64(3000),
	)
	participantPool.Div(participantPool, denominator)

	eachParticipant = new(big.Int).Div(
		participantPool,
		new(big.Int).SetUint64(uint64(participantCount)),
	)
	paidParticipants := new(big.Int).Mul(
		new(big.Int).Set(eachParticipant),
		new(big.Int).SetUint64(uint64(participantCount)),
	)

	// Rabbit Allocation receives the base 50% plus every integer
	// remainder from BPS flooring and equal participant division.
	rabbitAllocation = new(big.Int).Sub(
		new(big.Int).Set(fee),
		producer,
	)
	rabbitAllocation.Sub(
		rabbitAllocation,
		paidParticipants,
	)
	if rabbitAllocation.Sign() < 0 {
		return nil, nil, nil, fmt.Errorf(
			"invalid rabbit vrf settlement conservation",
		)
	}
	return producer, eachParticipant, rabbitAllocation, nil
}
