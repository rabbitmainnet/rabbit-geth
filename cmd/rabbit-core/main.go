// Copyright 2026 The Rabbit Chain Authors
// Rabbit Core is the user-facing launcher for the Rabbit node and Work V2 miner.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/console/prompt"
)

const (
	officialGenesisSHA256 = "e2e5494542e37689cb6e385456d6df239e478c1d12e9c3a1cc270e69c6b51686"
	officialChainID       = "0x2440"
	officialNetworkID     = "9280"
)

type options struct {
	checkOnly bool
	dataDir   string
	node      string
	miner     string
	genesis   string
	bootnodes string
	rpcPort   uint
	p2pPort   uint
	authPort  uint
}

type keyJSON struct {
	Address string `json:"address"`
}

type rpcResponse struct {
	Result string `json:"result"`
	Error  any    `json:"error"`
}

func adjacent(name string) string {
	executable, err := os.Executable()
	if err != nil {
		return name
	}
	if runtime.GOOS == "windows" && filepath.Ext(name) == "" {
		name += ".exe"
	}
	return filepath.Join(filepath.Dir(executable), name)
}

func defaultDataDir() string {
	root, err := os.UserConfigDir()
	if err != nil {
		root, _ = os.UserHomeDir()
	}
	return filepath.Join(root, "RabbitChain", "TestnetV2")
}

func parseOptions() options {
	var opts options
	flag.BoolVar(&opts.checkOnly, "check", false, "validate the package without starting a node or miner")
	flag.StringVar(&opts.dataDir, "data-dir", defaultDataDir(), "Rabbit Core data directory")
	flag.StringVar(&opts.node, "node", adjacent("rabbit-node"), "Rabbit node executable")
	flag.StringVar(&opts.miner, "miner", adjacent("rabbit-miner"), "Rabbit Work V2 miner executable")
	flag.StringVar(&opts.genesis, "genesis", adjacent("genesis.json"), "official Rabbit Testnet genesis")
	flag.StringVar(&opts.bootnodes, "bootnodes", "", "comma-separated official bootnode enode URLs")
	flag.UintVar(&opts.rpcPort, "rpc-port", 8545, "local private JSON-RPC port")
	flag.UintVar(&opts.p2pPort, "p2p-port", 30303, "Rabbit P2P port")
	flag.UintVar(&opts.authPort, "auth-port", 8551, "local authenticated RPC port")
	flag.Parse()
	return opts
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func validatePackage(opts options) error {
	for name, path := range map[string]string{
		"Rabbit node":  opts.node,
		"Rabbit miner": opts.miner,
		"genesis":      opts.genesis,
	} {
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("%s not found at %s: %w", name, path, err)
		}
		if info.IsDir() {
			return fmt.Errorf("%s is a directory: %s", name, path)
		}
	}
	hash, err := fileSHA256(opts.genesis)
	if err != nil {
		return fmt.Errorf("hash genesis: %w", err)
	}
	if hash != officialGenesisSHA256 {
		return fmt.Errorf("wrong genesis SHA-256 %s", hash)
	}
	return nil
}

func keyFiles(dataDir string) ([]string, error) {
	dir := filepath.Join(dataDir, "keystore")
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var files []string
	for _, entry := range entries {
		if !entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
			files = append(files, filepath.Join(dir, entry.Name()))
		}
	}
	sort.Strings(files)
	return files, nil
}

func keyAddress(path string) (common.Address, error) {
	encoded, err := os.ReadFile(path)
	if err != nil {
		return common.Address{}, err
	}
	var key keyJSON
	if err := json.Unmarshal(encoded, &key); err != nil {
		return common.Address{}, err
	}
	if !common.IsHexAddress(key.Address) {
		return common.Address{}, errors.New("keystore has an invalid address")
	}
	return common.HexToAddress(key.Address), nil
}

func password(confirm bool) (string, error) {
	value, err := prompt.Stdin.PromptPassword("Mining wallet password: ")
	if err != nil {
		return "", err
	}
	if value == "" {
		return "", errors.New("password cannot be empty")
	}
	if confirm {
		again, err := prompt.Stdin.PromptPassword("Repeat password: ")
		if err != nil {
			return "", err
		}
		if value != again {
			return "", errors.New("passwords do not match")
		}
	}
	return value, nil
}

func sessionPasswordFile(dataDir, password string) (string, func(), error) {
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return "", nil, err
	}
	file, err := os.CreateTemp(dataDir, ".rabbit-session-password-")
	if err != nil {
		return "", nil, err
	}
	name := file.Name()
	cleanup := func() { _ = os.Remove(name) }
	if err := file.Chmod(0600); err != nil {
		file.Close()
		cleanup()
		return "", nil, err
	}
	if _, err := file.WriteString(password + "\n"); err != nil {
		file.Close()
		cleanup()
		return "", nil, err
	}
	if err := file.Close(); err != nil {
		cleanup()
		return "", nil, err
	}
	return name, cleanup, nil
}

