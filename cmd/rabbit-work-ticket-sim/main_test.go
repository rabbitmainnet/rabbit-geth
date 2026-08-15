package main

import "testing"

func testOptions() options {
	return options{
		honestMiners:      20,
		identityScenarios: "1,10,100,1000",
		workScenarios:     "1,5,20",
		slots:             500,
		ticketsPerWork:    16,
		fallbacks:         5,
		committeeMin:      32,
		committeeMax:      128,
	}
}

func TestMassIdentitiesDoNotAmplifyFixedWork(t *testing.T) {
	opts := testOptions()
	var shares []float64
	for _, identities := range []int{1, 10, 100, 1000, 5000} {
		metrics := simulateWorkTicketRule(opts, identities, 1)
		shares = append(shares, metrics.ProducerPercent)
		if metrics.ProducerPercent > 10 {
			t.Fatalf("%d identities amplified producer share to %.2f%%", identities, metrics.ProducerPercent)
		}
		if metrics.CommitteeMajorityPercent != 0 {
			t.Fatalf("%d identities obtained committee majority %.2f%%", identities, metrics.CommitteeMajorityPercent)
		}
	}
	if spread(shares) > 10 {
		t.Fatalf("identity-dependent spread is too high: %.2f points (%v)", spread(shares), shares)
	}
}

func TestCurrentAddressRuleStillReproducesVulnerability(t *testing.T) {
	opts := testOptions()
	metrics := simulateCurrentAddressRule(opts, 1000)
	if metrics.ProducerPercent < 90 || metrics.CommitteeMajorityPercent < 99 {
		t.Fatalf("current vulnerability was not reproduced: %+v", metrics)
	}
}

func TestWorkNotIdentityDeterminesCandidateShare(t *testing.T) {
	opts := testOptions()
	low := simulateWorkTicketRule(opts, 5000, 1)
	high := simulateWorkTicketRule(opts, 5000, 20)
	if low.ProducerPercent >= high.ProducerPercent {
		t.Fatalf("more work did not increase selection: low=%+v high=%+v", low, high)
	}
}

func TestSimulationDoesNotClaimConsensusIsImplemented(t *testing.T) {
	report, err := runSimulation(testOptions())
	if err != nil {
		t.Fatal(err)
	}
	if report.ImplementationStatus != "SIMULATION_ONLY" || report.MainnetGate != "BLOCKED" {
		t.Fatalf("unsafe status: %+v", report)
	}
	if report.ConsensusChanged || report.GenesisChanged {
		t.Fatalf("simulation must not claim mutations: %+v", report)
	}
}
