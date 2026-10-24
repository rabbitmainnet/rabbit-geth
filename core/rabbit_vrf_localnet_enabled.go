//go:build rabbit_vrf_localnet

package core

import "math/big"

func rabbitVRFLocalnetChainIDAllowed(id *big.Int) bool {
	return id != nil && id.Cmp(big.NewInt(19280)) == 0
}