func runCommand(ctx context.Context, output io.Writer, executable string, args ...string) error {
	command := exec.CommandContext(ctx, executable, args...)
	command.Stdout = output
	command.Stderr = output
	command.Stdin = os.Stdin
	return command.Run()
}

func createWallet(ctx context.Context, opts options, passwordFile string) (string, common.Address, error) {
	var output bytes.Buffer
	err := runCommand(ctx, &output, opts.node,
		"account", "new", "--datadir", opts.dataDir, "--password", passwordFile,
	)
	if err != nil {
		return "", common.Address{}, fmt.Errorf("create mining wallet: %w (%s)", err, output.String())
	}
	files, err := keyFiles(opts.dataDir)
	if err != nil || len(files) != 1 {
		return "", common.Address{}, fmt.Errorf("locate created wallet: files=%d error=%v", len(files), err)
	}
	address, err := keyAddress(files[0])
	return files[0], address, err
}

func walletBackupMessage(keyFile string, address common.Address) string {
	return fmt.Sprintf(
		"Mining wallet created: %s\nEncrypted wallet backup file: %s\nIMPORTANT: copy this encrypted wallet file to a safe backup location before mining.",
		address.Hex(), keyFile,
	)
}

func showWalletInfo(opts options, keyFile string, address common.Address) {
	fmt.Printf("Wallet address: %s\n", address.Hex())
	fmt.Printf("Encrypted wallet file: %s\n", keyFile)
	fmt.Printf("Rabbit data directory: %s\n", opts.dataDir)
	fmt.Printf("Node log: %s\n", filepath.Join(opts.dataDir, "logs", "rabbit-node.log"))
	fmt.Println("Keep the encrypted wallet file and its password in separate safe locations.")
}

func prepareWallet(ctx context.Context, opts options) (string, common.Address, string, func(), error) {
	files, err := keyFiles(opts.dataDir)
	if err != nil {
		return "", common.Address{}, "", nil, err
	}
	if len(files) > 1 {
		return "", common.Address{}, "", nil, errors.New("multiple mining wallets found; keep one wallet in the Rabbit Core keystore")
	}
	creating := len(files) == 0
	if creating {
		fmt.Println("No mining wallet exists yet. Rabbit Core will create one encrypted wallet.")
	}
	secret, err := password(creating)
	if err != nil {
		return "", common.Address{}, "", nil, err
	}
	passwordFile, cleanup, err := sessionPasswordFile(opts.dataDir, secret)
	secret = ""
	if err != nil {
		return "", common.Address{}, "", nil, err
	}
	if creating {
		keyFile, address, err := createWallet(ctx, opts, passwordFile)
		if err != nil {
			cleanup()
			return "", common.Address{}, "", nil, err
		}
		fmt.Println(walletBackupMessage(keyFile, address))
		return keyFile, address, passwordFile, cleanup, nil
	}
	address, err := keyAddress(files[0])
	if err != nil {
		cleanup()
		return "", common.Address{}, "", nil, err
	}
	fmt.Println("Existing mining wallet found. Rabbit Core will reuse it.")
	return files[0], address, passwordFile, cleanup, nil
}

func initialize(ctx context.Context, opts options) error {
	chainData := filepath.Join(opts.dataDir, "rabbit", "chaindata")
	if _, err := os.Stat(chainData); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	fmt.Println("Initializing the official Rabbit Testnet genesis...")
	return runCommand(ctx, os.Stdout, opts.node, "--datadir", opts.dataDir, "init", opts.genesis)
}

func configuredBootnodes(opts options) string {
	if opts.bootnodes != "" {
		return opts.bootnodes
	}
	if value := strings.TrimSpace(os.Getenv("RABBIT_BOOTNODES")); value != "" {
		return value
	}
	encoded, err := os.ReadFile(adjacent("bootnodes.txt"))
	if err == nil {
		return strings.TrimSpace(string(encoded))
	}
	return ""
}

func rpcCall(port uint, method string) (string, error) {
	payload := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":%q,"params":[]}`, method)
	client := &http.Client{Timeout: 5 * time.Second}
	response, err := client.Post(
		fmt.Sprintf("http://127.0.0.1:%d", port),
		"application/json", strings.NewReader(payload),
	)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	var decoded rpcResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		return "", err
	}
	if decoded.Error != nil {
		return "", fmt.Errorf("RPC error: %v", decoded.Error)
	}
	return decoded.Result, nil
}

