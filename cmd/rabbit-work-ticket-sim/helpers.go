package main

import (
	"encoding/binary"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/crypto"
)

func countRoles(counter *metricCounter, attacker []bool, opts options) {
	if len(attacker) == 0 {
		return
	}
	if attacker[0] {
		counter.producer++
	}
	fallbackEnd := 1 + int(opts.fallbacks)
	if fallbackEnd > len(attacker) {
		fallbackEnd = len(attacker)
	}
	allFront := attacker[0]
	for _, controlled := range attacker[1:fallbackEnd] {
		counter.fallbackSeats++
		if controlled {
			counter.attackerFallbackSeats++
		} else {
			allFront = false
		}
	}
	if allFront {
		counter.producerFallbackFull++
	}

	committeeSize := lqc.ComputeCommitteeSizeWithBounds(
		uint64(len(attacker)),
		opts.committeeMin,
		opts.committeeMax,
	)
	committeeEnd := fallbackEnd + int(committeeSize)
	if committeeEnd > len(attacker) {
		committeeEnd = len(attacker)
	}
	controlledCommittee := uint64(0)
	for _, controlled := range attacker[fallbackEnd:committeeEnd] {
		counter.committeeSeats++
		if controlled {
			counter.attackerCommitteeSeats++
			controlledCommittee++
		}
	}
	seats := uint64(committeeEnd - fallbackEnd)
	if seats > 0 && controlledCommittee*2 > seats {
		counter.committeeMajority++
	}
}

func parsePositiveList(value string) ([]int, error) {
	var values []int
	seen := make(map[int]bool)
	for _, field := range strings.Split(value, ",") {
		parsed, err := strconv.Atoi(strings.TrimSpace(field))
		if err != nil || parsed <= 0 {
			return nil, fmt.Errorf("invalid value %q", field)
		}
		if !seen[parsed] {
			seen[parsed] = true
			values = append(values, parsed)
		}
	}
	if len(values) == 0 {
		return nil, fmt.Errorf("empty list")
	}
	return values, nil
}

func percentage(numerator, denominator uint64) float64 {
	if denominator == 0 {
		return 0
	}
	return roundPercent(float64(numerator) / float64(denominator))
}

func roundPercent(ratio float64) float64 {
	return math.Round(ratio*10000) / 100
}

func spread(values []float64) float64 {
	return max(values) - min(values)
}

func min(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	result := values[0]
	for _, value := range values[1:] {
		if value < result {
			result = value
		}
	}
	return result
}

func max(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	result := values[0]
	for _, value := range values[1:] {
		if value > result {
			result = value
		}
	}
	return result
}

func uint64Bytes(value uint64) []byte {
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], value)
	return encoded[:]
}

func nextHash(parent common.Hash, slot uint64) common.Hash {
	return crypto.Keccak256Hash(parent.Bytes(), uint64Bytes(slot), []byte("NEXT"))
}
