// Copyright 2026 The Rabbit Chain Authors
// This file is part of the Rabbit Chain library.

// rabbit-miner performs one-time Rabbit Work V2 admission locally. Once the
// wallet owns a persistent seat, mining stops and the node participates with
// exactly the same consensus weight as every other seated wallet. The private
// key never leaves this process and is never sent to the JSON-RPC endpoint.
package main

import (
	"bytes"
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
	"sync/atomic"
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
	officialChainID        = uint64(9280)
	pollInterval           = 3 * time.Second
	headFreshness          = 10 * time.Minute
	headFutureTolerance    = 30 * time.Second
	syncDiscoveryGrace     = 20 * time.Second
	offlineReadinessGrace  = 60 * time.Second
	syncStatusMessageEvery = 15 * time.Second
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
	Number    hexutil.Uint64 `json:"number"`
	Timestamp hexutil.Uint64 `json:"timestamp"`
	Miner     common.Address `json:"miner"`
}

type rpcSyncProgress struct {
	CurrentBlock hexutil.Uint64 `json:"currentBlock"`
	HighestBlock hexutil.Uint64 `json:"highestBlock"`
}

type networkTelemetry struct {
	Height         uint64
	Peers          uint64
	Syncing        bool
	SyncCurrent    uint64
	SyncHighest    uint64
	HeadFresh      bool
	Balance        *big.Int
	LatestProducer common.Address
}

type readinessTracker struct {
	startedAt     time.Time
	stableSince   time.Time
	lastHeight    uint64
	sawSync       bool
	wasSyncing    bool
	highestTarget uint64
	syncCompleted bool
}

