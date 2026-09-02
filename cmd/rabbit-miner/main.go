// Copyright 2026 The Rabbit Chain Authors
// This file is part of the Rabbit Chain library.

// rabbit-miner performs one-time Rabbit Work V2 admission locally. Once the
// wallet owns a persistent seat, mining stops and the node participates with
// exactly the same consensus weight as every other seated wallet. The private
// key never leaves this process and is never sent to the JSON-RPC endpoint.
package main

import (
	"context"
	"encoding/json"
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

type participantStatus struct {
	Participant    common.Address `json:"participant"`
	SelectionEpoch hexutil.Uint64 `json:"selectionEpoch"`
	SeatCount      hexutil.Uint64 `json:"seatCount"`
	ActiveSeat     bool           `json:"activeSeat"`
	Committed      bool           `json:"committed"`
	LocalPool      bool           `json:"localPool"`
}

type observedBlock struct {
	Number hexutil.Uint64 `json:"number"`
	Miner  common.Address `json:"miner"`
}

type networkTelemetry struct {
	Height         uint64
	Peers          uint64
	Synced         bool
	Balance        *big.Int
	LatestProducer common.Address
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
	flag.Uint64Var(&opts.statusEvery, "status-every", 1000, "print progress after this many attempts (0 disables)")
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

func fetchParticipantStatus(
	ctx context.Context,
	client *rpc.Client,
	participant common.Address,
) (participantStatus, error) {
	var result participantStatus
	if err := client.CallContext(
		ctx,
		&result,
		"lqc_workV2ParticipantStatus",
		participant,
	); err != nil {
		return participantStatus{}, err
	}
	if result.Participant != participant {
		return participantStatus{}, errors.New("node returned status for another participant")
	}
	return result, nil
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

func fetchNetworkTelemetry(
	ctx context.Context,
	client *rpc.Client,
	participant common.Address,
) (networkTelemetry, error) {
	var height hexutil.Uint64
	if err := client.CallContext(ctx, &height, "eth_blockNumber"); err != nil {
		return networkTelemetry{}, err
	}
	var peers hexutil.Uint64
	if err := client.CallContext(ctx, &peers, "net_peerCount"); err != nil {
		return networkTelemetry{}, err
	}
	var syncing json.RawMessage
	if err := client.CallContext(ctx, &syncing, "eth_syncing"); err != nil {
		return networkTelemetry{}, err
	}
	var balance hexutil.Big
	if err := client.CallContext(
		ctx,
		&balance,
		"eth_getBalance",
		participant,
		"latest",
	); err != nil {
		return networkTelemetry{}, err
	}
	var latest observedBlock
	if err := client.CallContext(
		ctx,
		&latest,
		"eth_getBlockByNumber",
		"latest",
		false,
	); err != nil {
		return networkTelemetry{}, err
	}
	return networkTelemetry{
		Height:         uint64(height),
		Peers:          uint64(peers),
		Synced:         string(syncing) == "false",
		Balance:        new(big.Int).Set((*big.Int)(&balance)),
		LatestProducer: latest.Miner,
	}, nil
}

func fetchObservedBlock(
	ctx context.Context,
	client *rpc.Client,
	number uint64,
) (observedBlock, error) {
	var block observedBlock
	err := client.CallContext(
		ctx,
		&block,
		"eth_getBlockByNumber",
		hexutil.EncodeUint64(number),
		false,
	)
	return block, err
}

func formatRAB(wei *big.Int) string {
	if wei == nil {
		return "0"
	}
	base := new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
	whole, fraction := new(big.Int), new(big.Int)
	whole.QuoRem(new(big.Int).Set(wei), base, fraction)
	if fraction.Sign() == 0 {
		return whole.String()
	}
	digits := fraction.String()
	digits = strings.Repeat("0", 18-len(digits)) + digits
	digits = strings.TrimRight(digits, "0")
	return whole.String() + "." + digits
}

func monitorNetwork(
	ctx context.Context,
	client *rpc.Client,
	participant common.Address,
) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	var lastHeight uint64
	var produced uint64
	for {
		telemetry, err := fetchNetworkTelemetry(ctx, client, participant)
		if err != nil {
			fmt.Printf("NETWORK_STATUS unavailable error=%q\n", err)
		} else {
			if lastHeight != 0 && telemetry.Height > lastHeight {
				start := lastHeight + 1
				if telemetry.Height-start > 255 {
					start = telemetry.Height - 255
				}
				for number := start; number <= telemetry.Height; number++ {
					block, blockErr := fetchObservedBlock(ctx, client, number)
					if blockErr == nil && block.Miner == participant {
						produced++
					}
				}
			}
			lastHeight = telemetry.Height
			fmt.Printf("NETWORK_STATUS height=%d peers=%d synced=%t balance=%s_RAB latest_producer=%s blocks_produced_session=%d\n",
				telemetry.Height,
				telemetry.Peers,
				telemetry.Synced,
				formatRAB(telemetry.Balance),
				telemetry.LatestProducer,
				produced,
			)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
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

	participant := key.Address
	go monitorNetwork(ctx, client, participant)

	var hasher *rabbitx.FullHasher
	defer func() {
		if hasher != nil {
			hasher.Close()
		}
	}()
	nonce := opts.startNonce
	var activeEpoch uint64
	var acceptedInEpoch uint64
	var attempts uint64
	var lastWaitingMessage time.Time
	var waitingReason string
	var statusStarted = time.Now()
	var statusAttempts uint64

	fmt.Printf("Rabbit Miner Work V2\nparticipant=%s chainId=%s rpc=%s\n", participant, chainID, opts.rpcURL)

	for ctx.Err() == nil {
		status, statusErr := fetchParticipantStatus(
			ctx,
			client,
			participant,
		)
		if statusErr != nil {
			reason := statusErr.Error()
			if reason != waitingReason || time.Since(lastWaitingMessage) >= time.Minute {
				fmt.Printf("Waiting for canonical Work V2 status. Details: %s\n", reason)
				waitingReason = reason
				lastWaitingMessage = time.Now()
			}
			if !wait(ctx, pollInterval) {
				break
			}
			continue
		}
		if status.ActiveSeat {
			if waitingReason != "active-seat" || time.Since(lastWaitingMessage) >= time.Minute {
				fmt.Printf("ACTIVE_SEAT wallet=%s selection_epoch=%d total_seats=%d. RandomX mining is no longer needed; this wallet now has one equal consensus seat.\n",
					participant, status.SelectionEpoch, status.SeatCount)
				waitingReason = "active-seat"
				lastWaitingMessage = time.Now()
			}
			if !wait(ctx, pollInterval) {
				break
			}
			continue
		}
		if status.Committed || status.LocalPool {
			state := "accepted_by_local_relay"
			if status.Committed {
				state = "canonical_waiting_for_activation"
			}
			if state != waitingReason || time.Since(lastWaitingMessage) >= time.Minute {
				fmt.Printf("ADMISSION_PENDING wallet=%s state=%s selection_epoch=%d total_seats=%d. No duplicate mining is needed.\n",
					participant, state, status.SelectionEpoch, status.SeatCount)
				waitingReason = state
				lastWaitingMessage = time.Now()
			}
			if !wait(ctx, pollInterval) {
				break
			}
			continue
		}

		work, err := fetchWorkContext(ctx, client)
		if err != nil {
			reason := err.Error()
			if reason != waitingReason || time.Since(lastWaitingMessage) >= time.Minute {
				fmt.Printf("Waiting for the next LCQ work window. Mining will resume automatically. Details: %s\n", reason)
				waitingReason = reason
				lastWaitingMessage = time.Now()
			}
			if !wait(ctx, pollInterval) {
				break
			}
			continue
		}
		waitingReason = ""
		if hasher == nil {
			hasher, err = rabbitx.NewFullHasher()
			if err != nil {
				return fmt.Errorf("start RandomX full-memory miner: %w", err)
			}
		}

		epoch := uint64(work.Epoch)
		if epoch != activeEpoch {
			activeEpoch = epoch
			acceptedInEpoch = 0
			attempts = 0
			statusAttempts = 0
			statusStarted = time.Now()
			fmt.Printf("epoch=%d difficulty=%s dataset=%s challenge=%s\n",
				epoch, (*big.Int)(work.Difficulty), work.DatasetAnchor, work.ChallengeAnchor)
			fmt.Println("Preparing the Rabbit RandomX 1 GiB dataset. This happens once per epoch; mining starts automatically when ready.")
		}

		if opts.ticketsPerEpoch != 0 && acceptedInEpoch >= opts.ticketsPerEpoch {
			if time.Since(lastWaitingMessage) >= time.Minute {
				fmt.Printf("Ticket already accepted for epoch %d. Waiting for LCQ selection or the next epoch.\n", epoch)
				lastWaitingMessage = time.Now()
			}
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
		firstAttempt := attempts == 0
		proofHash, err := hasher.Hash(epochKey, input)
		if err != nil {
			return fmt.Errorf("RandomX full-memory mining: %w", err)
		}
		attempts++
		if firstAttempt {
			fmt.Println("Rabbit RandomX dataset ready. Searching for this wallet's one-time Work V2 admission proof.")
		}

		meets, err := lqc.RandomXWorkHashMeetsTargetV1(proofHash, (*big.Int)(work.Difficulty))
		if err != nil {
			return err
		}
		if !meets {
			if opts.statusEvery != 0 && attempts%opts.statusEvery == 0 {
				now := time.Now()
				seconds := now.Sub(statusStarted).Seconds()
				rate := float64(attempts-statusAttempts) / seconds
				fmt.Printf("Mining Work V2 admission: wallet=%s epoch=%d attempts=%d rate=%.1f H/s\n",
					participant, epoch, attempts, rate)
				statusStarted = now
				statusAttempts = attempts
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
		fmt.Printf("ADMISSION_ACCEPTED_LOCAL epoch=%d nonce=%d proof=%s candidate=%s. Waiting for propagation and canonical activation.\n",
			epoch, ticket.Nonce, proofHash, accepted)
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
