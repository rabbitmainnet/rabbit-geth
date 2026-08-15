package eth

import (
	"errors"
	"sync"
)

var (
	errLQCWorkV1FairVerifierClosed = errors.New("lqc work v1 fair verifier closed")
	errLQCWorkV1FairPeerPending    = errors.New("lqc work v1 peer already has pending expensive verification")
	errLQCWorkV1FairInvalidTask    = errors.New("invalid lqc work v1 fair verification task")
)

type lqcWorkV1FairTask struct {
	peerID string
	work   func() error
	result chan error
}

// One connected peer may have at most one queued-or-running expensive
// verification. Queue size is therefore bounded by the node's P2P peer count.
// This is a local non-consensus DoS/liveness guard, not peer-identity Sybil
// resistance.
type lqcWorkV1FairVerifier struct {
	mu      sync.Mutex
	cond    *sync.Cond
	queue   []*lqcWorkV1FairTask
	pending map[string]struct{}
	closed  bool
}

func newLQCWorkV1FairVerifier() *lqcWorkV1FairVerifier {
	v := &lqcWorkV1FairVerifier{
		pending: make(map[string]struct{}),
	}
	v.cond = sync.NewCond(&v.mu)
	go v.loop()
	return v
}

func (v *lqcWorkV1FairVerifier) Run(peerID string, work func() error) error {
	if v == nil || peerID == "" || work == nil {
		return errLQCWorkV1FairInvalidTask
	}
	task := &lqcWorkV1FairTask{
		peerID: peerID,
		work:   work,
		result: make(chan error, 1),
	}

	v.mu.Lock()
	if v.closed {
		v.mu.Unlock()
		return errLQCWorkV1FairVerifierClosed
	}
	if _, exists := v.pending[peerID]; exists {
		v.mu.Unlock()
		return errLQCWorkV1FairPeerPending
	}
	v.pending[peerID] = struct{}{}
	v.queue = append(v.queue, task)
	v.cond.Signal()
	v.mu.Unlock()

	return <-task.result
}

func (v *lqcWorkV1FairVerifier) loop() {
	for {
		v.mu.Lock()
		for len(v.queue) == 0 && !v.closed {
			v.cond.Wait()
		}
		if v.closed && len(v.queue) == 0 {
			v.mu.Unlock()
			return
		}

		task := v.queue[0]
		v.queue = v.queue[1:]
		v.mu.Unlock()

		err := task.work()

		v.mu.Lock()
		delete(v.pending, task.peerID)
		v.mu.Unlock()

		task.result <- err
		close(task.result)
	}
}

func (v *lqcWorkV1FairVerifier) Close() {
	if v == nil {
		return
	}

	v.mu.Lock()
	if v.closed {
		v.mu.Unlock()
		return
	}
	v.closed = true
	queued := append([]*lqcWorkV1FairTask(nil), v.queue...)
	v.queue = nil
	for _, task := range queued {
		delete(v.pending, task.peerID)
	}
	v.cond.Broadcast()
	v.mu.Unlock()

	for _, task := range queued {
		task.result <- errLQCWorkV1FairVerifierClosed
		close(task.result)
	}
}

func (v *lqcWorkV1FairVerifier) pendingCount() int {
	if v == nil {
		return 0
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	return len(v.pending)
}
