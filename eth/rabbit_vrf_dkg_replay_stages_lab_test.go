//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestRabbitVRFDKGReplayStagesFailOpenForIndependentArtifactsV1(t *testing.T) {
	broken := errors.New("local transport disconnected")
	called := make([]int, 0, 5)
	stages := make([]func() error, 5)
	for i := range stages {
		i := i
		stages[i] = func() error {
			called = append(called, i+1)
			if i == 0 {
				return broken
			}
			return nil
		}
	}
	err := rabbitVRFDKGReplayStagesV1(stages...)
	if !errors.Is(err, broken) {
		t.Fatalf("lost original send error: %v", err)
	}
	if fmt.Sprint(called) != "[1 2 3 4 5]" {
		t.Fatalf("failure starved other stages: %v", called)
	}
}

func TestRabbitVRFDKGReplayStagesMultipleErrorsRetainedV1(t *testing.T) {
	e1, e3 := errors.New("local artifact failed"), errors.New("polynomial failed")
	err := rabbitVRFDKGReplayStagesV1(
		func() error { return e1 },
		func() error { return nil },
		func() error { return e3 },
		func() error { return nil },
		func() error { return nil },
	)
	if !errors.Is(err, e1) || !errors.Is(err, e3) {
		t.Fatalf("missing stage error: %v", err)
	}
	if !strings.Contains(err.Error(), "stage 1") || !strings.Contains(err.Error(), "stage 3") {
		t.Fatal("stage context missing")
	}
}

func TestRabbitVRFDKGReplayStagesHealthyV1(t *testing.T) {
	called := 0
	err := rabbitVRFDKGReplayStagesV1(func() error { called++; return nil }, func() error { called++; return nil })
	if err != nil || called != 2 {
		t.Fatalf("healthy replay failed: calls=%d err=%v", called, err)
	}
}
