package lqc

import (
	"bytes"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

func ComputeCommitteeSize(active uint64) uint64 {
	return ComputeCommitteeSizeWithBounds(active, 32, 128)
}

// ComputeCommitteeSizeWithBounds returns ceil(active*10%), clamped to the
// configured LCQ committee bounds. The caller still caps the result to the
// participants actually available after the producer/fallback positions.
func ComputeCommitteeSizeWithBounds(active, minSize, maxSize uint64) uint64 {
	size := (active + 9) / 10 // ceil(active * 0.10)

	if minSize > 0 && size < minSize {
		size = minSize
	}
	if maxSize > 0 && size > maxSize {
		size = maxSize
	}
	return size
}

func participantOrderKey(seed common.Hash, addr common.Address) common.Hash {
	data := append(seed.Bytes(), addr.Bytes()...)
	return crypto.Keccak256Hash(data)
}

func SelectCommittee(snapshot *Snapshot, seed common.Hash) []Participant {
	if snapshot == nil {
		return nil
	}
	active := snapshot.ActiveParticipants()
	sort.SliceStable(active, func(i, j int) bool {
		ki := participantOrderKey(seed, active[i].Address)
		kj := participantOrderKey(seed, active[j].Address)
		return bytes.Compare(ki.Bytes(), kj.Bytes()) < 0
	})
	target := ComputeCommitteeSize(uint64(len(active)))
	if uint64(len(active)) <= target {
		return active
	}
	return active[:target]
}

func CommitteeAddresses(committee []Participant) []common.Address {
	out := make([]common.Address, 0, len(committee))
	for _, p := range committee {
		out = append(out, p.Address)
	}
	return out
}
