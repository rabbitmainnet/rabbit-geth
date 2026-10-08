package eth

// lqcFailedSyncGate distinguishes an in-progress download from a failed one.
// A failed download can recover through validated live block import, but only
// after the executed canonical head advances and all production checks agree.
type lqcFailedSyncGate struct {
	required     bool
	failedSync   bool
	failedNumber uint64
}

func (g *lqcFailedSyncGate) started() {
	*g = lqcFailedSyncGate{required: true}
}

func (g *lqcFailedSyncGate) failed(number uint64) {
	*g = lqcFailedSyncGate{required: true, failedSync: true, failedNumber: number}
}

func (g *lqcFailedSyncGate) completed() {
	*g = lqcFailedSyncGate{}
}

func (g *lqcFailedSyncGate) recover(number uint64, live, stateAvailable, peerConfirmed, recoveryPending bool) bool {
	if !g.required || !g.failedSync || number <= g.failedNumber ||
		!live || !stateAvailable || !peerConfirmed || recoveryPending {
		return false
	}
	g.completed()
	return true
}
