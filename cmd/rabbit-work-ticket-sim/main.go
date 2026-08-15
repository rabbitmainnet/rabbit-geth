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
		fmt.Fprintln(os.Stderr, "ERRO:", err)
		os.Exit(1)
	}
	printSummary(result)
	if opts.outputDir != "" {
		if err := writeReports(opts.outputDir, result); err != nil {
			fmt.Fprintln(os.Stderr, "ERRO:", err)
			os.Exit(1)
		}
	}
}

func parseFlags() options {
	var opts options
	flag.IntVar(&opts.honestMiners, "honest", 20, "mineradores honestos com uma unidade de trabalho cada")
	flag.StringVar(&opts.identityScenarios, "identities", "1,10,100,1000,5000", "identidades controladas pelo mesmo participante")
	flag.StringVar(&opts.workScenarios, "work", "1,5,20,100", "unidades de trabalho do participante adversarial")
	flag.Uint64Var(&opts.slots, "slots", 2000, "seleções determinísticas por cenário")
	flag.Uint64Var(&opts.ticketsPerWork, "tickets-per-work", 32, "tickets simulados por unidade de trabalho")
	flag.Uint64Var(&opts.fallbacks, "fallbacks", 5, "posições de fallback")
	flag.Uint64Var(&opts.committeeMin, "committee-min", 32, "committee mínimo")
	flag.Uint64Var(&opts.committeeMax, "committee-max", 128, "committee máximo")
	flag.StringVar(&opts.outputDir, "output", "", "diretório dos relatórios")
	flag.Parse()
	return opts
}

func validateOptions(opts options) error {
	if opts.honestMiners <= 0 || opts.slots == 0 || opts.ticketsPerWork == 0 {
		return errors.New("honest, slots e tickets-per-work devem ser maiores que zero")
	}
	if opts.committeeMax > 0 && opts.committeeMin > opts.committeeMax {
		return errors.New("committee-min não pode exceder committee-max")
	}
	return nil
}
