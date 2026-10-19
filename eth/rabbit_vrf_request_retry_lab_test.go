//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"errors"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

func TestRabbitVRFRequestRetryKeepsPendingContinuesAndChangesSession(t *testing.T) {
	a, b := common.HexToHash("0x01"), common.HexToHash("0x02")
	session := common.HexToHash("0x11")
	now := time.Unix(1000, 0)
	q := rabbitVRFRequestRetryQueueV1{}
	q.trackV1(a, 100, common.HexToHash("0xaa"))
	q.trackV1(b, 101, common.HexToHash("0xbb"))

	blocked := errors.New("temporary local signing failure")
	calls := []common.Hash{}
	process := func(id common.Hash, origin rabbitVRFRequestRetryOriginV1) error {
		calls = append(calls, id)
		if origin.Hash == (common.Hash{}) {
			t.Fatal("canonical origin lost")
		}
		if id == a {
			return blocked
		}
		return nil
	}
	if err := q.runV1(now, session, process); !errors.Is(err, blocked) {
		t.Fatalf("unexpected first error: %v", err)
	}
	if len(calls) != 2 || calls[0] != a || calls[1] != b {
		t.Fatal("first request failure blocked another request")
	}
	if len(q.Pending) != 2 {
		t.Fatal("successful local partial incorrectly removed pending request")
	}
	if err := q.runV1(now.Add(time.Second), session, process); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 {
		t.Fatal("retry interval ignored")
	}
	completed := func(id common.Hash, _ rabbitVRFRequestRetryOriginV1) error {
		calls = append(calls, id)
		if id == b {
			return errRabbitVRFRequestNotPendingV1
		}
		return nil
	}
	if err := q.runV1(now.Add(31*time.Second), session, completed); err != nil {
		t.Fatal(err)
	}
	if len(q.Pending) != 1 {
		t.Fatal("completed request was not removed")
	}
	before := len(calls)
	if err := q.runV1(now.Add(32*time.Second), common.HexToHash("0x12"), completed); err != nil {
		t.Fatal(err)
	}
	if len(calls) != before+1 {
		t.Fatal("session change did not immediately retry pending request")
	}
	q = rabbitVRFRequestRetryQueueV1{}
	if len(q.Pending) != 0 {
		t.Fatal("reset retained requests from obsolete canonical branch")
	}
}

func TestRabbitVRFNewRequestDoesNotAccelerateExistingRetries(t *testing.T) {
	now := time.Unix(1000, 0)
	session := common.HexToHash("0x11")
	a, b := common.HexToHash("0x01"), common.HexToHash("0x02")
	q := rabbitVRFRequestRetryQueueV1{}
	calls := make(map[common.Hash]int)
	process := func(id common.Hash, _ rabbitVRFRequestRetryOriginV1) error {
		calls[id]++
		return errors.New("temporary failure")
	}
	q.trackV1(a, 100, common.HexToHash("0xaa"))
	q.runV1(now, session, process)
	q.trackV1(b, 101, common.HexToHash("0xbb"))
	q.runV1(now.Add(time.Second), session, process)
	if calls[a] != 1 || calls[b] != 1 {
		t.Fatalf("new request accelerated old retry: %v", calls)
	}
	q.runV1(now.Add(30*time.Second), session, process)
	if calls[a] != 2 || calls[b] != 1 {
		t.Fatalf("individual retry deadlines ignored: %v", calls)
	}
	q.runV1(now.Add(31*time.Second), session, process)
	if calls[a] != 2 || calls[b] != 2 {
		t.Fatalf("second request did not retry at its deadline: %v", calls)
	}
}
