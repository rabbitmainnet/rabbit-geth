//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"github.com/ethereum/go-ethereum/common"
	"testing"
	"time"
)

// A pending canonical request must retry without requiring a new head event.
func TestRabbitVRFStationaryHeadReplayOrderV1(t *testing.T) {
	var queue rabbitVRFRequestRetryQueueV1
	id := common.HexToHash("0x1234")
	session := common.HexToHash("0xabcd")
	queue.trackV1(id, 501, common.HexToHash("0x501"))
	now := time.Unix(100, 0)
	calls := 0
	process := func(request common.Hash, origin rabbitVRFRequestRetryOriginV1) error {
		if request != id || origin.Block != 501 {
			t.Fatal("canonical origin lost")
		}
		calls++
		if calls == 2 {
			return errRabbitVRFRequestNotPendingV1
		}
		return nil
	}
	if err := queue.runV1(now, session, process); err != nil {
		t.Fatal(err)
	}
	if err := queue.runV1(now.Add(31*time.Second), session, process); err != nil {
		t.Fatal(err)
	}
	if calls != 2 || len(queue.Pending) != 0 {
		t.Fatal("stationary pending request was not retried and retired")
	}
}
