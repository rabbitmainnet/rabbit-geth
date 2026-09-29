//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
)

var rabbitVRFRandomnessRequestedTopicV1 = crypto.Keccak256Hash(
	[]byte("RandomnessRequested(bytes32,address,uint64,uint32,bytes32,uint256)"),
)

var errRabbitVRFRequestWorkerReorgV1 = errors.New(
	"rabbit vrf request worker canonical reorg",
)

type rabbitVRFRequestScanCursorV1 struct {
	Next     uint64
	LastHash common.Hash
}

func (cursor *rabbitVRFRequestScanCursorV1) resetV1(activation uint64) {
	if cursor == nil {
		return
	}
	cursor.Next = activation
	cursor.LastHash = common.Hash{}
}

func (cursor *rabbitVRFRequestScanCursorV1) reconcilePreviousV1(
	activation uint64,
	canonicalPreviousHash common.Hash,
) bool {
	if cursor == nil ||
		cursor.Next <= activation ||
		cursor.LastHash == (common.Hash{}) {
		return false
	}
	if canonicalPreviousHash == cursor.LastHash {
		return false
	}
	cursor.resetV1(activation)
	return true
}

func rabbitVRFRequestIDsFromReceiptsV1(receipts types.Receipts) []common.Hash {
	if len(receipts) == 0 {
		return nil
	}

	seen := make(map[common.Hash]struct{})
	out := make([]common.Hash, 0)

	for _, receipt := range receipts {
		if receipt == nil {
			continue
		}

		for _, event := range receipt.Logs {
			if event == nil ||
				event.Address != params.RabbitVRFCoordinatorV1Address ||
				len(event.Topics) != 4 ||
				event.Topics[0] != rabbitVRFRandomnessRequestedTopicV1 {
				continue
			}

			requestID := event.Topics[1]
			if requestID == (common.Hash{}) {
				continue
			}
			if _, exists := seen[requestID]; exists {
				continue
			}

			seen[requestID] = struct{}{}
			out = append(out, requestID)
		}
	}

	return out
}

func (runtime *rabbitVRFDKGRuntime) rabbitVRFRequestScanStartV1() uint64 {
	if runtime == nil ||
		runtime.backend == nil ||
		runtime.backend.blockchain == nil {
		return 0
	}

	config := runtime.backend.blockchain.Config()
	if config == nil || config.LQC == nil {
		return 0
	}

	return config.LQC.VRFProtocolBlock
}

func (runtime *rabbitVRFDKGRuntime) processCanonicalPendingRequestsV1(
	cursor *rabbitVRFRequestScanCursorV1,
) error {
	if runtime == nil ||
		cursor == nil ||
		runtime.backend == nil ||
		runtime.backend.blockchain == nil {
		return errRabbitVRFDKGRuntimeV1
	}

	activation := runtime.rabbitVRFRequestScanStartV1()
	if activation == 0 {
		return nil
	}
	if cursor.Next == 0 || cursor.Next < activation {
		cursor.resetV1(activation)
	}

	if cursor.Next > activation && cursor.LastHash != (common.Hash{}) {
		previous := runtime.backend.blockchain.GetBlockByNumber(cursor.Next - 1)
		previousHash := common.Hash{}
		if previous != nil {
			previousHash = previous.Hash()
		}
		cursor.reconcilePreviousV1(activation, previousHash)
	}

	head := runtime.backend.blockchain.CurrentBlock()
	if head == nil ||
		head.Number == nil ||
		!head.Number.IsUint64() {
		return nil
	}

	headNumber := head.Number.Uint64()
	if headNumber < cursor.Next {
		return nil
	}

	if !runtime.secretReadyV1() {
		return nil
	}

	context := runtime.currentContext()
	if context.SessionID == (common.Hash{}) ||
		len(context.Members) == 0 {
		return nil
	}

	transport := runtime.backend.vrfDKGTransport
	if transport == nil {
		return errors.New(
			"rabbit vrf request worker transport unavailable",
		)
	}

	previousHash := cursor.LastHash

	for blockNumber := cursor.Next; blockNumber <= headNumber; blockNumber++ {
		block := runtime.backend.blockchain.GetBlockByNumber(blockNumber)
		if block == nil {
			return fmt.Errorf(
				"rabbit vrf request worker block %d unavailable",
				blockNumber,
			)
		}

		if previousHash != (common.Hash{}) &&
			block.ParentHash() != previousHash {
			cursor.resetV1(activation)
			return fmt.Errorf(
				"%w at block %d",
				errRabbitVRFRequestWorkerReorgV1,
				blockNumber,
			)
		}

		receipts :=
			runtime.backend.blockchain.GetReceiptsByHash(block.Hash())

		if len(block.Transactions()) > 0 && receipts == nil {
			return fmt.Errorf(
				"rabbit vrf request worker receipts %d unavailable",
				blockNumber,
			)
		}

		for _, requestID := range rabbitVRFRequestIDsFromReceiptsV1(receipts) {

			err :=
				transport.processCanonicalPendingRequestV1(requestID)

			if err != nil {
				if errors.Is(
					err,
					errRabbitVRFRequestNotPendingV1,
				) {
					continue
				}

				return fmt.Errorf(
					"process canonical rabbit vrf request %s from block %d: %w",
					requestID,
					blockNumber,
					err,
				)
			}

			log.Info(
				"Rabbit VRF canonical request threshold flow started",
				"request", requestID,
				"block", blockNumber,
			)
		}

		canonical := runtime.backend.blockchain.GetBlockByNumber(blockNumber)
		if canonical == nil || canonical.Hash() != block.Hash() {
			cursor.resetV1(activation)
			return fmt.Errorf(
				"%w while scanning block %d",
				errRabbitVRFRequestWorkerReorgV1,
				blockNumber,
			)
		}

		previousHash = block.Hash()
		cursor.Next = blockNumber + 1
		cursor.LastHash = block.Hash()
	}

	return nil
}
