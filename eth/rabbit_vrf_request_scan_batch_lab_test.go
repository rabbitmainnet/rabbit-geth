//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import "testing"

func TestRabbitVRFRequestScanEndV1(t *testing.T) {
	cases := []struct {
		name             string
		next, head, want uint64
	}{
		{"single", 10, 10, 10},
		{"small", 10, 100, 100},
		{"batch", 10, 10000, 10 + rabbitVRFRequestScanBatchBlocksV1 - 1},
		{"next batch", 10 + rabbitVRFRequestScanBatchBlocksV1, 10000, 10 + 2*rabbitVRFRequestScanBatchBlocksV1 - 1},
		{"ahead", 101, 100, 100},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := rabbitVRFRequestScanEndV1(tc.next, tc.head); got != tc.want {
				t.Fatalf("end=%d want=%d", got, tc.want)
			}
		})
	}
}
