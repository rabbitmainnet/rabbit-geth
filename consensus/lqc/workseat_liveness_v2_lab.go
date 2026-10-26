//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package lqc

import "github.com/ethereum/go-ethereum/common"

// Only canonical, already-admitted WorkSeats may reach this transition.
// Public testnet activation is scheduled by the core startup configuration.
func (l *LQC) workSeatMissedTurnWithLivenessV2(
	registry *CanonicalRegistry, address common.Address,
	blockNumber, maxMissedTurns, jailBlocks uint64,
) error {
	if l == nil || l.config == nil || l.config.WorkSeatLivenessV2Block == 0 ||
		blockNumber < l.config.WorkSeatLivenessV2Block {
		return registry.ApplyWorkSeatMissedTurn(address, blockNumber, maxMissedTurns, jailBlocks)
	}
	if registry == nil {
		return ErrParticipantNotActive
	}
	if address == (common.Address{}) || blockNumber == 0 {
		return ErrInvalidRegistryAddress
	}
	if _, exists := registry.entries[address]; !exists {
		// Bookkeeping is not a REGISTER operation or a new WorkSeat. Do not
		// synthesize a heartbeat, an active registry membership, or a signature.
		registry.entries[address] = CanonicalParticipant{
			Address: address, RegisteredAt: blockNumber,
		}
	}
	return registry.ApplyWorkSeatMissedTurn(address, blockNumber, maxMissedTurns, jailBlocks)
}
