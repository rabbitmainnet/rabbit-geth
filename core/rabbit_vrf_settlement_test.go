package core

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/misc"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/holiman/uint256"
)

var rabbitVRFTestSettlementProducerV1 = common.HexToAddress("0x0000000000000000000000000000000000009001")

var rabbitVRFTestSettlementParticipantsV1 = []common.Address{
	common.HexToAddress("0x0000000000000000000000000000000000009101"),
	common.HexToAddress("0x0000000000000000000000000000000000009102"),
	common.HexToAddress("0x0000000000000000000000000000000000009103"),
}

func rabbitVRFTestSettlementEVMV1(
	sdb *state.StateDB,
	timestamp uint64,
) *vm.EVM {
	evm := rabbitVRFTestEVM(sdb, timestamp)
	evm.Context.Coinbase = rabbitVRFTestSettlementProducerV1
	return evm
}

func rabbitVRFTestPendingSettlementRequestV1(
	t *testing.T,
) (
	*state.StateDB,
	common.Hash,
	uint64,
	*big.Int,
) {
	t.Helper()

	sdb := mkState(nil)
	misc.ApplyRabbitVRFCoordinatorV1(sdb)
	rabbitVRFTestPrepareRequestPricing(t, sdb)

	caller := common.HexToAddress(
		"0x1111111111111111111111111111111111111111",
	)
	sdb.SetBalance(
		caller,
		uint256.NewInt(1_000_000),
		tracing.BalanceChangeUnspecified,
	)

	const timestamp = uint64(2800)
	fee, _, err := rabbitVRFTestQuoteRequestFee(
		t,
		sdb,
		timestamp,
		0,
	)
	if err != nil {
		t.Fatalf("quoteRequestFee failed: %v", err)
	}
	if !fee.IsUint64() {
		t.Fatalf("request fee does not fit uint64: %s", fee)
	}

	ret, err := rabbitVRFTestRequestRandomness(
		sdb,
		timestamp,
		caller,
		0,
		crypto.Keccak256Hash(
			[]byte("rabbit-vrf-economic-settlement"),
		),
		fee.Uint64(),
	)
	if err != nil {
		t.Fatalf("requestRandomness failed: %v", err)
	}
	if len(ret) != 32 {
		t.Fatalf(
			"requestRandomness returned %d bytes, want 32",
			len(ret),
		)
	}

	requestID := common.BytesToHash(ret)
	requestBlock, _, _, _, _, status :=
		rabbitVRFTestRequestFinalizationStateV1(
			t,
			sdb,
			timestamp,
			requestID,
		)
	if requestBlock == 0 || status != 1 {
		t.Fatalf(
			"invalid pending request block=%d status=%d",
			requestBlock,
			status,
		)
	}

	return sdb, requestID, requestBlock, new(big.Int).Set(fee)
}

func rabbitVRFBalanceBigV1(
	sdb *state.StateDB,
	address common.Address,
) *big.Int {
	return new(big.Int).Set(
		sdb.GetBalance(address).ToBig(),
	)
}

func rabbitVRFBalanceDeltaV1(
	sdb *state.StateDB,
	address common.Address,
	before *big.Int,
) *big.Int {
	return new(big.Int).Sub(
		sdb.GetBalance(address).ToBig(),
		before,
	)
}

func TestRabbitVRFProtocolFeeSettlementConservationV1(t *testing.T) {
	if params.RabbitVRFAllocationV1Address != common.HexToAddress(
		"0x62B39E4f8e56a10Fc1CA65A7da1c46A02E9DB6e2",
	) {
		t.Fatalf(
			"Rabbit Allocation=%s",
			params.RabbitVRFAllocationV1Address,
		)
	}

	producer, participant, rabbit, err :=
		rabbitVRFSplitProtocolFeeV1(big.NewInt(2000), 3)
	if err != nil {
		t.Fatal(err)
	}
	if producer.Cmp(big.NewInt(600)) != 0 {
		t.Fatalf("producer=%s want=600", producer)
	}
	if participant.Cmp(big.NewInt(333)) != 0 {
		t.Fatalf("participant=%s want=333", participant)
	}
	if rabbit.Cmp(big.NewInt(401)) != 0 {
		t.Fatalf("rabbit=%s want=401", rabbit)
	}

	total := new(big.Int).Set(producer)
	total.Add(
		total,
		new(big.Int).Mul(participant, big.NewInt(3)),
	)
	total.Add(total, rabbit)
	if total.Cmp(big.NewInt(2000)) != 0 {
		t.Fatalf("settlement total=%s want=2000", total)
	}
}

