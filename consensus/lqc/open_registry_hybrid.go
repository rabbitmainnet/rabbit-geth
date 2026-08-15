package lqc

import (
	"bytes"
	"encoding/binary"
	"math/big"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
)

type HybridParticipantStatus uint8

const (
	ParticipantPendingActivation HybridParticipantStatus = iota
	ParticipantActiveCandidate
	ParticipantJailed
	ParticipantExited
)

type HybridParticipant struct {
	Address       common.Address
	Payout        common.Address
	NodeID        string
	Bond          *big.Int
	RegisteredAt  uint64
	LastHeartbeat uint64
	JailedUntil   uint64
	MissedTurns   uint64
	Status        HybridParticipantStatus
	IsBootstrap   bool
}

type HybridLQCConfig struct {
	MinBond            *big.Int
	ActivationDelay    uint64
	HeartbeatWindow    uint64
	HeartbeatGrace     uint64
	CommitteeSize      uint64
	FallbackCount      uint64
	JailBlocks         uint64
	MaxMissedTurns     uint64
	MinorSlashBps      uint64
	MajorSlashBps      uint64
	BootstrapOnlyUntil uint64
}

type HybridSelection struct {
	Ordered   []HybridParticipant
	Producer  *HybridParticipant
	Fallbacks []HybridParticipant
	Committee []HybridParticipant
}

type participantScore struct {
	P     HybridParticipant
	Score common.Hash
}

func DefaultDevnetBootstrapParticipants() []common.Address {
	return []common.Address{}
}

