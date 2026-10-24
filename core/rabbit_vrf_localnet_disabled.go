//go:build !rabbit_vrf_localnet

package core

import "math/big"

func rabbitVRFLocalnetChainIDAllowed(id *big.Int) bool {
	return false
}
