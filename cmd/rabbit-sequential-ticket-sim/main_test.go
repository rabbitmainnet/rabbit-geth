package main

import (
	"math"
	"testing"
)

func testOptions() options {
	return options{
		honestParticipants: 20,
		identities:         "1,10,1000",
		attackerLanes:      "1,4,16,64",
		networkSizes:       "20,100,1000",
		scaleAttackerLanes: 64,
		fallbacks:          5,
		committeeSize:      32,
	}
}

func TestIdentityCountDoesNotAmplifyFixedWork(t *testing.T) {
	report, err := runSimulation(testOptions())
	if err != nil {
		t.Fatal(err)
	}
	first := report.FixedWorkIdentityScenarios[0].selectionResult
	for _, scenario := range report.FixedWorkIdentityScenarios[1:] {
		if scenario.selectionResult != first {
			t.Fatalf("identity amplification detected: first=%+v scenario=%+v", first, scenario)
		}
	}
	if report.IdentityAmplificationStatus != "PASS" || report.MainnetGate != "BLOCKED" {
		t.Fatalf("unsafe report status: %+v", report)
	}
}

func TestProducerShareMatchesResourceShare(t *testing.T) {
	result := calculateSelection(20, 5, 5, 16)
	want := 20.0
	if math.Abs(result.AttackerProducerPercent-want) > 0.0001 {
		t.Fatalf("producer share %.6f want %.6f", result.AttackerProducerPercent, want)
	}
}

func TestCommitteeMajorityBoundaries(t *testing.T) {
	if got := hypergeometricMajorityProbability(1, 20, 21); got != 0 {
		t.Fatalf("one attacker cannot have majority: %.8f", got)
	}
	if got := hypergeometricMajorityProbability(20, 1, 21); got < 0.999999 {
		t.Fatalf("twenty attackers must have majority: %.8f", got)
	}
}

func TestSimulationDoesNotClaimCryptographicImplementation(t *testing.T) {
	report, err := runSimulation(testOptions())
	if err != nil {
		t.Fatal(err)
	}
	if report.CryptographicVDFStatus != "NOT_IMPLEMENTED" || report.ImplementationStatus != "SIMULATION_ONLY" {
		t.Fatalf("unsafe implementation claim: %+v", report)
	}
	if report.ConsensusChanged || report.GenesisChanged {
		t.Fatalf("simulation changed protected state: %+v", report)
	}
}
