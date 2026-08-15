package lqc

import "github.com/ethereum/go-ethereum/common"

// StaticTestParticipants is kept only for legacy fallback.
// Open/hybrid mode should not depend on this list.
func StaticTestParticipants() []common.Address {
	return []common.Address{}
}
