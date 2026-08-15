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
		fmt.Fprintln(os.Stderr, "ERRO:", err)
		os.Exit(1)
	}
	printSummary(report)
	if opts.outputDir != "" {
		if err := writeReports(opts.outputDir, report); err != nil {
			fmt.Fprintln(os.Stderr, "ERRO:", err)
			os.Exit(1)
		}
	}
}

func parseFlags() options {
	var opts options
	flag.Uint64Var(&opts.honestParticipants, "honest", 20, "participantes honestos, cada um com uma lane")
	flag.StringVar(&opts.identities, "identities", "1,10,100,1000,5000", "identidades controladas com trabalho fixo")
	flag.StringVar(&opts.attackerLanes, "attacker-lanes", "1,2,4,8,16,32,64,256", "lanes reais de trabalho do atacante")
	flag.StringVar(&opts.networkSizes, "network-sizes", "20,100,1000,10000", "quantidades de lanes honestas para o teste de escala")
	flag.Uint64Var(&opts.scaleAttackerLanes, "scale-attacker-lanes", 64, "lanes do atacante no teste de escala")
	flag.Uint64Var(&opts.fallbacks, "fallbacks", 5, "quantidade de fallbacks")
	flag.Uint64Var(&opts.committeeSize, "committee", 32, "tamanho alvo do committee")
	flag.StringVar(&opts.outputDir, "output", "", "diretório dos relatórios")
	flag.Parse()
	return opts
}
