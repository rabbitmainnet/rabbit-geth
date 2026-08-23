// Copyright 2026 The Rabbit Chain Authors
// This file is part of the Rabbit Chain library.

// rabbit-miner performs Rabbit Work V1 locally and submits only successful,
// signed RandomX tickets to a Rabbit node. The private key never leaves this
// process and is never sent to the JSON-RPC endpoint.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"math/big"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/rabbitx"
	"github.com/ethereum/go-ethereum/rpc"
)

const (
	officialChainID = uint64(9280)
	pollInterval    = 3 * time.Second
)

type workContext struct {
	Epoch           hexutil.Uint64 `json:"epoch"`
	DatasetAnchor   common.Hash    `json:"datasetAnchor"`
	ChallengeAnchor common.Hash    `json:"challengeAnchor"`
	Difficulty      *hexutil.Big   `json:"difficulty"`
}

type candidateArgs struct {
	Version     hexutil.Uint64 `json:"version"`
	Epoch       hexutil.Uint64 `json:"epoch"`
	Participant common.Address `json:"participant"`
	Nonce       hexutil.Uint64 `json:"nonce"`
	ProofHash   common.Hash    `json:"proofHash"`
	Signature   hexutil.Bytes  `json:"signature"`
}

type options struct {
	rpcURL          string
	keyFile         string
	passwordFile    string
	startNonce      uint64
	ticketsPerEpoch uint64
	statusEvery     uint64
	once            bool
}

func parseOptions() options {
	var opts options
	flag.StringVar(&opts.rpcURL, "rpc", "http://127.0.0.1:8545", "Rabbit JSON-RPC URL")
	flag.StringVar(&opts.keyFile, "keystore", "", "encrypted Web3 keystore file")
	flag.StringVar(&opts.passwordFile, "password-file", "", "file containing the keystore password")
	flag.Uint64Var(&opts.startNonce, "start-nonce", uint64(time.Now().UnixNano()), "first nonce to try")
	flag.Uint64Var(&opts.ticketsPerEpoch, "tickets-per-epoch", 1, "successful tickets to submit per epoch (0 means unlimited)")
	flag.Uint64Var(&opts.statusEvery, "status-every", 10000, "print progress after this many attempts (0 disables)")
	flag.BoolVar(&opts.once, "once", false, "stop after the first accepted ticket")
	flag.Parse()
	return opts
}

func readPassword(path string) (string, error) {
	if path == "" {
		return "", errors.New("missing --password-file")
	}
	encoded, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read password file: %w", err)
	}
	password := strings.TrimRight(string(encoded), "\r\n")
	if password == "" {
		return "", errors.New("empty password")
	}
	return password, nil
}

func resolveKeyFile(path string) (string, error) {
	if path == "" {
		return "", errors.New("missing --keystore")
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("open keystore path: %w", err)
	}
	if !info.IsDir() {
		return path, nil
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return "", fmt.Errorf("read keystore directory: %w", err)
	}
	var files []string
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		files = append(files, filepath.Join(path, entry.Name()))
	}
	if len(files) != 1 {
		return "", fmt.Errorf("keystore directory contains %d files; select one file explicitly", len(files))
	}
	return files[0], nil
}

func loadKey(keyFile, passwordFile string) (*keystore.Key, error) {
	resolved, err := resolveKeyFile(keyFile)
	if err != nil {
		return nil, err
	}
	password, err := readPassword(passwordFile)
	if err != nil {
		return nil, err
	}
	encoded, err := os.ReadFile(resolved)
	if err != nil {
		return nil, fmt.Errorf("read keystore: %w", err)
	}
	key, err := keystore.DecryptKey(encoded, password)
	if err != nil {
		return nil, fmt.Errorf("decrypt keystore: %w", err)
	}
	return key, nil
}

func zeroPrivateKey(key *keystore.Key) {
	if key == nil || key.PrivateKey == nil || key.PrivateKey.D == nil {
		return
	}
	bits := key.PrivateKey.D.Bits()
	for i := range bits {
		bits[i] = 0
	}
}

func fetchChainID(ctx context.Context, client *rpc.Client) (*big.Int, error) {
	var encoded hexutil.Big
	if err := client.CallContext(ctx, &encoded, "eth_chainId"); err != nil {
		return nil, err
	}
	chainID := new(big.Int).Set((*big.Int)(&encoded))
	if !chainID.IsUint64() || chainID.Uint64() != officialChainID {
		return nil, fmt.Errorf("wrong chain ID %s; expected %d", chainID, officialChainID)
	}
	return chainID, nil
}

func fetchWorkContext(ctx context.Context, client *rpc.Client) (workContext, error) {
	var result workContext
	if err := client.CallContext(ctx, &result, "lqc_workV1Context"); err != nil {
		return workContext{}, err
	}
	if uint64(result.Epoch) == 0 || result.DatasetAnchor == (common.Hash{}) ||
		result.ChallengeAnchor == (common.Hash{}) || result.Difficulty == nil ||
		(*big.Int)(result.Difficulty).Sign() <= 0 {
		return workContext{}, errors.New("invalid Work V1 context")
	}
	return result, nil
}

