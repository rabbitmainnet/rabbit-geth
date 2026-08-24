package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
)

const simulatorVersion = "rabbit-lqc-work-ticket-simulator/1.0.0"

func main() {
	opts := parseFlags()
	result, err := runSimulation(opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
		os.Exit(1)
	}
	printSummary(result)
	if opts.outputDir != "" {
		if err := writeReports(opts.outputDir, result); err != nil {
			fmt.Fprintln(os.Stderr, "ERROR:", err)
			os.Exit(1)
		}
	}
}

func parseFlags() options {
	var opts options
	flag.IntVar(&opts.honestMiners, "honest", 20, "honest miners with one work unit each")
	flag.StringVar(&opts.identityScenarios, "identities", "1,10,100,1000,5000", "identities controlled by the same participant")
	flag.StringVar(&opts.workScenarios, "work", "1,5,20,100", "adversarial participant work units")
	flag.Uint64Var(&opts.slots, "slots", 2000, "deterministic selections per scenario")
	flag.Uint64Var(&opts.ticketsPerWork, "tickets-per-work", 32, "simulated tickets per work unit")
	flag.Uint64Var(&opts.fallbacks, "fallbacks", 5, "fallback positions")
	flag.Uint64Var(&opts.committeeMin, "committee-min", 32, "minimum committee")
	flag.Uint64Var(&opts.committeeMax, "committee-max", 128, "maximum committee")
	flag.StringVar(&opts.outputDir, "output", "", "report directory")
	flag.Parse()
	return opts
}

func validateOptions(opts options) error {
	if opts.honestMiners <= 0 || opts.slots == 0 || opts.ticketsPerWork == 0 {
		return errors.New("honest, slots, and tickets-per-work must be greater than zero")
	}
	if opts.committeeMax > 0 && opts.committeeMin > opts.committeeMax {
		return errors.New("committee-min cannot exceed committee-max")
	}
	return nil
}