func TestRabbitVRFProtocolFeeSettlementStateTransitionV1(
	t *testing.T,
) {
	sdb, requestID, requestBlock, fee :=
		rabbitVRFTestPendingSettlementRequestV1(t)

	producerBefore := rabbitVRFBalanceBigV1(
		sdb,
		rabbitVRFTestSettlementProducerV1,
	)
	rabbitBefore := rabbitVRFBalanceBigV1(
		sdb,
		params.RabbitVRFAllocationV1Address,
	)
	coordinatorBefore := rabbitVRFBalanceBigV1(
		sdb,
		params.RabbitVRFCoordinatorV1Address,
	)
	participantBefore := make(
		[]*big.Int,
		len(rabbitVRFTestSettlementParticipantsV1),
	)
	for index, participant := range rabbitVRFTestSettlementParticipantsV1 {
		participantBefore[index] =
			rabbitVRFBalanceBigV1(sdb, participant)
	}

	value := consensus.RabbitVRFValidatedFinalization{
		RequestID: requestID,
		Epoch:     7,
		Round:     requestBlock,
		Randomness: crypto.Keccak256Hash(
			[]byte("rabbit-vrf-economic-randomness"),
		),
		ProofHash: crypto.Keccak256Hash(
			[]byte("rabbit-vrf-economic-proof"),
		),
		Participants: append(
			[]common.Address(nil),
			rabbitVRFTestSettlementParticipantsV1...,
		),
	}

	if err := ProcessRabbitVRFFinalizations(
		rabbitVRFTestSettlementEVMV1(sdb, 1001),
		nil,
		[]consensus.RabbitVRFValidatedFinalization{value},
	); err != nil {
		t.Fatalf("settlement failed: %v", err)
	}

	producerWant, participantWant, rabbitWant, err :=
		rabbitVRFSplitProtocolFeeV1(
			fee,
			len(rabbitVRFTestSettlementParticipantsV1),
		)
	if err != nil {
		t.Fatal(err)
	}

	if got := rabbitVRFBalanceDeltaV1(
		sdb,
		rabbitVRFTestSettlementProducerV1,
		producerBefore,
	); got.Cmp(producerWant) != 0 {
		t.Fatalf(
			"producer delta=%s want=%s",
			got,
			producerWant,
		)
	}

	for index, participant := range rabbitVRFTestSettlementParticipantsV1 {
		got := rabbitVRFBalanceDeltaV1(
			sdb,
			participant,
			participantBefore[index],
		)
		if got.Cmp(participantWant) != 0 {
			t.Fatalf(
				"participant %d delta=%s want=%s",
				index,
				got,
				participantWant,
			)
		}
	}

	if got := rabbitVRFBalanceDeltaV1(
		sdb,
		params.RabbitVRFAllocationV1Address,
		rabbitBefore,
	); got.Cmp(rabbitWant) != 0 {
		t.Fatalf(
			"Rabbit Allocation delta=%s want=%s",
			got,
			rabbitWant,
		)
	}

	coordinatorAfter := rabbitVRFBalanceBigV1(
		sdb,
		params.RabbitVRFCoordinatorV1Address,
	)
	coordinatorSpent := new(big.Int).Sub(
		coordinatorBefore,
		coordinatorAfter,
	)
	if coordinatorSpent.Cmp(fee) != 0 {
		t.Fatalf(
			"coordinator spent=%s want fee=%s",
			coordinatorSpent,
			fee,
		)
	}

	_, _, _, _, _, status :=
		rabbitVRFTestRequestFinalizationStateV1(
			t,
			sdb,
			1001,
			requestID,
		)
	if status != 2 {
		t.Fatalf("status=%d want=2 COMPLETED", status)
	}

	// Replay must fail and cannot pay twice.
	producerAfterFirst := rabbitVRFBalanceBigV1(
		sdb,
		rabbitVRFTestSettlementProducerV1,
	)
	rabbitAfterFirst := rabbitVRFBalanceBigV1(
		sdb,
		params.RabbitVRFAllocationV1Address,
	)
	coordinatorAfterFirst := rabbitVRFBalanceBigV1(
		sdb,
		params.RabbitVRFCoordinatorV1Address,
	)
	participantAfterFirst := make(
		[]*big.Int,
		len(rabbitVRFTestSettlementParticipantsV1),
	)
	for index, participant := range rabbitVRFTestSettlementParticipantsV1 {
		participantAfterFirst[index] =
			rabbitVRFBalanceBigV1(sdb, participant)
	}

	if err := ProcessRabbitVRFFinalizations(
		rabbitVRFTestSettlementEVMV1(sdb, 1002),
		nil,
		[]consensus.RabbitVRFValidatedFinalization{value},
	); err == nil {
		t.Fatal("duplicate/replay settlement was accepted")
	}

	if rabbitVRFBalanceBigV1(
		sdb,
		rabbitVRFTestSettlementProducerV1,
	).Cmp(producerAfterFirst) != 0 {
		t.Fatal("replay changed producer balance")
	}
	if rabbitVRFBalanceBigV1(
		sdb,
		params.RabbitVRFAllocationV1Address,
	).Cmp(rabbitAfterFirst) != 0 {
		t.Fatal("replay changed Rabbit Allocation balance")
	}
	if rabbitVRFBalanceBigV1(
		sdb,
		params.RabbitVRFCoordinatorV1Address,
	).Cmp(coordinatorAfterFirst) != 0 {
		t.Fatal("replay changed coordinator balance")
	}
	for index, participant := range rabbitVRFTestSettlementParticipantsV1 {
		if rabbitVRFBalanceBigV1(
			sdb,
			participant,
		).Cmp(participantAfterFirst[index]) != 0 {
			t.Fatalf(
				"replay changed participant %d balance",
				index,
			)
		}
	}
}