func NormalizeBootstrapParticipants(in []common.Address) []common.Address {
	if len(in) > 0 {
		out := make([]common.Address, 0, len(in))
		for _, a := range in {
			if a != (common.Address{}) {
				out = append(out, a)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	return DefaultDevnetBootstrapParticipants()
}

func cloneParticipant(p HybridParticipant) HybridParticipant {
	cp := p
	if p.Bond != nil {
		cp.Bond = new(big.Int).Set(p.Bond)
	}
	return cp
}

func normalizeHybridConfig(cfg HybridLQCConfig) HybridLQCConfig {
	if cfg.MinBond == nil {
		cfg.MinBond = big.NewInt(25)
	} else {
		cfg.MinBond = new(big.Int).Set(cfg.MinBond)
	}
	// ActivationDelay=0 é um valor válido (sem atraso).
	// Não substituir automaticamente por 64.

	if cfg.HeartbeatWindow == 0 {
		cfg.HeartbeatWindow = 64
	}
	if cfg.HeartbeatGrace == 0 {
		cfg.HeartbeatGrace = 16
	}
	if cfg.CommitteeSize == 0 {
		cfg.CommitteeSize = 6
	}
	if cfg.FallbackCount == 0 {
		cfg.FallbackCount = 2
	}
	if cfg.JailBlocks == 0 {
		cfg.JailBlocks = 256
	}
	if cfg.MaxMissedTurns == 0 {
		cfg.MaxMissedTurns = 3
	}
	if cfg.MinorSlashBps == 0 {
		cfg.MinorSlashBps = 500
	}
	if cfg.MajorSlashBps == 0 {
		cfg.MajorSlashBps = 2000
	}
	return cfg
}

func synthesizeBootstrapParticipants(bootstrap []common.Address, block uint64, cfg HybridLQCConfig) []HybridParticipant {
	cfg = normalizeHybridConfig(cfg)
	out := make([]HybridParticipant, 0, len(bootstrap))
	for _, addr := range bootstrap {
		if addr == (common.Address{}) {
			continue
		}
		out = append(out, HybridParticipant{
			Address:       addr,
			Payout:        addr,
			Bond:          new(big.Int).Set(cfg.MinBond),
			RegisteredAt:  0,
			LastHeartbeat: block,
			Status:        ParticipantActiveCandidate,
			IsBootstrap:   true,
		})
	}
	return out
}

func MergeParticipants(bootstrap []common.Address, onchain []HybridParticipant, block uint64, cfg HybridLQCConfig) []HybridParticipant {
	cfg = normalizeHybridConfig(cfg)

	byAddr := make(map[common.Address]HybridParticipant)

	bootstrapSet := synthesizeBootstrapParticipants(bootstrap, block, cfg)

	if block <= cfg.BootstrapOnlyUntil {
		for _, p := range bootstrapSet {
			byAddr[p.Address] = p
		}

		for _, p := range onchain {
			cp := cloneParticipant(p)
			if cp.Address != (common.Address{}) {
				byAddr[cp.Address] = cp
			}
		}
	} else {
		for _, p := range bootstrapSet {
			byAddr[p.Address] = p
		}
		for _, p := range onchain {
			cp := cloneParticipant(p)
			if cp.Address == (common.Address{}) {
				continue
			}
			if cp.Payout == (common.Address{}) {
				cp.Payout = cp.Address
			}
			if cp.Bond == nil {
				cp.Bond = big.NewInt(0)
			}
			if existing, ok := byAddr[cp.Address]; ok {
				cp.IsBootstrap = existing.IsBootstrap || cp.IsBootstrap
			}
			byAddr[cp.Address] = cp
		}
	}

	out := make([]HybridParticipant, 0, len(byAddr))
	for _, p := range byAddr {
		out = append(out, p)
	}

	sort.Slice(out, func(i, j int) bool {
		return bytes.Compare(out[i].Address.Bytes(), out[j].Address.Bytes()) < 0
	})
	return out
}

func IsHybridEligible(p HybridParticipant, block uint64, cfg HybridLQCConfig) bool {
	cfg = normalizeHybridConfig(cfg)

	log.Info("LQC ELIGIBILITY",
		"addr", p.Address.Hex(),
		"block", block,
		"status", p.Status,
		"registeredAt", p.RegisteredAt,
		"heartbeat", p.LastHeartbeat,
		"jailedUntil", p.JailedUntil,
		"bond", p.Bond,
		"activationDelay", cfg.ActivationDelay,
		"heartbeatWindow", cfg.HeartbeatWindow,
		"heartbeatGrace", cfg.HeartbeatGrace,
	)

	if p.Address == (common.Address{}) {
		log.Info("LQC FAIL", "reason", "zero-address")
		return false
	}
	if p.Status == ParticipantExited {
		log.Info("LQC FAIL", "reason", "exited")
		return false
	}
	if p.JailedUntil > block {
		log.Info("LQC FAIL", "reason", "jailed-until")
		return false
	}
	if p.Status == ParticipantJailed {
		log.Info("LQC FAIL", "reason", "status-jailed")
		return false
	}
	if p.Bond == nil || p.Bond.Cmp(cfg.MinBond) < 0 {
		log.Info("LQC FAIL", "reason", "bond")
		return false
	}
	if block < p.RegisteredAt+cfg.ActivationDelay {
		log.Info("LQC FAIL", "reason", "activation-delay")
		return false
	}
	if p.LastHeartbeat == 0 {
		// Permite que participantes recém-criados produzam o primeiro bloco.
		if block > 1 {
			log.Info("LQC FAIL", "reason", "heartbeat-zero")
			return false
		}
	}
	if block > p.LastHeartbeat+cfg.HeartbeatWindow+cfg.HeartbeatGrace {
		log.Info("LQC FAIL", "reason", "heartbeat-expired")
		return false
	}
	if p.Status != ParticipantActiveCandidate && p.Status != ParticipantPendingActivation {
		log.Info("LQC FAIL", "reason", "status")
		return false
	}
	return true
}

func EligibleHybridParticipants(input []HybridParticipant, block uint64, cfg HybridLQCConfig) []HybridParticipant {
	cfg = normalizeHybridConfig(cfg)

	out := make([]HybridParticipant, 0, len(input))
	for _, p := range input {

		bond := "<nil>"
		if p.Bond != nil {
			bond = p.Bond.String()
		}

		log.Info("LQC PARTICIPANT",
			"addr", p.Address.Hex(),
			"status", p.Status,
			"bond", bond,
			"registered", p.RegisteredAt,
			"heartbeat", p.LastHeartbeat,
			"jailedUntil", p.JailedUntil,
		)

		if IsHybridEligible(p, block, cfg) {
			out = append(out, cloneParticipant(p))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return bytes.Compare(out[i].Address.Bytes(), out[j].Address.Bytes()) < 0
	})
	return out
}

func buildSelectionSeed(parentHash common.Hash, block uint64) common.Hash {
	var n [8]byte
	binary.BigEndian.PutUint64(n[:], block)
	return crypto.Keccak256Hash(parentHash.Bytes(), n[:])
}

func DeterministicallyOrderParticipants(input []HybridParticipant, parentHash common.Hash, block uint64) []HybridParticipant {
	if len(input) == 0 {
		return nil
	}
	seed := buildSelectionSeed(parentHash, block)

	scored := make([]participantScore, 0, len(input))
	for _, p := range input {
		score := crypto.Keccak256Hash(seed.Bytes(), p.Address.Bytes())
		scored = append(scored, participantScore{
			P:     cloneParticipant(p),
			Score: score,
		})
	}

	sort.Slice(scored, func(i, j int) bool {
		cmp := bytes.Compare(scored[i].Score.Bytes(), scored[j].Score.Bytes())
		if cmp != 0 {
			return cmp < 0
		}
		return bytes.Compare(scored[i].P.Address.Bytes(), scored[j].P.Address.Bytes()) < 0
	})

	out := make([]HybridParticipant, len(scored))
	for i, s := range scored {
		out[i] = s.P
	}
	return out
}

func BuildHybridSelection(bootstrap []common.Address, onchain []HybridParticipant, parentHash common.Hash, block uint64, cfg HybridLQCConfig) HybridSelection {
	cfg = normalizeHybridConfig(cfg)

	merged := MergeParticipants(bootstrap, onchain, block, cfg)

	eligible := EligibleHybridParticipants(merged, block, cfg)

	ordered := DeterministicallyOrderParticipants(eligible, parentHash, block)

	sel := HybridSelection{
		Ordered: ordered,
	}
	if len(ordered) == 0 {
		return sel
	}

	sel.Producer = &ordered[0]

	fallbackEnd := 1 + int(cfg.FallbackCount)
	if fallbackEnd > len(ordered) {
		fallbackEnd = len(ordered)
	}
	if fallbackEnd > 1 {
		sel.Fallbacks = append(sel.Fallbacks, ordered[1:fallbackEnd]...)
	}

	committeeStart := fallbackEnd
	committeeEnd := committeeStart + int(cfg.CommitteeSize)
	if committeeEnd > len(ordered) {
		committeeEnd = len(ordered)
	}
	if committeeEnd > committeeStart {
		sel.Committee = append(sel.Committee, ordered[committeeStart:committeeEnd]...)
	}

	return sel
}

func BuildDevnetBootstrapSelection(parentHash common.Hash, block uint64, cfg HybridLQCConfig) HybridSelection {
	cfg = normalizeHybridConfig(cfg)
	bootstrap := []common.Address{}

	parts := make([]HybridParticipant, 0, len(bootstrap))
	for _, addr := range bootstrap {
		if addr == (common.Address{}) {
			continue
		}
		parts = append(parts, HybridParticipant{
			Address:       addr,
			Payout:        addr,
			Bond:          new(big.Int).Set(cfg.MinBond),
			RegisteredAt:  0,
			LastHeartbeat: block,
			Status:        ParticipantActiveCandidate,
			IsBootstrap:   true,
		})
	}

	ordered := DeterministicallyOrderParticipants(parts, parentHash, block)
	sel := HybridSelection{Ordered: ordered}
	if len(ordered) == 0 {
		return sel
	}

	sel.Producer = &ordered[0]

	fallbackEnd := 1 + int(cfg.FallbackCount)
	if fallbackEnd > len(ordered) {
		fallbackEnd = len(ordered)
	}
	if fallbackEnd > 1 {
		sel.Fallbacks = append(sel.Fallbacks, ordered[1:fallbackEnd]...)
	}

	committeeStart := fallbackEnd
	committeeEnd := committeeStart + int(cfg.CommitteeSize)
	if committeeEnd > len(ordered) {
		committeeEnd = len(ordered)
	}
	if committeeEnd > committeeStart {
		sel.Committee = append(sel.Committee, ordered[committeeStart:committeeEnd]...)
	}

	return sel
}

func ProducerAddress(sel HybridSelection) common.Address {
	if sel.Producer == nil {
		return common.Address{}
	}
	return sel.Producer.Address
}

func FallbackAddresses(sel HybridSelection) []common.Address {
	out := make([]common.Address, 0, len(sel.Fallbacks))
	for _, p := range sel.Fallbacks {
		out = append(out, p.Address)
	}
	return out
}

func IsAuthorAllowed(sel HybridSelection, author common.Address) (bool, int) {
	// Toda a fila determinística pode assumir a produção.
	// A posição define a janela mínima de publicação.
	for queuePos, participant := range sel.Ordered {
		if participant.Address == author {
			return true, queuePos
		}
	}
	return false, -1
}

func ApplySuccessfulTurn(p HybridParticipant) HybridParticipant {
	out := cloneParticipant(p)
	out.MissedTurns = 0
	if out.Status == ParticipantJailed {
		out.Status = ParticipantActiveCandidate
	}
	return out
}

func ApplyMissedTurnPenalty(p HybridParticipant, block uint64, cfg HybridLQCConfig) HybridParticipant {
	cfg = normalizeHybridConfig(cfg)

	out := cloneParticipant(p)
	out.MissedTurns++

	if out.MissedTurns >= cfg.MaxMissedTurns {
		out.Status = ParticipantJailed
		out.JailedUntil = block + cfg.JailBlocks

		if out.Bond != nil && out.Bond.Sign() > 0 {
			slash := new(big.Int).Mul(out.Bond, new(big.Int).SetUint64(cfg.MinorSlashBps))
			slash.Div(slash, big.NewInt(10000))
			if slash.Sign() > 0 && out.Bond.Cmp(slash) >= 0 {
				out.Bond.Sub(out.Bond, slash)
			} else {
				out.Bond = big.NewInt(0)
			}
		}
	}
	return out
}

func SplitReward(total *big.Int, producerBps uint64) (producer *big.Int, committee *big.Int) {
	if total == nil {
		return big.NewInt(0), big.NewInt(0)
	}
	if producerBps > 10000 {
		producerBps = 10000
	}
	producer = new(big.Int).Mul(total, new(big.Int).SetUint64(producerBps))
	producer.Div(producer, big.NewInt(10000))
	committee = new(big.Int).Sub(new(big.Int).Set(total), producer)
	return producer, committee
}
