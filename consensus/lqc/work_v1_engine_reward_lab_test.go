//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package lqc

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/holiman/uint256"
)

func workV1RewardAmountForV1(
	credits []WorkSeatRewardV1,
	address common.Address,
) *uint256.Int {
	for _, credit := range credits {
		if credit.Address == address {
			return new(uint256.Int).Set(credit.Amount)
		}
	}
	return uint256.NewInt(0)
}

func TestWorkV1EngineLabRewardRepeatedCommitteeSeatsKeepWeight(
	t *testing.T,
) {
	producer := common.HexToAddress(
		"0x00000000000000000000000000000000000000a1",
	)
	other := common.HexToAddress(
		"0x00000000000000000000000000000000000000b2",
	)

	// 70% producer + 30% committee.
	// Committee seats = [producer, producer, other].
	// Therefore producer receives 7000 + 1000 + 1000 = 9000.
	credits := workV1EngineLabSeatRewardCredits(
		uint256.NewInt(10_000),
		producer,
		[]HybridParticipant{
			{Address: producer},
			{Address: producer},
			{Address: other},
		},
		3000,
	)

	if got := workV1RewardAmountForV1(
		credits,
		producer,
	); got.Uint64() != 9000 {
		t.Fatalf("producer aggregate=%s want=9000", got)
	}
	if got := workV1RewardAmountForV1(
		credits,
		other,
	); got.Uint64() != 1000 {
		t.Fatalf("other aggregate=%s want=1000", got)
	}
	if got := workV1EngineLabRewardTotalV1(
		credits,
	); got.Uint64() != 10_000 {
		t.Fatalf("total=%s want=10000", got)
	}
}

func TestWorkV1EngineLabRewardRemainderConservesEveryWeiBySeat(
	t *testing.T,
) {
	producer := common.HexToAddress(
		"0x00000000000000000000000000000000000000a1",
	)
	other := common.HexToAddress(
		"0x00000000000000000000000000000000000000b2",
	)

	credits := workV1EngineLabSeatRewardCredits(
		uint256.NewInt(10_001),
		producer,
		[]HybridParticipant{
			{Address: producer},
			{Address: producer},
			{Address: other},
		},
		3000,
	)

	// producer share = floor(10001*70%) = 7000
	// committee = 3001; seat shares = 1001,1000,1000
	// producer total = 7000+1001+1000 = 9001.
	if got := workV1RewardAmountForV1(
		credits,
		producer,
	); got.Uint64() != 9001 {
		t.Fatalf("producer aggregate=%s want=9001", got)
	}
	if got := workV1RewardAmountForV1(
		credits,
		other,
	); got.Uint64() != 1000 {
		t.Fatalf("other aggregate=%s want=1000", got)
	}
	if got := workV1EngineLabRewardTotalV1(
		credits,
	); got.Uint64() != 10_001 {
		t.Fatalf("total=%s want=10001", got)
	}
}

func TestWorkV1EngineLabRewardProducerGetsFullWithoutCommittee(
	t *testing.T,
) {
	producer := common.HexToAddress(
		"0x00000000000000000000000000000000000000a1",
	)
	credits := workV1EngineLabSeatRewardCredits(
		uint256.NewInt(12345),
		producer,
		nil,
		3000,
	)
	if got := workV1RewardAmountForV1(
		credits,
		producer,
	); got.Uint64() != 12345 {
		t.Fatalf("producer=%s want=12345", got)
	}
	if got := workV1EngineLabRewardTotalV1(
		credits,
	); got.Uint64() != 12345 {
		t.Fatalf("total=%s want=12345", got)
	}
}

func TestWorkV1EngineLabAuthorizedFallbackReceivesProducerShare(
	t *testing.T,
) {
	producer := common.HexToAddress(
		"0x00000000000000000000000000000000000000a1",
	)
	fallback := common.HexToAddress(
		"0x00000000000000000000000000000000000000b2",
	)
	committee := common.HexToAddress(
		"0x00000000000000000000000000000000000000c3",
	)
	selection := HybridSelection{
		Producer: &HybridParticipant{Address: producer},
		Ordered: []HybridParticipant{
			{Address: producer},
			{Address: fallback},
			{Address: committee},
		},
		Fallbacks: []HybridParticipant{{Address: fallback}},
		Committee: []HybridParticipant{{Address: committee}},
	}

	credits, ok := workV1EngineLabRewardCreditsForAuthor(
		selection,
		fallback,
		uint256.NewInt(10_000),
		3000,
	)
	if !ok {
		t.Fatal("authorized fallback reward rejected")
	}
	if got := workV1RewardAmountForV1(
		credits,
		fallback,
	); got.Uint64() != 7000 {
		t.Fatalf("fallback producer share=%s want=7000", got)
	}
	if got := workV1RewardAmountForV1(
		credits,
		committee,
	); got.Uint64() != 3000 {
		t.Fatalf("committee share=%s want=3000", got)
	}
	if _, ok := workV1EngineLabRewardCreditsForAuthor(
		selection,
		common.HexToAddress("0x00000000000000000000000000000000000000d4"),
		uint256.NewInt(10_000),
		3000,
	); ok {
		t.Fatal("unauthorized author received Work V1 reward")
	}
}

