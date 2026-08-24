package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/rpc"
)

type commandOptions struct {
	audit        auditOptions
	jsonPath     string
	csvPath      string
	markdownPath string
}

func main() {
	os.Exit(runMain())
}

func runMain() int {
	options := parseFlags()
	genesis, err := loadGenesis(options.audit.GenesisPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ERRO:", err)
		return 1
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	client, err := rpc.DialContext(ctx, options.audit.RPCEndpoint)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ERROR: could not connect to the lab:", err)
		return 1
	}
	defer client.Close()
	fmt.Println("Rabbit Chain — professional reward audit")
	fmt.Println("RPC:", options.audit.RPCEndpoint)
	fmt.Println("Genesis:", options.audit.GenesisPath)
	runner := newAuditRunner(options.audit, genesis, client)
	report, err := runner.run(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ERRO:", err)
		return 1
	}
	if err := writeJSONReport(options.jsonPath, report); err != nil {
		fmt.Fprintln(os.Stderr, "ERROR writing JSON:", err)
		return 1
	}
	if err := writeCSVReport(options.csvPath, report); err != nil {
		fmt.Fprintln(os.Stderr, "ERROR writing CSV:", err)
		return 1
	}
	if err := writeMarkdownReport(options.markdownPath, report); err != nil {
		fmt.Fprintln(os.Stderr, "ERROR writing summary:", err)
		return 1
	}
	fmt.Println()
	fmt.Println("AUDIT COMPLETED")
	fmt.Println("Status geral:", report.Status)
	fmt.Println("Runtime rewards:", report.RewardRuntimeStatus)
	fmt.Println("Consensus architecture:", report.ArchitectureStatus)
	fmt.Printf("Blocks: %d | passed: %d | failures: %d | incomplete: %d\n",
		report.Summary.BlocksScanned,
		report.Summary.PassingBlocks,
		report.Summary.FailingBlocks,
		report.Summary.IncompleteBlocks,
	)
	fmt.Println("Expected issuance (wei):", report.Supply.ExpectedScannedEmissionWei)
	fmt.Println("Observed issuance (wei):", report.Supply.ObservedScannedEmissionWei)
	fmt.Println("Difference (wei):", report.Supply.ScannedDifferenceWei)
	fmt.Println("Resumo:", options.markdownPath)
	fmt.Println("Detalhes JSON:", options.jsonPath)
	fmt.Println("Detalhes CSV:", options.csvPath)
	if report.Status == "FAIL" {
		return 2
	}
	if report.Status == "INCOMPLETE" {
		return 3
	}
	return 0
}

func parseFlags() commandOptions {
	var options commandOptions
	flag.StringVar(&options.audit.RPCEndpoint, "rpc", "/tmp/rabbit-20nodes/node1/geth.ipc", "node HTTP, WebSocket, or IPC endpoint")
	flag.StringVar(&options.audit.GenesisPath, "genesis", "/tmp/rabbit-20nodes/genesis-runtime.json", "exact genesis used by the lab")
	flag.Uint64Var(&options.audit.FromBlock, "from", 1, "first block in the audit")
	flag.Uint64Var(&options.audit.ToBlock, "to", 0, "last block; zero uses the current height")
	flag.Uint64Var(&options.audit.ProgressEvery, "progress", 100, "show progress every N blocks; zero disables it")
	flag.StringVar(&options.jsonPath, "json", "rabbit-reward-audit.json", "detailed JSON report")
	flag.StringVar(&options.csvPath, "csv", "rabbit-reward-audit.csv", "per-recipient CSV report")
	flag.StringVar(&options.markdownPath, "summary", "rabbit-reward-audit.md", "readable Markdown summary")
	flag.Parse()
	return options
}

