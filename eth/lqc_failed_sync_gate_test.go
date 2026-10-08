package eth

import "testing"

func TestLQCFailedSyncLiveRecovery(t *testing.T) {
	var gate lqcFailedSyncGate
	gate.started()
	if gate.recover(101, true, true, true, false) {
		t.Fatal("active download must stay blocked")
	}
	gate.failed(100)
	for _, tc := range []struct {
		name                            string
		number                          uint64
		live, state, confirmed, pending bool
	}{
		{"same head", 100, true, true, true, false},
		{"rewound head", 99, true, true, true, false},
		{"stale head", 101, false, true, true, false},
		{"missing state", 101, true, false, true, false},
		{"isolated or unconfirmed", 101, true, true, false, false},
		{"fork recovery pending", 101, true, true, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if gate.recover(tc.number, tc.live, tc.state, tc.confirmed, tc.pending) || !gate.required {
				t.Fatal("unsafe recovery released production gate")
			}
		})
	}
	if !gate.recover(101, true, true, true, false) || gate.required {
		t.Fatal("confirmed executed live advance must release failed sync gate")
	}
	gate.started()
	if gate.recover(102, true, true, true, false) {
		t.Fatal("another active download must block again")
	}
	gate.failed(102)
	if gate.recover(101, true, true, true, false) {
		t.Fatal("a subsequent failure must use its own head boundary")
	}
	gate.completed()
	if gate.required || gate.failedSync {
		t.Fatal("successful sync must clear gate")
	}
}
