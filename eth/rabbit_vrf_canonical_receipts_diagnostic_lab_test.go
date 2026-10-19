//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/ethereum/go-ethereum/core/types"
)

func TestRabbitVRFCanonicalReceiptsDiagnostic(t *testing.T) {
	data, err := os.ReadFile("testdata/rabbit_vrf_canonical_receipts_diagnostic.json")
	if err != nil {
		t.Fatal(err)
	}
	var receipts types.Receipts
	if err := json.Unmarshal(data, &receipts); err != nil {
		t.Fatal(err)
	}
	ids := rabbitVRFRequestIDsFromReceiptsV1(receipts)
	if len(ids) == 0 {
		t.Fatal("worker extractor recognized no canonical request events")
	}
	t.Logf("REQUEST EVENTS RECOGNIZED BY WORKER: %d", len(ids))
	for _, id := range ids {
		t.Logf("request=%s", id)
	}
}