func submitCandidate(ctx context.Context, client *rpc.Client, args candidateArgs) (common.Hash, error) {
	var accepted common.Hash
	if err := client.CallContext(ctx, &accepted, "lqc_submitWorkV1Candidate", args); err != nil {
		return common.Hash{}, err
	}
	if accepted == (common.Hash{}) {
		return common.Hash{}, errors.New("node returned an empty candidate hash")
	}
	return accepted, nil
}

func wait(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func run(ctx context.Context, opts options) error {
	key, err := loadKey(opts.keyFile, opts.passwordFile)
	if err != nil {
		return err
	}
	defer zeroPrivateKey(key)

	client, err := rpc.DialContext(ctx, opts.rpcURL)
	if err != nil {
		return fmt.Errorf("connect RPC: %w", err)
	}
	defer client.Close()

	chainID, err := fetchChainID(ctx, client)
	if err != nil {
		return fmt.Errorf("validate network: %w", err)
	}

	hasher, err := rabbitx.NewLightHasher()
	if err != nil {
		return fmt.Errorf("start RandomX: %w", err)
	}
	defer hasher.Close()

	participant := key.Address
	nonce := opts.startNonce
	var activeEpoch uint64
	var acceptedInEpoch uint64
	var attempts uint64

	fmt.Printf("Rabbit Miner Work V1\nparticipant=%s chainId=%s rpc=%s\n", participant, chainID, opts.rpcURL)

	for ctx.Err() == nil {
		work, err := fetchWorkContext(ctx, client)
		if err != nil {
			fmt.Printf("waiting_for_work_context error=%q\n", err)
			if !wait(ctx, pollInterval) {
				break
			}
			continue
		}

		epoch := uint64(work.Epoch)
		if epoch != activeEpoch {
			activeEpoch = epoch
			acceptedInEpoch = 0
			attempts = 0
			fmt.Printf("epoch=%d difficulty=%s dataset=%s challenge=%s\n",
				epoch, (*big.Int)(work.Difficulty), work.DatasetAnchor, work.ChallengeAnchor)
		}

		if opts.ticketsPerEpoch != 0 && acceptedInEpoch >= opts.ticketsPerEpoch {
			if !wait(ctx, pollInterval) {
				break
			}
			continue
		}

		ticket := lqc.RandomXWorkTicketV1{
			Version:     lqc.RandomXWorkProtocolVersion,
			Epoch:       epoch,
			Participant: participant,
			Nonce:       nonce,
		}
		nonce++

		epochKey, err := lqc.RandomXWorkEpochKeyV1(chainID, epoch, work.DatasetAnchor)
		if err != nil {
			return err
		}
		input, err := lqc.RandomXWorkInputV1(chainID, work.ChallengeAnchor, ticket)
		if err != nil {
			return err
		}
		proofHash, err := hasher.Hash(epochKey, input)
		if err != nil {
			return fmt.Errorf("RandomX: %w", err)
		}
		attempts++

		meets, err := lqc.RandomXWorkHashMeetsTargetV1(proofHash, (*big.Int)(work.Difficulty))
		if err != nil {
			return err
		}
		if !meets {
			if opts.statusEvery != 0 && attempts%opts.statusEvery == 0 {
				fmt.Printf("participant=%s epoch=%d attempts=%d nonce=%d\n", participant, epoch, attempts, ticket.Nonce)
			}
			continue
		}

		signingHash, err := lqc.RandomXWorkSigningHashV1(chainID, work.ChallengeAnchor, ticket, proofHash)
		if err != nil {
			return err
		}
		signature, err := crypto.Sign(signingHash[:], key.PrivateKey)
		if err != nil {
			return fmt.Errorf("sign successful work: %w", err)
		}

		accepted, err := submitCandidate(ctx, client, candidateArgs{
			Version:     hexutil.Uint64(ticket.Version),
			Epoch:       hexutil.Uint64(ticket.Epoch),
			Participant: ticket.Participant,
			Nonce:       hexutil.Uint64(ticket.Nonce),
			ProofHash:   proofHash,
			Signature:   signature,
		})
		if err != nil {
			fmt.Printf("candidate_rejected epoch=%d nonce=%d error=%q\n", epoch, ticket.Nonce, err)
			if !wait(ctx, pollInterval) {
				break
			}
			continue
		}

		acceptedInEpoch++
		fmt.Printf("ticket_accepted epoch=%d nonce=%d proof=%s candidate=%s tickets_in_epoch=%d\n",
			epoch, ticket.Nonce, proofHash, accepted, acceptedInEpoch)
		if opts.once {
			return nil
		}
	}
	return ctx.Err()
}

func main() {
	opts := parseOptions()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, opts); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
		os.Exit(1)
	}
}
