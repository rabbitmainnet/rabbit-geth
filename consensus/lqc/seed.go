package lqc

import (
	"encoding/binary"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

func BuildSeed(parentHash common.Hash, blockNumber uint64) common.Hash {
	var num [8]byte
	binary.BigEndian.PutUint64(num[:], blockNumber)
	data := append(parentHash.Bytes(), num[:]...)
	return crypto.Keccak256Hash(data)
}
