package lqc

import (
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum/common"
)

type RegistryEntry struct {
	Address       common.Address
	Registered    bool
	Active        bool
	JoinedBlock   uint64
	LastSeenBlock uint64
	Weight        uint64
}

type Registry struct {
	Entries []RegistryEntry
}

var (
	runtimeRegistryMu sync.Mutex
	runtimeRegistry   = &Registry{}
)

func (r *Registry) Find(addr common.Address) *RegistryEntry {
	if r == nil {
		return nil
	}
	for i := range r.Entries {
		if r.Entries[i].Address == addr {
			return &r.Entries[i]
		}
	}
	return nil
}

func (r *Registry) Register(addr common.Address, blockNumber uint64) {
	if r == nil || addr == (common.Address{}) {
		return
	}

	existing := r.Find(addr)

	if existing != nil {
		existing.Registered = true
		existing.Active = true

		if existing.JoinedBlock == 0 {
			existing.JoinedBlock = blockNumber
		}

		if existing.Weight == 0 {
			existing.Weight = 1
		}

		existing.LastSeenBlock = blockNumber

		return
	}

	r.Entries = append(r.Entries, RegistryEntry{
		Address:       addr,
		Registered:    true,
		Active:        true,
		JoinedBlock:   blockNumber,
		LastSeenBlock: blockNumber,
		Weight:        1,
	})
}

func (r *Registry) MarkSeen(addr common.Address, blockNumber uint64) {
	if r == nil || addr == (common.Address{}) {
		return
	}
	entry := r.Find(addr)
	if entry == nil {
		r.Register(addr, blockNumber)
		entry = r.Find(addr)
		if entry == nil {
			return
		}
	}

	entry.Active = true
	entry.LastSeenBlock = blockNumber

	if entry.Weight == 0 {
		entry.Weight = 1
	}
}

func (r *Registry) ApplyActivityWindow(currentBlock uint64, activityWindow uint64) {
	if r == nil {
		return
	}
	for i := range r.Entries {
		e := &r.Entries[i]
		if !e.Registered {
			e.Active = false
			continue
		}
		if activityWindow == 0 {
			e.Active = true
			continue
		}
		if currentBlock >= e.LastSeenBlock && currentBlock-e.LastSeenBlock <= activityWindow {
			e.Active = true
		} else {
			e.Active = false
		}
	}
}

func (r *Registry) ActiveEntries() []RegistryEntry {
	if r == nil {
		return nil
	}
	out := make([]RegistryEntry, 0, len(r.Entries))
	for _, e := range r.Entries {
		if e.Registered && e.Active {
			out = append(out, e)
		}
	}
	return out
}

func (r *Registry) ToSnapshot(blockNumber uint64) *Snapshot {
	if r == nil {
		return nil
	}
	active := r.ActiveEntries()
	parts := make([]Participant, 0, len(active))
	for _, e := range active {
		parts = append(parts, Participant{
			Address:         e.Address,
			Active:          true,
			LastActiveBlock: e.LastSeenBlock,
			Weight:          e.Weight,
		})
	}
	return &Snapshot{
		BlockNumber:  blockNumber,
		Participants: parts,
	}
}

func (r *Registry) ToHybridParticipants() []HybridParticipant {
	if r == nil {
		return nil
	}
	out := make([]HybridParticipant, 0, len(r.Entries))
	for _, e := range r.Entries {
		status := ParticipantPendingActivation
		if e.Registered && e.Active {
			status = ParticipantActiveCandidate
		}
		out = append(out, HybridParticipant{
			Address:       e.Address,
			Payout:        e.Address,
			Bond:          big.NewInt(25),
			RegisteredAt:  e.JoinedBlock,
			LastHeartbeat: e.LastSeenBlock,
			JailedUntil:   0,
			MissedTurns:   0,
			Status:        status,
			IsBootstrap:   false,
		})
	}
	return out
}

func cloneRegistry(src *Registry) *Registry {
	if src == nil {
		return &Registry{}
	}
	out := &Registry{Entries: make([]RegistryEntry, 0, len(src.Entries))}
	out.Entries = append(out.Entries, src.Entries...)
	return out
}

func mergeRegistry(dst *Registry, src *Registry) {
	if dst == nil || src == nil {
		return
	}

	for _, e := range src.Entries {
		dst.Register(e.Address, e.JoinedBlock)

		existing := dst.Find(e.Address)
		if existing != nil {
			existing.Registered = true
			existing.Active = true

			if e.JoinedBlock > 0 {
				existing.JoinedBlock = e.JoinedBlock
			}

			if e.LastSeenBlock > 0 {
				existing.LastSeenBlock = e.LastSeenBlock
			}
		}
	}
}

func RuntimeRegistry() *Registry {
	runtimeRegistryMu.Lock()
	defer runtimeRegistryMu.Unlock()

	return cloneRegistry(runtimeRegistry)
}

func ResetRuntimeRegistry() {
	runtimeRegistryMu.Lock()
	defer runtimeRegistryMu.Unlock()
	runtimeRegistry = &Registry{}
}

func DevnetRegistry(coinbase common.Address) *Registry {
	if coinbase == (common.Address{}) {
		coinbase = common.HexToAddress("0x74b8db40d1bC5B590bB8dBB62dCc60Eb2DaD8f12")
	}
	r := &Registry{}
	r.Register(coinbase, 0)
	r.MarkSeen(coinbase, 0)
	return r
}

func BootstrapRegistry(addrs []common.Address, blockNumber uint64) *Registry {
	reg := &Registry{}

	for _, addr := range addrs {
		if addr == (common.Address{}) {
			continue
		}
		reg.Register(addr, 0)
		reg.MarkSeen(addr, blockNumber)
	}
	return reg
}

func RealRegistry(blockNumber uint64, bootstrap []common.Address, mode string) *Registry {
	reg := &Registry{}

	switch mode {
	case "bootstrap":
		mergeRegistry(reg, BootstrapRegistry(bootstrap, blockNumber))
	case "static":
		for _, addr := range StaticTestParticipants() {
			reg.Register(addr, 0)
			reg.MarkSeen(addr, blockNumber)
		}
	default:
		mergeRegistry(reg, BootstrapRegistry(bootstrap, blockNumber))
		mergeRegistry(reg, RuntimeRegistry())
		for _, addr := range StaticTestParticipants() {
			reg.Register(addr, 0)
			reg.MarkSeen(addr, blockNumber)
		}
	}

	return reg
}

func RegisterParticipant(reg *Registry, addr common.Address, blockNumber uint64) {
	runtimeRegistryMu.Lock()
	defer runtimeRegistryMu.Unlock()

	if runtimeRegistry == nil {
		runtimeRegistry = &Registry{}
	}

	runtimeRegistry.Register(addr, blockNumber)

}

func UpdateParticipantActivity(reg *Registry, addr common.Address, blockNumber uint64) {
	runtimeRegistryMu.Lock()
	defer runtimeRegistryMu.Unlock()
	if runtimeRegistry == nil {
		runtimeRegistry = &Registry{}
	}
	runtimeRegistry.MarkSeen(addr, blockNumber)
}

func EligibleParticipants(reg *Registry, blockNumber uint64, activityWindow uint64) []RegistryEntry {
	if reg == nil {
		return nil
	}
	reg.ApplyActivityWindow(blockNumber, activityWindow)
	return reg.ActiveEntries()
}