func waitForNode(ctx context.Context, port uint, nodeDone <-chan error) error {
	for attempt := 0; attempt < 120; attempt++ {
		select {
		case err := <-nodeDone:
			return fmt.Errorf("Rabbit node exited during startup: %w", err)
		default:
		}
		chainID, err := rpcCall(port, "eth_chainId")
		if err == nil {
			if chainID != officialChainID {
				return fmt.Errorf("node chain ID is %s, expected %s", chainID, officialChainID)
			}
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return errors.New("Rabbit node did not become ready")
}

func waitForProcess(name string, command *exec.Cmd, done <-chan error) {
	select {
	case <-done:
		fmt.Printf("%s stopped safely.\n", name)
	case <-time.After(20 * time.Second):
		fmt.Printf("%s did not stop in time; forcing shutdown.\n", name)
		if command.Process != nil {
			_ = command.Process.Kill()
		}
		<-done
	}
}

func start(ctx context.Context, opts options, keyFile string, address common.Address, passwordFile, bootnodes string) error {
	logs := filepath.Join(opts.dataDir, "logs")
	if err := os.MkdirAll(logs, 0700); err != nil {
		return err
	}
	nodeLog, err := os.OpenFile(filepath.Join(logs, "rabbit-node.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	defer nodeLog.Close()

	nodeArgs := []string{
		"--datadir", opts.dataDir,
		"--networkid", officialNetworkID,
		"--syncmode", "full",
		"--port", fmt.Sprint(opts.p2pPort),
		"--bootnodes", bootnodes,
		"--maxpeers", "50",
		"--http", "--http.addr", "127.0.0.1",
		"--http.port", fmt.Sprint(opts.rpcPort),
		"--http.api", "eth,net,web3,lqc",
		"--http.vhosts", "localhost,127.0.0.1",
		"--authrpc.addr", "127.0.0.1",
		"--authrpc.port", fmt.Sprint(opts.authPort),
		"--ipcdisable",
		"--mine",
		"--miner.etherbase", address.Hex(),
		"--password", passwordFile,
		"--cache", "1024",
	}
	// Do not use CommandContext here. On Ctrl+C it kills the database process
	// immediately, racing the node's own graceful shutdown and risking damage.
	node := exec.Command(opts.node, nodeArgs...)
	node.Stdout, node.Stderr = nodeLog, nodeLog
	node.Env = append(os.Environ(), "RABBIT_LQC_COINBASE="+address.Hex())
	if err := node.Start(); err != nil {
		return fmt.Errorf("start Rabbit node: %w", err)
	}
	nodeDone := make(chan error, 1)
	go func() { nodeDone <- node.Wait() }()
	if err := waitForNode(ctx, opts.rpcPort, nodeDone); err != nil {
		if node.Process != nil {
			_ = node.Process.Kill()
		}
		return err
	}

	fmt.Printf("Rabbit node RPC ready. Wallet: %s\n", address)
	fmt.Println("Rabbit Core will verify canonical blockchain synchronization before Work V2 admission or LCQ production becomes active.")
	miner := exec.Command(opts.miner,
		"--rpc", fmt.Sprintf("http://127.0.0.1:%d", opts.rpcPort),
		"--keystore", keyFile,
		"--password-file", passwordFile,
		"--tickets-per-epoch", "1",
	)
	miner.Stdout, miner.Stderr = os.Stdout, os.Stderr
	if err := miner.Start(); err != nil {
		if node.Process != nil {
			_ = node.Process.Kill()
		}
		return fmt.Errorf("start Rabbit miner: %w", err)
	}
	minerDone := make(chan error, 1)
	go func() { minerDone <- miner.Wait() }()

	fmt.Println("Rabbit Core is running. Synchronization and mining activation are automatic. Press Ctrl+C to stop safely.")
	select {
	case <-ctx.Done():
		fmt.Println("Stopping Rabbit Miner and Rabbit Node safely. Please wait...")
		waitForProcess("Rabbit Miner", miner, minerDone)
		waitForProcess("Rabbit Node", node, nodeDone)
		fmt.Println("Rabbit Core stopped. Your wallet remains safely stored.")
		return nil
	case err := <-minerDone:
		if node.Process != nil {
			_ = node.Process.Kill()
		}
		return fmt.Errorf("Rabbit Miner stopped unexpectedly: %w", err)
	case err := <-nodeDone:
		if miner.Process != nil {
			_ = miner.Process.Kill()
		}
		return fmt.Errorf("Rabbit Node stopped unexpectedly: %w", err)
	}
}

func run(ctx context.Context, opts options) error {
	if err := validatePackage(opts); err != nil {
		return err
	}
	fmt.Println("Rabbit Core package: OK")
	fmt.Println("Rabbit Testnet chain ID: 9280")
	if opts.checkOnly {
		fmt.Println("Check completed. No node or miner was started.")
		return nil
	}
	bootnodes := configuredBootnodes(opts)
	if bootnodes == "" {
		return errors.New("official bootnodes are not configured yet; Rabbit Core will not start an isolated chain")
	}
	keyFile, address, passwordFile, cleanup, err := prepareWallet(ctx, opts)
	if err != nil {
		return err
	}
	defer cleanup()
	showWalletInfo(opts, keyFile, address)
	if err := initialize(ctx, opts); err != nil {
		return err
	}
	return start(ctx, opts, keyFile, address, passwordFile, bootnodes)
}

func main() {
	opts := parseOptions()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, opts); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintln(os.Stderr, "Rabbit Core error:", err)
		os.Exit(1)
	}
}