func loadGenesis(path string) (*core.Genesis, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("abrir genesis %s: %w", path, err)
	}
	defer file.Close()
	var genesis core.Genesis
	if err := json.NewDecoder(file).Decode(&genesis); err != nil {
		return nil, fmt.Errorf("ler genesis %s: %w", path, err)
	}
	if genesis.Config == nil {
		return nil, fmt.Errorf("genesis has no config")
	}
	if genesis.Config.ChainID == nil {
		return nil, fmt.Errorf("genesis has no chainId")
	}
	if genesis.Config.LQC == nil {
		return nil, fmt.Errorf("genesis does not use LQC consensus")
	}
	if len(genesis.Config.LQC.BootstrapParticipants) == 0 {
		return nil, fmt.Errorf("genesis has no bootstrapParticipants; the committee cannot be reconstructed")
	}
	return &genesis, nil
}

func writeJSONReport(path string, report *auditReport) error {
	return writeAtomically(path, func(writer io.Writer) error {
		encoder := json.NewEncoder(writer)
		encoder.SetIndent("", "  ")
		return encoder.Encode(report)
	})
}

func writeCSVReport(path string, report *auditReport) error {
	return writeAtomically(path, func(writer io.Writer) error {
		csvWriter := csv.NewWriter(writer)
		defer csvWriter.Flush()
		headers := []string{
			"block", "hash", "era", "producer", "queue_position", "committee",
			"recipient", "role", "expected_wei", "observed_emission_wei", "difference_wei",
			"balance_delta_wei", "transaction_delta_wei", "consensus_liquid_delta_wei",
			"locked_delta_wei", "released_wei", "match", "block_status",
		}
		if err := csvWriter.Write(headers); err != nil {
			return err
		}
		for _, block := range report.Blocks {
			for _, allocation := range block.Allocations {
				record := []string{
					strconv.FormatUint(block.Number, 10),
					block.Hash,
					strconv.FormatUint(block.Era, 10),
					block.Producer,
					strconv.Itoa(block.QueuePosition),
					joinStrings(block.Committee, ";"),
					allocation.Address,
					allocation.Role,
					allocation.ExpectedWei,
					allocation.ObservedEmissionWei,
					allocation.DifferenceWei,
					allocation.ObservedBalanceDeltaWei,
					allocation.TransactionBalanceDeltaWei,
					allocation.ConsensusLiquidDeltaWei,
					allocation.ObservedLockedDeltaWei,
					allocation.ObservedReleaseWei,
					strconv.FormatBool(allocation.Match),
					block.Status,
				}
				if err := csvWriter.Write(record); err != nil {
					return err
				}
			}
		}
		csvWriter.Flush()
		return csvWriter.Error()
	})
}

