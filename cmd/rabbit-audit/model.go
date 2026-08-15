package main

import (
	"bytes"
	"encoding/binary"
	"math"
	"math/big"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
)

const (
	defaultEraLength    uint64 = 8_409_600
	defaultCommitteeBPS uint64 = 3_000
)

var (
	vestingSystemAddress = common.HexToAddress("0x0000000000000000000000000000000000001001")
	weiPerRAB            = new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
	rewardByEra          = []*big.Int{
		mustBig("1200000000000000000"),
		mustBig("600000000000000000"),
		mustBig("300000000000000000"),
		mustBig("150000000000000000"),
	}
)

type selectionConfig struct {
	FallbackCount    uint64
	CommitteeSize    uint64
	CommitteeMin     uint64
	CommitteeMax     uint64
	DynamicCommittee bool
}

type rewardAllocation struct {
	Address common.Address
	Role    string
	Amount  *big.Int
}

type accountState struct {
	Balance  *big.Int
	Locked   *big.Int
	Original *big.Int
	Stage    uint8
}

func mustBig(value string) *big.Int {
	n, ok := new(big.Int).SetString(value, 10)
	if !ok {
		panic("invalid integer constant: " + value)
	}
	return n
}

func cloneBig(value *big.Int) *big.Int {
	if value == nil {
		return new(big.Int)
	}
	return new(big.Int).Set(value)
}

func cloneAccountState(value accountState) accountState {
	return accountState{
		Balance:  cloneBig(value.Balance),
		Locked:   cloneBig(value.Locked),
		Original: cloneBig(value.Original),
		Stage:    value.Stage,
	}
}

func effectiveEraLength(config *params.LQCConfig) uint64 {
	if config != nil && config.EraLength > 0 {
		return config.EraLength
	}
	return defaultEraLength
}

func effectiveCommitteeBPS(config *params.LQCConfig) uint64 {
	ratio := defaultCommitteeBPS
	if config != nil && config.CommitteeRatioBps > 0 {
		ratio = config.CommitteeRatioBps
	}
	if ratio > 10_000 {
		return 10_000
	}
	return ratio
}

func effectiveSelectionConfig(config *params.LQCConfig) selectionConfig {
	result := selectionConfig{
		FallbackCount:    2,
		CommitteeMin:     32,
		CommitteeMax:     128,
		DynamicCommittee: true,
	}
	if config == nil {
		return result
	}
	if config.FallbackCount > 0 {
		result.FallbackCount = config.FallbackCount
	} else if config.FallbackSlots > 0 {
		result.FallbackCount = config.FallbackSlots
	}
	if config.CommitteeSize > 0 {
		result.CommitteeSize = config.CommitteeSize
		result.DynamicCommittee = false
	}
	if config.CommitteeMin > 0 {
		result.CommitteeMin = config.CommitteeMin
	}
	if config.CommitteeMax > 0 {
		result.CommitteeMax = config.CommitteeMax
	}
	return result
}

func (config selectionConfig) canonicalCommitteeSize(active uint64) uint64 {
	if !config.DynamicCommittee {
		return config.CommitteeSize
	}
	size := (active + 9) / 10
	if config.CommitteeMin > 0 && size < config.CommitteeMin {
		size = config.CommitteeMin
	}
	if config.CommitteeMax > 0 && size > config.CommitteeMax {
		size = config.CommitteeMax
	}
	return size
}

func (config selectionConfig) legacyCommitteeSize() uint64 {
	if !config.DynamicCommittee {
		return config.CommitteeSize
	}
	if config.CommitteeMax > 0 {
		return config.CommitteeMax
	}
	return config.CommitteeMin
}

