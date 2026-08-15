package eth

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

func waitFairPendingV1(t *testing.T, v *lqcWorkV1FairVerifier, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if v.pendingCount() >= want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("pending=%d want-at-least=%d", v.pendingCount(), want)
}

func TestLQCWorkV1FairVerifierOneOutstandingPerPeer(t *testing.T) {
	v := newLQCWorkV1FairVerifier()
	defer v.Close()

	release := make(chan struct{})
	firstDone := make(chan error, 1)

	go func() {
		firstDone <- v.Run("peer-a", func() error {
			<-release
			return nil
		})
	}()
	waitFairPendingV1(t, v, 1)

	err := v.Run("peer-a", func() error { return nil })
	if !errors.Is(err, errLQCWorkV1FairPeerPending) {
		t.Fatalf("second same-peer task error=%v", err)
	}

	close(release)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
}

func TestLQCWorkV1FairVerifierConnectedPeerNotStarved(t *testing.T) {
	v := newLQCWorkV1FairVerifier()
	defer v.Close()

	release := make(chan struct{})
	firstDone := make(chan error, 1)

	go func() {
		firstDone <- v.Run("attacker-00", func() error {
			<-release
			return errors.New("fake RandomX claim")
		})
	}()
	waitFairPendingV1(t, v, 1)

	const attackers = 31
	var (
		mu       sync.Mutex
		executed []string
		wg       sync.WaitGroup
	)

	for i := 1; i <= attackers; i++ {
		peerID := fmt.Sprintf("attacker-%02d", i)
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			_ = v.Run(id, func() error {
				mu.Lock()
				executed = append(executed, id)
				mu.Unlock()
				return errors.New("fake RandomX claim")
			})
		}(peerID)
	}

	honestDone := make(chan error, 1)
	go func() {
		honestDone <- v.Run("honest-peer", func() error {
			mu.Lock()
			executed = append(executed, "honest-peer")
			mu.Unlock()
			return nil
		})
	}()

	waitFairPendingV1(t, v, attackers+2)
	close(release)

	if err := <-firstDone; err == nil {
		t.Fatal("fake first task unexpectedly succeeded")
	}
	if err := <-honestDone; err != nil {
		t.Fatalf("honest peer starved/failed: %v", err)
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	honestIndex := -1
	for i, id := range executed {
		if id == "honest-peer" {
			honestIndex = i
			break
		}
	}
	if honestIndex < 0 {
		t.Fatal("honest peer never executed")
	}
	if honestIndex > attackers {
		t.Fatalf("honest execution index=%d attackers=%d", honestIndex, attackers)
	}
}

func TestLQCWorkV1FairVerifierInvalidPeerDoesNotStopQueue(t *testing.T) {
	v := newLQCWorkV1FairVerifier()
	defer v.Close()

	start := make(chan struct{})
	badDone := make(chan error, 1)
	goodDone := make(chan error, 1)

	go func() {
		badDone <- v.Run("bad", func() error {
			<-start
			return errors.New("invalid proof")
		})
	}()
	waitFairPendingV1(t, v, 1)

	go func() {
		goodDone <- v.Run("good", func() error { return nil })
	}()
	waitFairPendingV1(t, v, 2)

	close(start)

	if err := <-badDone; err == nil {
		t.Fatal("bad peer unexpectedly passed")
	}
	if err := <-goodDone; err != nil {
		t.Fatalf("good peer blocked after bad peer: %v", err)
	}
}
