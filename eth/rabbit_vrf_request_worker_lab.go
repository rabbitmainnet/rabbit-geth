//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
)

// Limit each request-scan pass so a historical backlog cannot monopolize
// the DKG runtime event loop. Progress is retained in the canonical cursor.
const rabbitVRFRequestScanBatchBlocksV1 uint64 = 512

// rabbitVRFRequestScanEndV1 bounds the inclusive block range. It never
// advances the cursor or skips blocks; the next pass resumes at cursor.Next.
func rabbitVRFRequestScanEndV1(next, head uint64) uint64 {
	if next > head {
		return head
	}
	if head-next >= rabbitVRFRequestScanBatchBlocksV1 {
		return next + rabbitVRFRequestScanBatchBlocksV1 - 1
	}
	return head
}

var rabbitVRFRandomnessRequestedTopicV1 = crypto.Keccak256Hash(
	[]byte("RandomnessRequested(bytes32,address,uint64,uint32,bytes32,uint256)"),
)

var errRabbitVRFRequestWorkerReorgV1 = errors.New(
	"rabbit vrf request worker canonical reorg",
)

type rabbitVRFRequestScanCursorV1 struct {
	Pending  []common.Hash
	Retry    rabbitVRFRequestRetryQueueV1
	Next     uint64
	LastHash common.Hash
}

func (cursor *rabbitVRFRequestScanCursorV1) resetV1(activation uint64) {
	if cursor == nil {
		return
	}
	cursor.Next = activation
	cursor.LastHash = common.Hash{}
	cursor.Pending = nil
	cursor.Retry = rabbitVRFRequestRetryQueueV1{}
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
	// Discover canonical requests even while the active signing epoch is not
	// ready. Public discovery must not stall behind DKG preparation.
	context := runtime.thresholdContextV1()
	transport := runtime.backend.vrfDKGTransport

	// Restore persisted IDs into the canonical-origin retry queue. Finalized
	// requests are removed; other failures remain pending for a later cycle.
	for _, id := range cursor.Pending {
		if _, exists := cursor.Retry.Pending[id]; exists {
			continue
		}
		request, err := runtime.canonicalPendingRequestV1(id)
		if errors.Is(err, errRabbitVRFRequestNotPendingV1) {
			continue
		}
		if err != nil {
			return err
		}
		origin := runtime.backend.blockchain.GetBlockByNumber(request.RequestBlock)
		if origin == nil {
			return errRabbitVRFRequestWorkerReorgV1
		}
		cursor.Retry.trackV1(id, request.RequestBlock, origin.Hash())
	}
	defer func() {
		cursor.Pending = cursor.Pending[:0]
		for id := range cursor.Retry.Pending {
			cursor.Pending = append(cursor.Pending, id)
		}
		sort.Slice(cursor.Pending, func(i, j int) bool { return bytes.Compare(cursor.Pending[i][:], cursor.Pending[j][:]) < 0 })
	}()

	headNumber = rabbitVRFRequestScanEndV1(cursor.Next, headNumber)
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

		canonical := runtime.backend.blockchain.GetBlockByNumber(blockNumber)
		if canonical == nil || canonical.Hash() != block.Hash() {
			cursor.resetV1(activation)
			return fmt.Errorf(
				"%w while scanning block %d",
				errRabbitVRFRequestWorkerReorgV1,
				blockNumber,
			)
		}

		for _, requestID := range rabbitVRFRequestIDsFromReceiptsV1(receipts) {
			cursor.Retry.trackV1(requestID, blockNumber, block.Hash())
		}
		previousHash = block.Hash()
		cursor.Next = blockNumber + 1
		cursor.LastHash = block.Hash()
	}

	if context.SessionID == (common.Hash{}) || len(context.Members) == 0 || !runtime.secretReadyV1() {
		// Retire completed requests without signing or fabricating an active
		// context. Pending requests retain their canonical origin for retry.
		for id, origin := range cursor.Retry.Pending {
			canonical := runtime.backend.blockchain.GetBlockByNumber(origin.Block)
			if canonical == nil || canonical.Hash() != origin.Hash {
				cursor.resetV1(activation)
				return errRabbitVRFRequestWorkerReorgV1
			}
			if _, err := runtime.canonicalPendingRequestV1(id); errors.Is(err, errRabbitVRFRequestNotPendingV1) {
				delete(cursor.Retry.Pending, id)
			}
		}
		return nil
	}
	if transport == nil {
		return errors.New("rabbit vrf request worker transport unavailable")
	}

	err := cursor.Retry.runConcurrentV1(time.Now(), context.SessionID,
		func(requestID common.Hash, origin rabbitVRFRequestRetryOriginV1) error {
			current := runtime.thresholdContextV1()
			if current.SessionID != context.SessionID || !runtime.secretReadyV1() {
				return errRabbitVRFDKGArtifactSessionMismatch
			}
			canonical := runtime.backend.blockchain.GetBlockByNumber(origin.Block)
			if canonical == nil || canonical.Hash() != origin.Hash {
				return errRabbitVRFRequestWorkerReorgV1
			}
			err := transport.processCanonicalPendingRequestV1(requestID)
			canonical = runtime.backend.blockchain.GetBlockByNumber(origin.Block)
			if canonical == nil || canonical.Hash() != origin.Hash {
				return errRabbitVRFRequestWorkerReorgV1
			}
			if err != nil {
				return fmt.Errorf(
					"process canonical rabbit vrf request %s from block %d: %w",
					requestID, origin.Block, err,
				)
			}
			if origin.StartedSession != context.SessionID {
				log.Info(
					"Rabbit VRF canonical request threshold flow started",
					"request", requestID, "block", origin.Block,
				)
			}
			return nil
		},
	)
	if errors.Is(err, errRabbitVRFRequestWorkerReorgV1) {
		cursor.resetV1(activation)
	}
	return err
}

// rabbitVRFReplayPendingV1 retains requests whose threshold flow has
// started but whose canonical completion has not been confirmed.
func rabbitVRFReplayPendingV1(
	pending []common.Hash,
	process func(common.Hash) error,
) ([]common.Hash, error) {
	remaining := make([]common.Hash, 0, len(pending))
	for _, id := range pending {
		err := process(id)
		if errors.Is(err, errRabbitVRFRequestNotPendingV1) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("recover rabbit vrf request %s: %w", id, err)
		}
		remaining = append(remaining, id)
	}
	return remaining, nil
}
