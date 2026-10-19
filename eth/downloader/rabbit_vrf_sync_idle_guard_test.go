package downloader

import (
	"testing"
	"time"
)

func TestSyncIdleGuardSerializesSynchronisation(t *testing.T) {
	var d Downloader

	if !d.TryAcquireSyncIdleGuard() {
		t.Fatal("initial idle guard was rejected")
	}

	syncStarted := make(chan struct{})
	syncAcquired := make(chan struct{})
	releaseSync := make(chan struct{})
	syncDone := make(chan struct{})

	go func() {
		if !d.synchronising.CompareAndSwap(false, true) {
			close(syncDone)
			return
		}
		close(syncStarted)

		d.syncIdleGate.Lock()
		close(syncAcquired)

		<-releaseSync

		d.synchronising.Store(false)
		d.syncIdleGate.Unlock()
		close(syncDone)
	}()

	<-syncStarted

	if d.TryAcquireSyncIdleGuard() {
		d.ReleaseSyncIdleGuard()
		t.Fatal("new idle guard entered after synchronisation started")
	}

	select {
	case <-syncAcquired:
		t.Fatal("synchronisation acquired writer gate while idle guard was held")
	default:
	}

	d.ReleaseSyncIdleGuard()

	select {
	case <-syncAcquired:
	case <-time.After(time.Second):
		t.Fatal("synchronisation did not acquire writer gate after idle guard release")
	}

	if d.TryAcquireSyncIdleGuard() {
		d.ReleaseSyncIdleGuard()
		t.Fatal("idle guard entered while synchronisation held writer gate")
	}

	close(releaseSync)

	select {
	case <-syncDone:
	case <-time.After(time.Second):
		t.Fatal("synchronisation writer gate did not finish")
	}

	if !d.TryAcquireSyncIdleGuard() {
		t.Fatal("idle guard did not reopen after synchronisation")
	}
	d.ReleaseSyncIdleGuard()
}
