package miner

import (
	"context"
	"errors"
	"testing"
)

// A cancelled producer must not start transaction selection for an obsolete head.
func TestLQCCommitTransactionsCancelledBeforeSelection(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	m := &Miner{}
	err := m.commitTransactions(ctx, nil, nil, nil, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled work returned %v, want context.Canceled", err)
	}
}
