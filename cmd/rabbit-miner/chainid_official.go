//go:build !rabbit_vrf_localnet

package main

func allowedMinerChainID(id uint64) bool {
	return id == officialChainID
}
