package lqc

import "github.com/ethereum/go-ethereum/common"

type Participant struct {
	Address         common.Address
	Active          bool
	LastActiveBlock uint64
	Weight          uint64
}

type Snapshot struct {
	BlockNumber  uint64
	Participants []Participant
}

func (s *Snapshot) ActiveParticipants() []Participant {
	if s == nil {
		return nil
	}
	out := make([]Participant, 0, len(s.Participants))
	for _, p := range s.Participants {
		if p.Active {
			out = append(out, p)
		}
	}
	return out
}

func (s *Snapshot) ActiveCount() uint64 {
	return uint64(len(s.ActiveParticipants()))
}

func DevnetSnapshot(coinbase common.Address, blockNumber uint64, activityWindow uint64) *Snapshot {
	reg := DevnetRegistry(coinbase)
	reg.MarkSeen(coinbase, blockNumber)
	reg.ApplyActivityWindow(blockNumber, activityWindow)
	return reg.ToSnapshot(blockNumber)
}
