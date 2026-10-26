//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"errors"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

// The request lane remains the sole owner of queue/cursor mutations. Workers
// receive immutable origins and process different request IDs only. Every
// callback retains the normal canonical-origin, session and secret-ready checks.
func (q *rabbitVRFRequestRetryQueueV1) runConcurrentV1(now time.Time, session common.Hash, process func(common.Hash, rabbitVRFRequestRetryOriginV1) error) error {
	if len(q.Pending) == 0 || session == (common.Hash{}) {
		return nil
	}
	changed := q.Session != session
	q.Session = session
	type job struct {
		id     common.Hash
		origin rabbitVRFRequestRetryOriginV1
	}
	jobs := make([]job, 0, len(q.Pending))
	for id, origin := range q.Pending {
		if !changed && now.Before(origin.NextRetry) {
			continue
		}
		origin.NextRetry = now.Add(30 * time.Second)
		jobs = append(jobs, job{id, origin})
	}
	sort.Slice(jobs, func(i, j int) bool {
		if jobs[i].origin.Block != jobs[j].origin.Block {
			return jobs[i].origin.Block < jobs[j].origin.Block
		}
		return jobs[i].id.Hex() < jobs[j].id.Hex()
	})
	if len(jobs) == 0 {
		return nil
	}
	results := make([]error, len(jobs))
	indices := make(chan int)
	var workers sync.WaitGroup
	var reorg atomic.Bool
	limit := 3
	if len(jobs) < limit {
		limit = len(jobs)
	}
	for worker := 0; worker < limit; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for i := range indices {
				if reorg.Load() {
					results[i] = errRabbitVRFRequestWorkerReorgV1
					continue
				}
				results[i] = process(jobs[i].id, jobs[i].origin)
				if errors.Is(results[i], errRabbitVRFRequestWorkerReorgV1) {
					reorg.Store(true)
				}
			}
		}()
	}
	for i := range jobs {
		indices <- i
	}
	close(indices)
	workers.Wait()
	// Never commit partial queue progress when any canonical-origin check failed.
	if reorg.Load() {
		return errRabbitVRFRequestWorkerReorgV1
	}
	var firstErr error
	for i, job := range jobs {
		origin := job.origin
		if results[i] == nil {
			origin.StartedSession = session
		}
		if errors.Is(results[i], errRabbitVRFRequestNotPendingV1) {
			delete(q.Pending, job.id)
		} else {
			q.Pending[job.id] = origin
			if results[i] != nil && firstErr == nil {
				firstErr = results[i]
			}
		}
	}
	return firstErr
}
