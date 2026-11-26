//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"errors"
	"strings"
	"testing"
)

func TestRabbitVRFDKGReplayRoundRouteFailureDoesNotStarveArtifactsV1(t *testing.T) {
	routeErr := errors.New("route connection closed")
	var routes, artifacts, relays int
	err := rabbitVRFDKGReplayRoundV1(
		func() error { routes++; return routeErr },
		func() error { artifacts++; return nil },
		func() error { relays++; return nil },
	)
	if routes != 1 || artifacts != 1 || relays != 0 {
		t.Fatalf("route=%d artifacts=%d relay=%d", routes, artifacts, relays)
	}
	if !errors.Is(err, routeErr) || !strings.Contains(err.Error(), "peer-route") {
		t.Fatalf("missing route error attribution: %v", err)
	}
}

func TestRabbitVRFDKGReplayRoundErrorIsolationV1(t *testing.T) {
	routeErr, artifactErr := errors.New("route failed"), errors.New("artifact failed")
	called := false
	err := rabbitVRFDKGReplayRoundV1(
		func() error { return routeErr },
		func() error { return artifactErr },
		func() error { called = true; return nil },
	)
	if called || !errors.Is(err, routeErr) || !errors.Is(err, artifactErr) {
		t.Fatalf("relay invoked=%v errors=%v", called, err)
	}
}

func TestRabbitVRFDKGReplayRoundHealthyPathV1(t *testing.T) {
	var order string
	err := rabbitVRFDKGReplayRoundV1(
		func() error { order += "R"; return nil },
		func() error { order += "A"; return nil },
		func() error { order += "L"; return nil },
	)
	if err != nil || order != "RAL" {
		t.Fatalf("order=%s err=%v", order, err)
	}
}