func writeMarkdownReport(path string, report *auditReport) error {
	return writeAtomically(path, func(writer io.Writer) error {
		fmt.Fprintln(writer, "# Reward audit — Rabbit Chain")
		fmt.Fprintln(writer)
		fmt.Fprintf(writer, "**Status geral: %s**\n\n", report.Status)
		fmt.Fprintf(writer, "- Runtime rewards: **%s**\n", report.RewardRuntimeStatus)
		fmt.Fprintf(writer, "- Consensus architecture: **%s**\n\n", report.ArchitectureStatus)
		fmt.Fprintf(writer, "Audited blocks: `%d` through `%d` (%d blocks).\n\n", report.FromBlock, report.ToBlock, report.Summary.BlocksScanned)
		fmt.Fprintln(writer, "## Resumo")
		fmt.Fprintln(writer)
		fmt.Fprintln(writer, "| Check | Result |")
		fmt.Fprintln(writer, "| --- | ---: |")
		fmt.Fprintf(writer, "| Passing blocks | %d |\n", report.Summary.PassingBlocks)
		fmt.Fprintf(writer, "| Failing blocks | %d |\n", report.Summary.FailingBlocks)
		fmt.Fprintf(writer, "| Incomplete blocks | %d |\n", report.Summary.IncompleteBlocks)
		fmt.Fprintf(writer, "| Reward mismatches | %d |\n", report.Summary.RewardMismatchBlocks)
		fmt.Fprintf(writer, "| Balance/immediate reward mismatches | %d |\n", report.Summary.StateMismatchBlocks)
		fmt.Fprintf(writer, "| Unexpected legacy vesting changes | %d |\n", report.Summary.VestingIndexMismatchBlocks)
		fmt.Fprintf(writer, "| Producers outside the queue | %d |\n", report.Summary.UnauthorizedProducerBlocks)
		fmt.Fprintln(writer)
		fmt.Fprintln(writer, "## Issuance")
		fmt.Fprintln(writer)
		fmt.Fprintf(writer, "- Expected in range: `%s wei`\n", report.Supply.ExpectedScannedEmissionWei)
		fmt.Fprintf(writer, "- Observed in range: `%s wei`\n", report.Supply.ObservedScannedEmissionWei)
		fmt.Fprintf(writer, "- Difference: `%s wei`\n", report.Supply.ScannedDifferenceWei)
		fmt.Fprintf(writer, "- Scheduled issuance through block %d: `%s RAB`\n", report.ToBlock, report.Supply.ScheduledEmissionThroughToRAB)
		fmt.Fprintf(writer, "- Terminal reward: `%s RAB per block`\n", report.Supply.TerminalRewardRAB)
		fmt.Fprintln(writer)
		fmt.Fprintln(writer, "## Observed engine and selection")
		fmt.Fprintln(writer)
		fmt.Fprintf(writer, "- Engine connected by the client: `%s`\n", report.Config.Engine)
		fmt.Fprintf(writer, "- Selection sizing rule: `%s`\n", report.Config.SelectionSizing)
		fmt.Fprintf(writer, "- Registry source: `canonical headers since block %d`\n", report.Config.RegistryProtocolBlock)
		fmt.Fprintf(writer, "- Participantes bootstrap: `%d`\n", len(report.Config.BootstrapParticipants))
		fmt.Fprintf(writer, "- Reward mode: `%s`\n", report.Config.RewardMode)
		fmt.Fprintf(writer, "- Fallbacks efetivos: `%d`\n", report.Config.FallbackCount)
		if report.Config.CommitteeSize > 0 {
			fmt.Fprintf(writer, "- Committee fixo: `%d`\n", report.Config.CommitteeSize)
		} else {
			fmt.Fprintf(writer, "- Dynamic committee: minimum `%d`, maximum `%d`\n", report.Config.CommitteeMin, report.Config.CommitteeMax)
		}
		fmt.Fprintln(writer)
		fmt.Fprintln(writer, "## Achados")
		fmt.Fprintln(writer)
		if len(report.Findings) == 0 {
			fmt.Fprintln(writer, "No inconsistencies found.")
		} else {
			for _, item := range report.Findings {
				fmt.Fprintf(writer, "### %s — %s\n\n%s\n\n", item.Severity, item.Code, item.Description)
			}
		}
		fmt.Fprintln(writer, "## Observed eras")
		fmt.Fprintln(writer)
		fmt.Fprintln(writer, "| Era | Blocks | Reward (RAB) | Expected (wei) | Observed (wei) | Difference (wei) |")
		fmt.Fprintln(writer, "| ---: | ---: | ---: | ---: | ---: | ---: |")
		for _, era := range report.Eras {
			fmt.Fprintf(writer, "| %d | %d | %s | %s | %s | %s |\n",
				era.Era, era.BlocksScanned, era.RewardPerBlockRAB, era.ExpectedEmissionWei, era.ObservedEmissionWei, era.DifferenceWei)
		}
		fmt.Fprintln(writer)
		fmt.Fprintln(writer, "Details for each block and recipient are available in the JSON and CSV reports.")
		return nil
	})
}

func writeAtomically(path string, write func(io.Writer) error) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".rabbit-audit-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	ok := false
	defer func() {
		temporary.Close()
		if !ok {
			os.Remove(temporaryPath)
		}
	}()
	if err := write(temporary); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return err
	}
	ok = true
	return nil
}

func joinStrings(values []string, separator string) string {
	if len(values) == 0 {
		return ""
	}
	result := values[0]
	for _, value := range values[1:] {
		result += separator + value
	}
	return result
}
