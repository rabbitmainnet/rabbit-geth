package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math/big"
	"os"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

const auditorVersion = "rabbit-seal-audit/1.0.0"

type options struct {
	rpcURL   string
	from     uint64
	to       uint64
	jsonPath string
	timeout  time.Duration
}

type blockFailure struct {
	Number   uint64         `json:"number"`
	Hash     common.Hash    `json:"hash"`
	Producer common.Address `json:"producer"`
	Error    string         `json:"error"`
}

type report struct {
	AuditorVersion string            `json:"auditorVersion"`
	Status         string            `json:"status"`
	RPC            string            `json:"rpc"`
	ChainID        string            `json:"chainId"`
	FromBlock      uint64            `json:"fromBlock"`
	ToBlock        uint64            `json:"toBlock"`
	Blocks         uint64            `json:"blocks"`
	ValidSeals     uint64            `json:"validSeals"`
	Distinct       int               `json:"distinctProducers"`
	Producers      map[string]uint64 `json:"producers"`
	Failures       []blockFailure    `json:"failures"`
}

type headerReader interface {
	ChainID(context.Context) (*big.Int, error)
	HeaderByNumber(context.Context, *big.Int) (*types.Header, error)
}

func main() {
	opts := parseFlags()
	ctx, cancel := context.WithTimeout(context.Background(), opts.timeout)
	defer cancel()

	client, err := ethclient.DialContext(ctx, opts.rpcURL)
	if err != nil {
		fatalf("connect to RPC: %v", err)
	}
	defer client.Close()

	result, err := audit(ctx, client, opts)
	if err != nil {
		fatalf("audit: %v", err)
	}
	if opts.jsonPath != "" {
		if err := writeJSON(opts.jsonPath, result); err != nil {
			fatalf("gravar JSON: %v", err)
		}
	}

	fmt.Println("LQC SIGNATURE AUDIT COMPLETED")
	fmt.Println("Status:", result.Status)
	fmt.Printf("Blocks: %d | valid signatures: %d | producers: %d\n", result.Blocks, result.ValidSeals, result.Distinct)
	fmt.Printf("Intervalo: %d..%d | chain ID: %s\n", result.FromBlock, result.ToBlock, result.ChainID)
	if result.Status != "PASS" {
		os.Exit(1)
	}
}

func parseFlags() options {
	var opts options
	flag.StringVar(&opts.rpcURL, "rpc", "", "HTTP, WebSocket, or IPC RPC endpoint")
	flag.Uint64Var(&opts.from, "from", 1, "first block to audit")
	flag.Uint64Var(&opts.to, "to", 0, "last block; zero uses the current head")
	flag.StringVar(&opts.jsonPath, "json", "", "optional JSON file")
	flag.DurationVar(&opts.timeout, "timeout", 5*time.Minute, "maximum audit duration")
	flag.Parse()
	if opts.rpcURL == "" {
		fatalf("informe --rpc")
	}
	return opts
}

func audit(ctx context.Context, client headerReader, opts options) (*report, error) {
	chainID, err := client.ChainID(ctx)
	if err != nil {
		return nil, fmt.Errorf("query chain ID: %w", err)
	}
	if chainID == nil || chainID.Sign() <= 0 {
		return nil, errors.New("invalid chain ID")
	}
	to := opts.to
	if to == 0 {
		head, err := client.HeaderByNumber(ctx, nil)
		if err != nil {
			return nil, fmt.Errorf("query head: %w", err)
		}
		if head == nil || head.Number == nil || !head.Number.IsUint64() {
			return nil, errors.New("invalid canonical head")
		}
		to = head.Number.Uint64()
	}
	if opts.from == 0 || to < opts.from {
		return nil, fmt.Errorf("invalid range: %d..%d", opts.from, to)
	}

	result := &report{
		AuditorVersion: auditorVersion,
		Status:         "PASS",
		RPC:            opts.rpcURL,
		ChainID:        chainID.String(),
		FromBlock:      opts.from,
		ToBlock:        to,
		Blocks:         to - opts.from + 1,
		Producers:      make(map[string]uint64),
		Failures:       make([]blockFailure, 0),
	}
	for number := opts.from; number <= to; number++ {
		header, err := client.HeaderByNumber(ctx, new(big.Int).SetUint64(number))
		if err != nil {
			return nil, fmt.Errorf("query block %d: %w", number, err)
		}
		if header == nil || header.Number == nil || !header.Number.IsUint64() || header.Number.Uint64() != number {
			return nil, fmt.Errorf("RPC returned an invalid header for block %d", number)
		}
		producer := header.Coinbase.Hex()
		result.Producers[producer]++
		if err := lqc.VerifyProducerSeal(chainID, header); err != nil {
			result.Status = "FAIL"
			result.Failures = append(result.Failures, blockFailure{
				Number:   number,
				Hash:     header.Hash(),
				Producer: header.Coinbase,
				Error:    err.Error(),
			})
		} else {
			result.ValidSeals++
		}
		if number == ^uint64(0) {
			break
		}
	}
	result.Distinct = len(result.Producers)
	return result, nil
}

func writeJSON(path string, value any) error {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	return os.WriteFile(path, encoded, 0o600)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "ERRO: "+format+"\n", args...)
	os.Exit(1)
}
