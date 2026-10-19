//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"errors"
	"sort"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

type rabbitVRFRequestRetryOriginV1 struct {
	StartedSession common.Hash
	NextRetry      time.Time
	Block          uint64
	Hash           common.Hash
}

type rabbitVRFRequestRetryQueueV1 struct {
	Pending map[common.Hash]rabbitVRFRequestRetryOriginV1
	Session common.Hash
}

func (q *rabbitVRFRequestRetryQueueV1) trackV1(
	request common.Hash, block uint64, hash common.Hash,
) {
	if q.Pending == nil {
		q.Pending = make(map[common.Hash]rabbitVRFRequestRetryOriginV1)
	}
	if _, exists := q.Pending[request]; !exists {
		q.Pending[request] = rabbitVRFRequestRetryOriginV1{Block: block, Hash: hash}
	}
}

func (q *rabbitVRFRequestRetryQueueV1) runV1(
	now time.Time,
	session common.Hash,
	process func(common.Hash, rabbitVRFRequestRetryOriginV1) error,
) error {
	if len(q.Pending) == 0 || session == (common.Hash{}) {
		return nil
	}
	changedSession := q.Session != session
	q.Session = session
	ids := make([]common.Hash, 0, len(q.Pending))
	for id := range q.Pending {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		a, b := q.Pending[ids[i]], q.Pending[ids[j]]
		if a.Block != b.Block {
			return a.Block < b.Block
		}
		return ids[i].Hex() < ids[j].Hex()
	})
	var firstErr error
	for _, id := range ids {
		origin := q.Pending[id]
		if !changedSession && now.Before(origin.NextRetry) {
			continue
		}
		origin.NextRetry = now.Add(30 * time.Second)
		q.Pending[id] = origin
		err := process(id, origin)
		if errors.Is(err, errRabbitVRFRequestWorkerReorgV1) {
			return err
		}
		if err == nil {
			origin := q.Pending[id]
			origin.StartedSession = session
			q.Pending[id] = origin
		}

		if errors.Is(err, errRabbitVRFRequestNotPendingV1) {
			delete(q.Pending, id)
		} else if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
