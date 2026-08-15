//go:build linux && cgo

package main

import (
	"encoding/binary"
	"fmt"
	"math"
	"sync"
	"testing"

	"golang.org/x/crypto/argon2"
)

func TestParseBatchSizes(t *testing.T) {
	got, err := parseBatchSizes("1,8,16,32,64")
	if err != nil {
		t.Fatal(err)
	}
	want := []uint64{1, 8, 16, 32, 64}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("batch sizes = %v, want %v", got, want)
		}
	}
	if _, err := parseBatchSizes("1,65"); err == nil {
		t.Fatal("batch size above canonical maximum must fail")
	}
}

func TestReusableBatchOutputMatchesOfficial(t *testing.T) {
	resetReusableWorkspace()
	t.Cleanup(resetReusableWorkspace)
	salt := []byte("RABBIT-LQC-WORK")
	errors := make(chan error, 2)
	var wait sync.WaitGroup
	for worker := uint32(0); worker < 2; worker++ {
		wait.Add(1)
		go func(worker uint32) {
			defer wait.Done()
			input := make([]byte, 40)
			for nonce := uint64(worker); nonce < 16; nonce += 2 {
				binary.BigEndian.PutUint64(input[32:], nonce)
				var actual [32]byte
				if err := reusableArgon2IDInto(worker, input, salt, 8*1024, &actual); err != nil {
					errors <- err
					return
				}
				expected := argon2.IDKey(input, salt, 1, 8*1024, 1, 32)
				if string(actual[:]) != string(expected) {
					errors <- fmt.Errorf("worker %d nonce %d diverged from official Argon2id", worker, nonce)
					return
				}
			}
		}(worker)
	}
	wait.Wait()
	close(errors)
	for err := range errors {
		t.Fatal(err)
	}
	bytes, allocations := reusableWorkspaceStats()
	if bytes != 16*1024*1024 || allocations != 2 {
		t.Fatalf("workspace bytes=%d allocations=%d", bytes, allocations)
	}
}

func TestAnalyzeBatchProfileUsesStableCoreAndAbsoluteBudgets(t *testing.T) {
	opts := options{weakSlowdown: 4, targetBlockTimeMs: 10000, verificationBudget: 5000}
	durations := []float64{900, 910, 920, 930, 940, 950, 960, 970, 980, 990, 1000, 1010, 1020, 1030, 1040, 2000}
	profile := analyzeBatchProfile(64, durations, opts)
	if profile.ProfileStatus != "PASS" || profile.VerificationBudgetStatus != "PASS" || profile.WorstRoundBlockTimeStatus != "PASS" {
		t.Fatalf("expected safe profile, got %+v", profile)
	}
	if math.Abs(profile.EstimatedWeakWorstRoundMs-8000) > 0.001 {
		t.Fatalf("weak worst = %.3f, want 8000", profile.EstimatedWeakWorstRoundMs)
	}
}

func TestAnalyzeBatchProfileRejectsWeakWorstBeyondBlock(t *testing.T) {
	opts := options{weakSlowdown: 4, targetBlockTimeMs: 10000, verificationBudget: 5000}
	durations := []float64{900, 910, 920, 930, 940, 950, 960, 970, 980, 990, 1000, 1010, 1020, 1030, 3000}
	profile := analyzeBatchProfile(64, durations, opts)
	if profile.ProfileStatus != "FAIL" || profile.WorstRoundBlockTimeStatus != "FAIL" {
		t.Fatalf("unsafe weak worst round must fail: %+v", profile)
	}
}
