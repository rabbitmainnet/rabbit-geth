package core

import (
	"bytes"
	"context"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
	"math/big"
	"testing"
)

func TestRabbitVRFEconomicsV2PendingAcrossFork(
	t *testing.T,
) {
	sdb, requestID, requestBlock, fee :=
		rabbitVRFTestPendingSettlementRequestV1(t)

	oldQuote, err := rabbitVRFTestQuoteProtocolFee(t, sdb, 2800)
	if err != nil {
		t.Fatal(err)
	}

	config := rabbitVRFTestConfig(4)
	config.LQC.VRFEconomicsV2Block = 312000
	seed := rabbitVRFTestSettlementEVMV1(sdb, 2800)
	seed.Context.BlockNumber = big.NewInt(312000)
	upgradedEVM := vm.NewEVM(seed.Context, sdb, config, vm.Config{NoBaseFee: true})
	defer upgradedEVM.Release()
	PreExecution(
		context.Background(), nil,
		&types.Header{Number: big.NewInt(311999), Time: 2790},
		config, upgradedEVM, big.NewInt(312000), 2800,
	)
	if !bytes.Equal(sdb.GetCode(params.RabbitVRFCoordinatorV1Address),
		params.RabbitVRFCoordinatorV2Code) {
		t.Fatal("V2 not installed at fork")
	}
	newQuote, err := rabbitVRFTestQuoteProtocolFee(t, sdb, 2800)
	if err != nil {
		t.Fatal(err)
	}
	wantQuote := new(big.Int).Mul(oldQuote, big.NewInt(5))
	if newQuote.Cmp(wantQuote) != 0 {
		t.Fatalf("TWAP quote=%s want=%s", newQuote, wantQuote)
	}
	savedBlock, _, _, _, _, pending :=
		rabbitVRFTestRequestFinalizationStateV1(t, sdb, 2800, requestID)
	if savedBlock != requestBlock || pending != 1 {
		t.Fatal("pending request lost during upgrade")
	}

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
		upgradedEVM,
		nil,
		[]consensus.RabbitVRFValidatedFinalization{value},
	); err != nil {
		t.Fatalf("settlement failed: %v", err)
	}

	producerWant, participantWant, rabbitWant, err :=
		rabbitVRFSplitProtocolFeeV2(
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
		upgradedEVM,
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
