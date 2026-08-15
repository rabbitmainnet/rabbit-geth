package lqc

import "testing"

func TestMinAllowedTimeProducer(t *testing.T) {
	l := &LQC{}
	got := l.minAllowedTime(100, 0)
	if got != 115 {
		t.Fatalf("expected 115, got %d", got)
	}
}

func TestMinAllowedTimeFallbacks(t *testing.T) {
	l := &LQC{}
	if got := l.minAllowedTime(100, 1); got != 120 {
		t.Fatalf("expected fallback1 min time 120, got %d", got)
	}
	if got := l.minAllowedTime(100, 2); got != 125 {
		t.Fatalf("expected fallback2 min time 125, got %d", got)
	}
}