func TestWorkV1EngineLabShortSeatPoolReservesCommitteeReward(
	t *testing.T,
) {
	a := common.HexToAddress(
		"0x00000000000000000000000000000000000000a1",
	)
	b := common.HexToAddress(
		"0x00000000000000000000000000000000000000b2",
	)
	seats := []WorkSeatV1{
		{
			TicketHash:  crypto.Keccak256Hash([]byte("seat-a-1")),
			Participant: a,
		},
		{
			TicketHash:  crypto.Keccak256Hash([]byte("seat-a-2")),
			Participant: a,
		},
		{
			TicketHash:  crypto.Keccak256Hash([]byte("seat-b-1")),
			Participant: b,
		},
		{
			TicketHash:  crypto.Keccak256Hash([]byte("seat-b-2")),
			Participant: b,
		},
	}

	snapshot, err := NewBootstrapRegistrySnapshot(
		256,
		crypto.Keccak256Hash([]byte("live-registry-256")),
		[]common.Address{a, b},
	)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := snapshot.Registry()
	if err != nil {
		t.Fatal(err)
	}
	engine := &LQC{
		config: canonicalRegistryEngineConfig([]common.Address{a, b}, 1),
	}
	engine.config.FallbackCount = 5
	engine.config.CommitteeSize = 2

	// Reproduces the live gate: 4 WorkSeats, fallbackCount=5 and committee=2.
	// The old LAB wiring consumed every non-producer seat as a fallback, leaving
	// no committee and paying the full subsidy to the block producer.
	selection, active, err := engine.workV1EngineLabBuildSeatSelection(
		big.NewInt(928),
		1,
		crypto.Keccak256Hash([]byte("closed-selection-root")),
		seats,
		crypto.Keccak256Hash([]byte("dataset-key")),
		257,
		registry,
		RegistrySnapshotRules{
			ActivationDelay: 0,
			HeartbeatWindow: 128,
			HeartbeatGrace:  16,
		},
		func(key common.Hash, input []byte) (common.Hash, error) {
			return crypto.Keccak256Hash(key.Bytes(), input), nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !active {
		t.Fatal("non-empty eligible WorkSeat set fell back to registry")
	}
	if selection.Producer == nil {
		t.Fatal("missing producer")
	}
	if len(selection.Fallbacks) != 1 || len(selection.Committee) != 2 {
		t.Fatalf(
			"roles fallback=%d committee=%d want=1/2",
			len(selection.Fallbacks),
			len(selection.Committee),
		)
	}

	credits := workV1EngineLabSeatRewardCredits(
		uint256.NewInt(1_200_000_000_000_000_000),
		selection.Producer.Address,
		selection.Committee,
		3000,
	)
	if got := workV1RewardAmountForV1(credits, a); got.IsZero() {
		t.Fatal("participant A received no WorkSeat reward")
	}
	if got := workV1RewardAmountForV1(credits, b); got.IsZero() {
		t.Fatal("participant B received no WorkSeat reward")
	}
	if got := workV1EngineLabRewardTotalV1(credits); got.Cmp(
		uint256.NewInt(1_200_000_000_000_000_000),
	) != 0 {
		t.Fatalf("total=%s want=1200000000000000000", got)
	}
}

func TestWorkV1EngineLabZeroWorkPolicyIsRegistryNoSubsidy(
	t *testing.T,
) {
	if got := workV1EngineLabRewardModeForSource(
		false,
		false,
	); got != workV1EngineLabRewardLegacy {
		t.Fatalf("pre-work mode=%d", got)
	}
	if got := workV1EngineLabRewardModeForSource(
		true,
		true,
	); got != workV1EngineLabRewardSeats {
		t.Fatalf("seat mode=%d", got)
	}
	if got := workV1EngineLabRewardModeForSource(
		true,
		false,
	); got != workV1EngineLabRewardEmergencyNoSubsidy {
		t.Fatalf("zero-work mode=%d", got)
	}
}