type options struct {
	rpcURL          string
	keyFile         string
	passwordFile    string
	startNonce      uint64
	ticketsPerEpoch uint64
	statusEvery     uint64
	once            bool
	verbose         bool
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
	flag.BoolVar(&opts.verbose, "verbose", false, "show technical Work V2 and RPC details")
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

func observedHeadFresh(block observedBlock, height uint64, now time.Time) bool {
	if height == 0 || uint64(block.Number) != height || uint64(block.Timestamp) == 0 {
		return false
	}
	age := now.Sub(time.Unix(int64(block.Timestamp), 0))
	return age >= -headFutureTolerance && age <= headFreshness
}

func newReadinessTracker(now time.Time) *readinessTracker {
	return &readinessTracker{
		startedAt:   now,
		stableSince: now,
	}
}

func (r *readinessTracker) observe(telemetry networkTelemetry, now time.Time) (bool, string) {
	if telemetry.Height != r.lastHeight {
		r.lastHeight = telemetry.Height
		r.stableSince = now
	}
	if telemetry.Syncing {
		r.sawSync = true
		r.syncCompleted = false
		if !r.wasSyncing {
			r.highestTarget = telemetry.SyncHighest
		} else if telemetry.SyncHighest > r.highestTarget {
			r.highestTarget = telemetry.SyncHighest
		}
		r.wasSyncing = true
		if telemetry.SyncHighest > 0 {
			return false, fmt.Sprintf("syncing blockchain %d/%d", telemetry.SyncCurrent, telemetry.SyncHighest)
		}
		return false, "blockchain synchronization is active"
	}
	r.wasSyncing = false

	if r.sawSync && !r.syncCompleted {
		if r.highestTarget > 0 && telemetry.Height >= r.highestTarget {
			r.syncCompleted = true
			return true, "downloaded canonical sync target"
		}
		if r.highestTarget > 0 {
			return false, fmt.Sprintf("waiting for canonical sync target %d (local %d)", r.highestTarget, telemetry.Height)
		}
		return false, "waiting for successful blockchain synchronization"
	}
	if r.syncCompleted {
		return true, "canonical synchronization completed"
	}

	elapsed := now.Sub(r.startedAt)
	if telemetry.HeadFresh && telemetry.Peers > 0 && elapsed >= syncDiscoveryGrace {
		return true, "live head confirmed after peer discovery grace"
	}
	if telemetry.Height > 0 && !r.sawSync && now.Sub(r.stableSince) >= offlineReadinessGrace {
		return true, "offline recovery grace satisfied"
	}
	if telemetry.Peers == 0 {
		return false, "waiting for Rabbit peers and canonical chain discovery"
	}
	if telemetry.HeadFresh {
		return false, "confirming live head before mining activation"
	}
	return false, "waiting for canonical blockchain synchronization"
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
	var syncProgress rpcSyncProgress
	syncingActive := !bytes.Equal(bytes.TrimSpace(syncing), []byte("false"))
	if syncingActive {
		if err := json.Unmarshal(syncing, &syncProgress); err != nil {
			return networkTelemetry{}, fmt.Errorf("decode eth_syncing: %w", err)
		}
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
		Syncing:        syncingActive,
		SyncCurrent:    uint64(syncProgress.CurrentBlock),
		SyncHighest:    uint64(syncProgress.HighestBlock),
		HeadFresh:      observedHeadFresh(latest, uint64(height), time.Now()),
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

func fetchBalanceAt(
	ctx context.Context,
	client *rpc.Client,
	participant common.Address,
	number uint64,
) (*big.Int, error) {
	var balance hexutil.Big
	if err := client.CallContext(
		ctx,
		&balance,
		"eth_getBalance",
		participant,
		hexutil.EncodeUint64(number),
	); err != nil {
		return nil, err
	}
	return new(big.Int).Set((*big.Int)(&balance)), nil
}

func positiveBalanceDelta(current, previous *big.Int) *big.Int {
	if current == nil || previous == nil || current.Cmp(previous) <= 0 {
		return new(big.Int)
	}
	return new(big.Int).Sub(new(big.Int).Set(current), previous)
}

func shortAddress(address common.Address) string {
	text := address.Hex()
	if len(text) <= 14 {
		return text
	}
	return text[:8] + "..." + text[len(text)-4:]
}

func syncPercent(current, highest uint64) float64 {
	if highest == 0 {
		return 0
	}
	if current > highest {
		current = highest
	}
	return float64(current) * 100 / float64(highest)
}

func printMinerHeader(participant common.Address, chainID *big.Int, rpcURL string) {
	fmt.Println()
	fmt.Println("============================================================")
	fmt.Println(" Rabbit Miner - Work V2 Admission")
	fmt.Println(" Rabbit Chain Testnet")
	fmt.Println("============================================================")
	fmt.Printf(" Wallet : %s\n", participant)
	fmt.Printf(" Network: Rabbit Chain Testnet (Chain ID %s)\n", chainID)
	fmt.Printf(" Node   : %s\n", rpcURL)
	fmt.Println()
	fmt.Println(" The miner will wait for the blockchain to synchronize.")
	fmt.Println(" Work V2 starts automatically only when the node is ready.")
	fmt.Println(" After your admission proof becomes active, keep Rabbit Core")
	fmt.Println(" running: LCQ will produce blocks automatically when selected.")
	fmt.Println("============================================================")
	fmt.Println()
}

func monitorNetwork(
	ctx context.Context,
	client *rpc.Client,
	participant common.Address,
	ready *atomic.Bool,
	activeSeat *atomic.Bool,
	verbose bool,
) {
	ticker := time.NewTicker(pollInterval)
	var displayChainID hexutil.Uint64
	unit := "RAB"
	if err := client.CallContext(ctx, &displayChainID, "eth_chainId"); err == nil && uint64(displayChainID) == 9280 {
		unit = "tRAB"
	}
	defer ticker.Stop()

	var (
		lastHeight       uint64
		lastBalance      *big.Int
		trackingLive     bool
		produced         uint64
		committeeRewards uint64
		lastStatus       time.Time
		lastSyncStatus   time.Time
	)

	for {
		telemetry, err := fetchNetworkTelemetry(ctx, client, participant)
		if err != nil {
			if time.Since(lastStatus) >= syncStatusMessageEvery {
				fmt.Println("STATUS | Waiting for node/network data | Mining: PAUSED")
				lastStatus = time.Now()
			}
			if verbose {
				fmt.Printf("NETWORK_STATUS unavailable error=%q\n", err)
			}
		} else if telemetry.Syncing && telemetry.SyncHighest > 0 {
			trackingLive = false
			lastHeight = telemetry.Height
			lastBalance = nil
			if time.Since(lastSyncStatus) >= syncStatusMessageEvery {
				fmt.Printf("SYNC   | %d / %d blocks | %.1f%% | Peers: %d | Mining: WAITING\n",
					telemetry.SyncCurrent,
					telemetry.SyncHighest,
					syncPercent(telemetry.SyncCurrent, telemetry.SyncHighest),
					telemetry.Peers,
				)
				lastSyncStatus = time.Now()
			}
		} else if ready.Load() {
			if !trackingLive || telemetry.Height < lastHeight {
				trackingLive = true
				lastHeight = telemetry.Height
				lastBalance = new(big.Int).Set(telemetry.Balance)
			} else if telemetry.Height > lastHeight {
				if telemetry.Height-lastHeight > 128 {
					fmt.Printf("CATCHUP| Rejoined live chain at block %d | Skipped %d historical terminal lines\n",
						telemetry.Height,
						telemetry.Height-lastHeight,
					)
					lastHeight = telemetry.Height
					lastBalance = new(big.Int).Set(telemetry.Balance)
				} else {
					if lastBalance == nil {
						balance, balanceErr := fetchBalanceAt(ctx, client, participant, lastHeight)
						if balanceErr == nil {
							lastBalance = balance
						}
					}

					for number := lastHeight + 1; number <= telemetry.Height; number++ {
						block, blockErr := fetchObservedBlock(ctx, client, number)
						if blockErr != nil {
							if verbose {
								fmt.Printf("BLOCK_ACTIVITY number=%d error=%q\n", number, blockErr)
							}
							continue
						}
						balance, balanceErr := fetchBalanceAt(ctx, client, participant, number)
						if balanceErr != nil {
							if verbose {
								fmt.Printf("BLOCK_BALANCE number=%d error=%q\n", number, balanceErr)
							}
							continue
						}
						delta := positiveBalanceDelta(balance, lastBalance)

						switch {
						case block.Miner == participant:
							produced++
							fmt.Printf("BLOCK  | #%d | 🐇 PRODUCER 🟢  | +%s %s | Balance: %s %s\n",
								number,
								formatRAB(delta),
								unit,
								formatRAB(balance),
								unit,
							)
						case delta.Sign() > 0 && activeSeat != nil && activeSeat.Load():
							committeeRewards++
							fmt.Printf("BLOCK  | #%d | 🥕 COMMITTEE 🟠 | +%s %s | Balance: %s %s\n",
								number,
								formatRAB(delta),
								unit,
								formatRAB(balance),
								unit,
							)
						case delta.Sign() > 0:
							fmt.Printf("BLOCK  | #%d | CREDIT | +%s %s | Balance: %s %s\n",
								number,
								formatRAB(delta),
								unit,
								formatRAB(balance),
								unit,
							)
						default:
							fmt.Printf("BLOCK  | #%d | Balance: %s %s\n",
								number,
								formatRAB(balance),
								unit,
							)
						}
						lastBalance = balance
					}
					lastHeight = telemetry.Height
				}
			}

			if time.Since(lastStatus) >= 30*time.Second {
				state := "LCQ PENDING"
				if activeSeat != nil && activeSeat.Load() {
					state = "LCQ ACTIVE"
				}
				fmt.Printf("STATUS | %s | Block: %d | Peers: %d | Balance: %s %s | Produced: %d | Committee: %d\n",
					state,
					telemetry.Height,
					telemetry.Peers,
					formatRAB(telemetry.Balance),
					unit,
					produced,
					committeeRewards,
				)
				lastStatus = time.Now()
			}
		} else {
			trackingLive = false
			lastHeight = telemetry.Height
			lastBalance = nil
			if time.Since(lastStatus) >= 30*time.Second {
				fmt.Printf("STATUS | Checking canonical chain | Block: %d | Peers: %d | Mining: WAITING\n",
					telemetry.Height,
					telemetry.Peers,
				)
				lastStatus = time.Now()
			}
		}

		if verbose && err == nil {
			fmt.Printf("NETWORK_STATUS height=%d peers=%d synced=%t syncing=%t target=%d balance=%s_%s latest_producer=%s blocks_produced_session=%d committee_rewards_session=%d active_seat=%t\n",
				telemetry.Height,
				telemetry.Peers,
				ready.Load(),
				telemetry.Syncing,
				telemetry.SyncHighest,
				formatRAB(telemetry.Balance),
				unit,
				telemetry.LatestProducer,
				produced,
				committeeRewards,
				activeSeat != nil && activeSeat.Load(),
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
	var miningReady atomic.Bool
	var activeSeat atomic.Bool
	go monitorNetwork(ctx, client, participant, &miningReady, &activeSeat, opts.verbose)
	readiness := newReadinessTracker(time.Now())
	var lastReadinessCheck time.Time
	var readinessOK bool

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
	var seatWasActive bool
	var lastSyncStatusMessage time.Time
	var statusStarted = time.Now()
	var statusAttempts uint64

	printMinerHeader(participant, chainID, opts.rpcURL)

	for ctx.Err() == nil {
		now := time.Now()
		if lastReadinessCheck.IsZero() || now.Sub(lastReadinessCheck) >= pollInterval {
			telemetry, telemetryErr := fetchNetworkTelemetry(ctx, client, participant)
			lastReadinessCheck = now
			if telemetryErr != nil {
				readinessOK = false
				miningReady.Store(false)
				reason := telemetryErr.Error()
				if time.Since(lastSyncStatusMessage) >= syncStatusMessageEvery {
					fmt.Println("SYNC   | Waiting for Rabbit Core network data. Mining remains paused.")
					if opts.verbose {
						fmt.Printf("SYNC_WAIT status=unavailable reason=%q\n", reason)
					}
					lastSyncStatusMessage = time.Now()
				}
			} else {
				var reason string
				readinessOK, reason = readiness.observe(telemetry, now)
				miningReady.Store(readinessOK)
				if !readinessOK && time.Since(lastSyncStatusMessage) >= syncStatusMessageEvery {
					if telemetry.Syncing && telemetry.SyncHighest > 0 {
						fmt.Printf("SYNC   | %d / %d blocks | %.1f%% | Peers: %d | Mining: WAITING\n",
							telemetry.SyncCurrent, telemetry.SyncHighest, syncPercent(telemetry.SyncCurrent, telemetry.SyncHighest), telemetry.Peers)
					} else {
						fmt.Printf("SYNC   | Block: %d | Peers: %d | Confirming network readiness | Mining: WAITING\n", telemetry.Height, telemetry.Peers)
					}
					if opts.verbose {
						fmt.Printf("SYNC_WAIT height=%d peers=%d syncing=%t target=%d reason=%q\n",
							telemetry.Height, telemetry.Peers, telemetry.Syncing, telemetry.SyncHighest, reason)
					}
					lastSyncStatusMessage = time.Now()
				}
			}
		}
		if !readinessOK {
			if !wait(ctx, pollInterval) {
				break
			}
			continue
		}

		status, statusErr := fetchParticipantStatus(
			ctx,
			client,
			participant,
		)
		if statusErr != nil {
			reason := statusErr.Error()
			if reason != waitingReason || time.Since(lastWaitingMessage) >= time.Minute {
				fmt.Println("WAIT   | Waiting for canonical Work V2 status. Mining will continue automatically when ready.")
				if opts.verbose {
					fmt.Printf("       | Details: %s\n", reason)
				}
				waitingReason = reason
				lastWaitingMessage = time.Now()
			}
			if !wait(ctx, pollInterval) {
				break
			}
			continue
		}
		activeSeat.Store(status.ActiveSeat)
		if status.ActiveSeat {
			if !seatWasActive {
				fmt.Println()
				fmt.Println("READY  | Your wallet has an ACTIVE LCQ seat.")
				fmt.Printf("       | Selection epoch: %d | Total seats: %d\n", status.SelectionEpoch, status.SeatCount)
				fmt.Println("       | Work V2 admission mining is complete.")
				fmt.Println("       | Keep Rabbit Core running. Block production is automatic when your seat is selected.")
				if opts.verbose {
					fmt.Printf("ACTIVE_SEAT wallet=%s selection_epoch=%d total_seats=%d\n",
						participant, status.SelectionEpoch, status.SeatCount)
				}
				waitingReason = "active-seat"
				lastWaitingMessage = time.Now()
				seatWasActive = true
			}
			if !wait(ctx, pollInterval) {
				break
			}
			continue
		}
		seatWasActive = false
		if status.Committed || status.LocalPool {
			state := "accepted_by_local_relay"
			if status.Committed {
				state = "canonical_waiting_for_activation"
			}
			if state != waitingReason || time.Since(lastWaitingMessage) >= time.Minute {
				fmt.Printf("WAIT   | Admission proof accepted | Activation pending | Selection epoch: %d | Seats: %d\n",
					status.SelectionEpoch, status.SeatCount)
				fmt.Println("       | No duplicate mining is needed. Keep Rabbit Core running.")
				if opts.verbose {
					fmt.Printf("ADMISSION_PENDING wallet=%s state=%s selection_epoch=%d total_seats=%d\n",
						participant, state, status.SelectionEpoch, status.SeatCount)
				}
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
				fmt.Println("WAIT   | Waiting for the next LCQ Work V2 window. It will start automatically.")
				if opts.verbose {
					fmt.Printf("       | Details: %s\n", reason)
				}
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
			fmt.Printf("WORK   | Admission epoch %d opened. Preparing RandomX 1 GiB dataset...\n", epoch)
			fmt.Println("       | This preparation happens once per epoch. Mining starts automatically.")
			if opts.verbose {
				fmt.Printf("WORK_CONTEXT epoch=%d difficulty=%s dataset=%s challenge=%s\n",
					epoch, (*big.Int)(work.Difficulty), work.DatasetAnchor, work.ChallengeAnchor)
			}
		}

		if opts.ticketsPerEpoch != 0 && acceptedInEpoch >= opts.ticketsPerEpoch {
			if time.Since(lastWaitingMessage) >= time.Minute {
				fmt.Printf("WAIT   | Admission proof already accepted for epoch %d. Waiting for LCQ activation.\n", epoch)
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
			fmt.Println("MINE   | RandomX dataset ready. Searching for this wallet's Work V2 admission proof.")
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
				fmt.Printf("MINE   | Epoch: %d | Attempts: %d | Speed: %.1f H/s\n",
					epoch, attempts, rate)
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
			fmt.Println("RETRY  | Candidate was not accepted. Retrying automatically.")
			if opts.verbose {
				fmt.Printf("candidate_rejected epoch=%d nonce=%d error=%q\n", epoch, ticket.Nonce, err)
			}
			if !wait(ctx, pollInterval) {
				break
			}
			continue
		}

		acceptedInEpoch++
		fmt.Println()
		fmt.Println("SUCCESS| Work V2 admission proof accepted by your local Rabbit node.")
		fmt.Println("       | Waiting for canonical propagation and LCQ activation.")
		fmt.Println("       | No duplicate mining is needed. Keep Rabbit Core running.")
		if opts.verbose {
			fmt.Printf("ADMISSION_ACCEPTED_LOCAL epoch=%d nonce=%d proof=%s candidate=%s\n",
				epoch, ticket.Nonce, proofHash, accepted)
		}
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
