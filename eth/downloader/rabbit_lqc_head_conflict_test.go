package downloader

import (
	"errors"
	"fmt"
	"testing"
)

func TestIsLQCBeaconHeadConflict(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{"reorg", errChainReorged, true},
		{"gap", errChainGapped, true},
		{"fork", errChainForked, true},
		{"wrapped fork", fmt.Errorf("test: %w", errChainForked), true},
		{"other error", errors.New("database unavailable"), false},
		{"sync restarted", errSyncReorged, false},
		{"nil", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsLQCBeaconHeadConflict(tc.err); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}