func TestRabbitVRFProtocolFeeFailedFinalizationPaysZeroV1(
	t *testing.T,
) {
	sdb, requestID, requestBlock, _ :=
		rabbitVRFTestPendingSettlementRequestV1(t)

	addresses := append(
		[]common.Address{
			rabbitVRFTestSettlementProducerV1,
			params.RabbitVRFAllocationV1Address,
			params.RabbitVRFCoordinatorV1Address,
		},
		rabbitVRFTestSettlementParticipantsV1...,
	)
	before := make([]*big.Int, len(addresses))
	for index, address := range addresses {
		before[index] = rabbitVRFBalanceBigV1(
			sdb,
			address,
		)
	}

	err := ProcessRabbitVRFFinalizations(
		rabbitVRFTestSettlementEVMV1(sdb, 1001),
		nil,
		[]consensus.RabbitVRFValidatedFinalization{
			{
				RequestID: requestID,
				Epoch:     7,
				Round:     requestBlock + 1,
				Randomness: crypto.Keccak256Hash(
					[]byte("rabbit-vrf-bad-round-randomness"),
				),
				ProofHash: crypto.Keccak256Hash(
					[]byte("rabbit-vrf-bad-round-proof"),
				),
				Participants: append(
					[]common.Address(nil),
					rabbitVRFTestSettlementParticipantsV1...,
				),
			},
		},
	)
	if err == nil {
		t.Fatal("wrong-round finalization was accepted")
	}

	for index, address := range addresses {
		if rabbitVRFBalanceBigV1(
			sdb,
			address,
		).Cmp(before[index]) != 0 {
			t.Fatalf(
				"failed finalization changed balance %s",
				address,
			)
		}
	}

	_, epoch, round, randomness, proofHash, status :=
		rabbitVRFTestRequestFinalizationStateV1(
			t,
			sdb,
			1001,
			requestID,
		)
	if epoch != 0 ||
		round != 0 ||
		randomness != (common.Hash{}) ||
		proofHash != (common.Hash{}) ||
		status != 1 {
		t.Fatal("failed finalization mutated request")
	}
}

func TestRabbitVRFProtocolFeeSettlementRejectsDuplicateParticipantV1(
	t *testing.T,
) {
	value := consensus.RabbitVRFValidatedFinalization{
		RequestID:  common.HexToHash("0x1"),
		Epoch:      1,
		Round:      1,
		Randomness: common.HexToHash("0x2"),
		ProofHash:  common.HexToHash("0x3"),
		Participants: []common.Address{
			rabbitVRFTestSettlementParticipantsV1[0],
			rabbitVRFTestSettlementParticipantsV1[0],
		},
	}
	sdb := mkState(nil)
	if err := ProcessRabbitVRFFinalizations(
		rabbitVRFTestSettlementEVMV1(sdb, 1),
		nil,
		[]consensus.RabbitVRFValidatedFinalization{value},
	); err == nil {
		t.Fatal("duplicate participant was accepted")
	}
}

func TestRabbitVRFProtocolFeeSettlementTinyFeeV1(t *testing.T) {
	producer, participant, rabbit, err :=
		rabbitVRFSplitProtocolFeeV1(big.NewInt(1), 3)
	if err != nil {
		t.Fatal(err)
	}
	if producer.Sign() != 0 ||
		participant.Sign() != 0 ||
		rabbit.Cmp(big.NewInt(1)) != 0 {
		t.Fatalf(
			"tiny settlement producer=%s participant=%s rabbit=%s",
			producer,
			participant,
			rabbit,
		)
	}
}
