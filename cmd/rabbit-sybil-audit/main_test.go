package main

import (
	"testing"
)

func TestParseScenariosRejectsInvalidValues(t *testing.T) {
	if _, err := parseScenarios("10,0,100"); err == nil {
		t.Fatal("expected invalid zero scenario")
	}
	got, err := parseScenarios("10,100,10")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != 10 || got[1] != 100 {
		t.Fatalf("unexpected scenarios: %v", got)
	}
}

func TestExactLCQSelectionExposesMassIdentityDominance(t *testing.T) {
	opts := options{
		honest:       20,
		blocks:       1000,
		fallbacks:    5,
		committeeMin: 32,
		committeeMax: 128,
		difficulty:   100000,
	}
	got := analyzeScenario(opts, 1000, 1_000_000)
	if got.ProducerSharePercent < 94 {
		t.Fatalf("producer share %.2f%% is unexpectedly low", got.ProducerSharePercent)
	}
	if got.FallbackSharePercent < 94 {
		t.Fatalf("fallback share %.2f%% is unexpectedly low", got.FallbackSharePercent)
	}
	if got.CommitteeSharePercent < 94 {
		t.Fatalf("committee share %.2f%% is unexpectedly low", got.CommitteeSharePercent)
	}
	if got.CommitteeMajorityPercent < 99 {
		t.Fatalf("committee majority %.2f%% is unexpectedly low", got.CommitteeMajorityPercent)
	}
	if !got.DominatesProducerSelection || !got.DominatesCommittee {
		t.Fatalf("mass identities did not trigger dominance flags: %+v", got)
	}
}

func TestRealSignedLightHashOperationsEnterCanonicalRegistry(t *testing.T) {
	opts := options{
		proofSamples: 3,
		difficulty:   16,
		chainID:      928,
	}
	got, err := benchmarkProofs(opts)
	if err != nil {
		t.Fatal(err)
	}
	if got.ValidatedSignedOperations != 3 || got.AppliedCanonicalOperations != 3 {
		t.Fatalf("unexpected benchmark result: %+v", got)
	}
}
