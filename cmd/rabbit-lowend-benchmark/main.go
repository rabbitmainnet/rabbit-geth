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
	flag.StringVar(&opts.memoryProfiles, "memory-mib", "8,16,32,64", "Argon2id memory profiles in MiB")
	flag.DurationVar(&opts.duration, "duration", 750*time.Millisecond, "minimum duration per profile")
	flag.UintVar(&opts.rounds, "rounds", 5, "number of interleaved rounds per profile")
	flag.UintVar(&opts.warmups, "warmups", 1, "warm-up operations before each round")
	flag.UintVar(&opts.isolatedSamples, "isolated-samples", 7, "samples with memory collection outside the timed interval")
	flag.UintVar(&opts.iterations, "iterations", 1, "Argon2id iterations")
	flag.UintVar(&opts.parallelism, "parallelism", 1, "lanes Argon2id")
	flag.Float64Var(&opts.weakSlowdown, "weak-slowdown", 4, "estimated slowdown of a low-end PC")
	flag.Uint64Var(&opts.epochSeconds, "epoch-seconds", 1280, "reference epoch duration")
	flag.Float64Var(&opts.targetSuccess, "target-success", 0.80, "target probability of at least one ticket per epoch")
	flag.Float64Var(&opts.verifyBudgetMs, "verify-budget-ms", 1000, "verification budget per block")
	flag.Uint64Var(&opts.maxTicketsBlock, "max-tickets-block", 64, "upper limit of tickets per block")
	flag.StringVar(&opts.outputDir, "output", "", "report directory")
	flag.Parse()
	return opts
}

func validateOptions(opts options) error {
	if opts.duration <= 0 || opts.rounds < 3 || opts.rounds > 21 || opts.warmups == 0 || opts.warmups > 10 {
		return errors.New("duration, rounds, and warmups must be valid")
	}
	if opts.isolatedSamples < 5 || opts.isolatedSamples > 101 {
		return errors.New("isolated-samples must be between 5 and 101")
	}
	if opts.iterations == 0 || opts.parallelism == 0 || opts.parallelism > 255 {
		return errors.New("iterations and parallelism must be valid")
	}
	if opts.weakSlowdown < 1 || opts.epochSeconds == 0 || opts.targetSuccess <= 0 || opts.targetSuccess >= 1 {
		return errors.New("invalid weak-slowdown, epoch-seconds, or target-success")
	}
	if opts.verifyBudgetMs <= 0 || opts.maxTicketsBlock == 0 {
		return errors.New("budget and ticket limit must be greater than zero")
	}
	return nil
}
