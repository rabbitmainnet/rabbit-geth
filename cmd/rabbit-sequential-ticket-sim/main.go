package main

import (
	"flag"
	"fmt"
	"os"
)

const simulatorVersion = "rabbit-lqc-sequential-ticket-simulator/1.0.0"

func main() {
	opts := parseFlags()
	report, err := runSimulation(opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
		os.Exit(1)
	}
	printSummary(report)
	if opts.outputDir != "" {
		if err := writeReports(opts.outputDir, report); err != nil {
			fmt.Fprintln(os.Stderr, "ERROR:", err)
			os.Exit(1)
		}
	}
}

func parseFlags() options {
	var opts options
	flag.Uint64Var(&opts.honestParticipants, "honest", 20, "honest participants, each with one lane")
	flag.StringVar(&opts.identities, "identities", "1,10,100,1000,5000", "controlled identities with fixed work")
	flag.StringVar(&opts.attackerLanes, "attacker-lanes", "1,2,4,8,16,32,64,256", "actual attacker work lanes")
	flag.StringVar(&opts.networkSizes, "network-sizes", "20,100,1000,10000", "honest lane counts for the scale test")
	flag.Uint64Var(&opts.scaleAttackerLanes, "scale-attacker-lanes", 64, "attacker lanes in the scale test")
	flag.Uint64Var(&opts.fallbacks, "fallbacks", 5, "number of fallbacks")
	flag.Uint64Var(&opts.committeeSize, "committee", 32, "target committee size")
	flag.StringVar(&opts.outputDir, "output", "", "report directory")
	flag.Parse()
	return opts
}