// legacySelectionConfig models the historical bounded-selection rule and is retained only for
// regression tests. The active LQC engine uses effectiveSelectionConfig.
func legacySelectionConfig(active int) selectionConfig {
	if active <= 1 {
		return selectionConfig{}
	}
	fallbacks := int(math.Ceil(math.Log2(float64(active))))
	if fallbacks < 2 {
		fallbacks = 2
	}
	committee := int(math.Ceil(math.Sqrt(float64(active))))
	if committee < 5 {
		committee = 5
	}
	if fallbacks > active-1 {
		fallbacks = active - 1
	}
	remaining := active - 1 - fallbacks
	if remaining < 0 {
		remaining = 0
	}
	if committee > remaining {
		committee = remaining
	}
	return selectionConfig{
		FallbackCount: uint64(fallbacks),
		CommitteeSize: uint64(committee),
	}
}

func rewardForBlock(block, eraLength uint64) (*big.Int, uint64) {
	if eraLength == 0 {
		eraLength = defaultEraLength
	}
	era := block / eraLength
	index := era
	if index >= uint64(len(rewardByEra)) {
		index = uint64(len(rewardByEra) - 1)
	}
	return cloneBig(rewardByEra[index]), era
}

func scheduledRewards(from, to, eraLength uint64) *big.Int {
	total := new(big.Int)
	if from == 0 {
		from = 1
	}
	if to < from {
		return total
	}
	for cursor := from; cursor <= to; {
		reward, era := rewardForBlock(cursor, eraLength)
		nextEra := (era + 1) * eraLength
		end := to
		if nextEra > 0 && nextEra-1 < end {
			end = nextEra - 1
		}
		count := new(big.Int).SetUint64(end - cursor + 1)
		total.Add(total, new(big.Int).Mul(reward, count))
		if end == to {
			break
		}
		cursor = end + 1
	}
	return total
}

func deterministicQueue(participants []common.Address, parentHash common.Hash, block uint64) []common.Address {
	unique := make(map[common.Address]struct{})
	for _, address := range participants {
		if address != (common.Address{}) {
			unique[address] = struct{}{}
		}
	}
	addresses := make([]common.Address, 0, len(unique))
	for address := range unique {
		addresses = append(addresses, address)
	}
	sort.Slice(addresses, func(i, j int) bool {
		return bytes.Compare(addresses[i].Bytes(), addresses[j].Bytes()) < 0
	})
	var number [8]byte
	binary.BigEndian.PutUint64(number[:], block)
	seed := crypto.Keccak256Hash(parentHash.Bytes(), number[:])
	type scoredAddress struct {
		Address common.Address
		Score   common.Hash
	}
	scored := make([]scoredAddress, 0, len(addresses))
	for _, address := range addresses {
		scored = append(scored, scoredAddress{
			Address: address,
			Score:   crypto.Keccak256Hash(seed.Bytes(), address.Bytes()),
		})
	}
	sort.Slice(scored, func(i, j int) bool {
		if comparison := bytes.Compare(scored[i].Score.Bytes(), scored[j].Score.Bytes()); comparison != 0 {
			return comparison < 0
		}
		return bytes.Compare(scored[i].Address.Bytes(), scored[j].Address.Bytes()) < 0
	})
	ordered := make([]common.Address, len(scored))
	for i := range scored {
		ordered[i] = scored[i].Address
	}
	return ordered
}

func rewardCommittee(queue []common.Address, producer common.Address, config selectionConfig) []common.Address {
	start := 1 + int(config.FallbackCount)
	if start > len(queue) {
		start = len(queue)
	}
	end := start + int(config.legacyCommitteeSize())
	if end > len(queue) {
		end = len(queue)
	}
	seen := make(map[common.Address]struct{})
	committee := make([]common.Address, 0, end-start)
	for _, address := range queue[start:end] {
		if address == (common.Address{}) || address == producer {
			continue
		}
		if _, exists := seen[address]; exists {
			continue
		}
		seen[address] = struct{}{}
		committee = append(committee, address)
	}
	return committee
}

