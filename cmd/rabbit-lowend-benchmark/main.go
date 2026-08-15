package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"time"
)

const benchmarkVersion = "rabbit-lowend-accessibility-benchmark/1.0.3"

func main() {
	opts := parseFlags()
	report, err := runBenchmark(opts)
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
	flag.StringVar(&opts.memoryProfiles, "memory-mib", "8,16,32,64", "perfis de memória Argon2id em MiB")
	flag.DurationVar(&opts.duration, "duration", 750*time.Millisecond, "duração mínima por perfil")
	flag.UintVar(&opts.rounds, "rounds", 5, "quantidade de rodadas intercaladas por perfil")
	flag.UintVar(&opts.warmups, "warmups", 1, "operações de aquecimento antes de cada rodada")
	flag.UintVar(&opts.isolatedSamples, "isolated-samples", 7, "amostras com coleta de memória fora do cronômetro")
	flag.UintVar(&opts.iterations, "iterations", 1, "iterações Argon2id")
	flag.UintVar(&opts.parallelism, "parallelism", 1, "lanes Argon2id")
	flag.Float64Var(&opts.weakSlowdown, "weak-slowdown", 4, "estimativa de quanto o PC fraco é mais lento")
	flag.Uint64Var(&opts.epochSeconds, "epoch-seconds", 1280, "duração da época de referência")
	flag.Float64Var(&opts.targetSuccess, "target-success", 0.80, "probabilidade alvo de ao menos um ticket por época")
	flag.Float64Var(&opts.verifyBudgetMs, "verify-budget-ms", 1000, "orçamento de verificação por bloco")
	flag.Uint64Var(&opts.maxTicketsBlock, "max-tickets-block", 64, "limite superior de tickets por bloco")
	flag.StringVar(&opts.outputDir, "output", "", "diretório dos relatórios")
	flag.Parse()
	return opts
}

func validateOptions(opts options) error {
	if opts.duration <= 0 || opts.rounds < 3 || opts.rounds > 21 || opts.warmups == 0 || opts.warmups > 10 {
		return errors.New("duration, rounds e warmups devem ser válidos")
	}
	if opts.isolatedSamples < 5 || opts.isolatedSamples > 101 {
		return errors.New("isolated-samples deve estar entre 5 e 101")
	}
	if opts.iterations == 0 || opts.parallelism == 0 || opts.parallelism > 255 {
		return errors.New("iterations e parallelism devem ser válidos")
	}
	if opts.weakSlowdown < 1 || opts.epochSeconds == 0 || opts.targetSuccess <= 0 || opts.targetSuccess >= 1 {
		return errors.New("weak-slowdown, epoch-seconds e target-success inválidos")
	}
	if opts.verifyBudgetMs <= 0 || opts.maxTicketsBlock == 0 {
		return errors.New("orçamento e limite de tickets devem ser maiores que zero")
	}
	return nil
}
