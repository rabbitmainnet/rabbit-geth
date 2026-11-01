package eth

import (
	"testing"
	"time"
)

func TestLQCStartupRetryDelayV2419(t *testing.T) {
	cases := []struct {
		attempt int
		want    time.Duration
	}{
		{1, time.Second},
		{2, 2 * time.Second},
		{3, 4 * time.Second},
		{4, 8 * time.Second},
		{10, 8 * time.Second},
		{1000, 8 * time.Second},
	}

	for _, tc := range cases {
		got := lqcStartupRetryDelayV2419(tc.attempt)
		if got != tc.want {
			t.Fatalf("attempt=%d got=%v want=%v",
				tc.attempt, got, tc.want)
		}
	}
}