// canonicalRewardCommittee mirrors the post-registry selection rule. The
// committee is selected from the parent registry queue. The actual author may
// be a fallback at any queue position, so it is removed from the committee
// exactly as consensus/lqc does before distributing rewards.
func canonicalRewardCommittee(queue []common.Address, producer common.Address, config selectionConfig) []common.Address {
	start := 1 + int(config.FallbackCount)
	if start > len(queue) {
		start = len(queue)
	}
	end := start + int(config.canonicalCommitteeSize(uint64(len(queue))))
	if end > len(queue) {
		end = len(queue)
	}
	seen := make(map[common.Address]struct{})
	committee := make([]common.Address, 0, end-start)
	for _, address := range queue[start:end] {
		if address == (common.Address{}) || address == producer {
			continue
		}
		if _, exists := seen[address]; exists {
			continue
		}
		seen[address] = struct{}{}
		committee = append(committee, address)
	}
	return committee
}

func allocateReward(total *big.Int, producer common.Address, committee []common.Address, committeeBPS uint64) []rewardAllocation {
	if committeeBPS > 10_000 {
		committeeBPS = 10_000
	}
	if len(committee) == 0 || committeeBPS == 0 {
		return []rewardAllocation{{Address: producer, Role: "producer", Amount: cloneBig(total)}}
	}
	producerBPS := 10_000 - committeeBPS
	producerReward := new(big.Int).Mul(total, new(big.Int).SetUint64(producerBPS))
	producerReward.Div(producerReward, big.NewInt(10_000))
	committeeReward := new(big.Int).Sub(cloneBig(total), producerReward)
	perMember := new(big.Int).Div(cloneBig(committeeReward), new(big.Int).SetUint64(uint64(len(committee))))
	allocated := new(big.Int).Mul(cloneBig(perMember), new(big.Int).SetUint64(uint64(len(committee))))
	remainder := new(big.Int).Sub(cloneBig(committeeReward), allocated)
	result := []rewardAllocation{{Address: producer, Role: "producer", Amount: producerReward}}
	for i, address := range committee {
		amount := cloneBig(perMember)
		if i == 0 {
			amount.Add(amount, remainder)
		}
		if amount.Sign() > 0 {
			result = append(result, rewardAllocation{Address: address, Role: "committee", Amount: amount})
		}
	}
	return result
}

// creditExpected models the frozen Rabbit rule: every mining reward is liquid
// immediately. Legacy vesting storage is observed only to prove that consensus
// did not mutate it.
func creditExpected(state *accountState, amount *big.Int) {
	if amount == nil || amount.Sign() == 0 {
		return
	}
	state.Balance.Add(state.Balance, amount)
}

func storageSlot(prefix string, address common.Address) common.Hash {
	return crypto.Keccak256Hash([]byte(prefix), address.Bytes())
}

func lockedBalanceSlot(address common.Address) common.Hash {
	return storageSlot("RABBIT_VESTING_LOCKED_BALANCE", address)
}

func originalLockedBalanceSlot(address common.Address) common.Hash {
	return storageSlot("RABBIT_VESTING_ORIGINAL_BALANCE", address)
}

func releasedStageSlot(address common.Address) common.Hash {
	return storageSlot("RABBIT_VESTING_RELEASE_STAGE", address)
}

func vestingIndexCountSlot() common.Hash {
	return crypto.Keccak256Hash([]byte("RABBIT_VESTING_INDEX_COUNT"))
}

func vestingIndexSlot(index uint64) common.Hash {
	return crypto.Keccak256Hash([]byte("RABBIT_VESTING_INDEX"), new(big.Int).SetUint64(index).Bytes())
}

func formatRAB(wei *big.Int) string {
	if wei == nil {
		return "0"
	}
	sign := ""
	value := cloneBig(wei)
	if value.Sign() < 0 {
		sign = "-"
		value.Abs(value)
	}
	whole := new(big.Int).Div(cloneBig(value), weiPerRAB)
	fraction := new(big.Int).Mod(value, weiPerRAB)
	if fraction.Sign() == 0 {
		return sign + whole.String()
	}
	text := fraction.Text(10)
	for len(text) < 18 {
		text = "0" + text
	}
	for len(text) > 0 && text[len(text)-1] == '0' {
		text = text[:len(text)-1]
	}
	return sign + whole.String() + "." + text
}
