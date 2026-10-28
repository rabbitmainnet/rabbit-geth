//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"errors"
	"fmt"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestRabbitVRFThresholdMissingRequestKeepsPeerContext(t *testing.T) {
	stateErr := errors.New("canonical state unavailable")
	cases := []struct {
		name  string
		input error
		want  error
	}{
		{"missing", errRabbitVRFRequestMissingV1, errRabbitVRFThresholdContextNotReadyV1},
		{"wrapped-missing", fmt.Errorf("lookup: %w", errRabbitVRFRequestMissingV1), errRabbitVRFThresholdContextNotReadyV1},
		{"not-pending", errRabbitVRFRequestNotPendingV1, errRabbitVRFRequestNotPendingV1},
		{"state-error", stateErr, stateErr},
		{"pending", nil, nil},
	}
	id := common.HexToHash("0x1234")
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			runtime := &rabbitVRFDKGRuntime{
				canonicalRequestLookup: func(got common.Hash) (rabbitVRFCanonicalRequestV1, error) {
					calls++
					if got != id {
						t.Fatal("request identity changed")
					}
					if tc.input != nil {
						return rabbitVRFCanonicalRequestV1{}, tc.input
					}
					return rabbitVRFCanonicalRequestV1{
						RequestID: id, Status: rabbitVRFRequestStatusPendingV1,
					}, nil
				},
			}
			transport := &rabbitVRFDKGTransport{runtime: runtime}
			request, err := transport.thresholdPartialRequestReadyV1(id)
			if calls != 1 {
				t.Fatalf("lookup calls: %d", calls)
			}
			if tc.want == nil {
				if err != nil || request.RequestID != id {
					t.Fatalf("valid pending request changed: %+v %v", request, err)
				}
			} else {
				if !errors.Is(err, tc.want) {
					t.Fatalf("got %v, want %v", err, tc.want)
				}
				if request.RequestID != (common.Hash{}) {
					t.Fatal("failed lookup returned an accepted request")
				}
			}
		})
	}
}
