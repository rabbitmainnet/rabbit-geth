package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
)

const auditVersion = "rabbit-tx-auditor/1.2.0"

type options struct {
	base          string
	nodes         int
	senderNode    int
	recipientNode int
	valueWei      string
	tipWei        string
	timeout       time.Duration
	jsonPath      string
	markdownPath  string
	verifyNodes   string
}

type balanceResult struct {
	Address       string `json:"address"`
	Role          string `json:"role"`
	BeforeWei     string `json:"beforeWei"`
	AfterWei      string `json:"afterWei"`
	ExpectedDelta string `json:"expectedDeltaWei"`
	ObservedDelta string `json:"observedTransactionDeltaWei"`
	BlockDelta    string `json:"observedWholeBlockDeltaWei"`
	Match         bool   `json:"match"`
}

type traceAccount struct {
	Balance *hexutil.Big `json:"balance,omitempty"`
}

type prestateDiff struct {
	Pre  map[common.Address]traceAccount `json:"pre"`
	Post map[common.Address]traceAccount `json:"post"`
}

type traceEnvelope struct {
	Result json.RawMessage `json:"result"`
	Error  string          `json:"error,omitempty"`
}

type nodeResult struct {
	Node        int    `json:"node"`
	IPC         string `json:"ipc"`
	BlockHash   string `json:"blockHash,omitempty"`
	ReceiptHash string `json:"receiptBlockHash,omitempty"`
	Status      string `json:"status"`
	Error       string `json:"error,omitempty"`
}

type report struct {
	AuditVersion      string          `json:"auditVersion"`
	GeneratedAt       string          `json:"generatedAt"`
	Status            string          `json:"status"`
	ChainID           string          `json:"chainId"`
	Sender            string          `json:"sender"`
	Recipient         string          `json:"recipient"`
	Producer          string          `json:"producer"`
	TransactionHash   string          `json:"transactionHash"`
	BlockNumber       uint64          `json:"blockNumber"`
	BlockHash         string          `json:"blockHash"`
	ParentHash        string          `json:"parentHash"`
	TransactionIndex  uint            `json:"transactionIndex"`
	ReceiptStatus     uint64          `json:"receiptStatus"`
	GasUsed           uint64          `json:"gasUsed"`
	ValueWei          string          `json:"valueWei"`
	BaseFeePerGasWei  string          `json:"baseFeePerGasWei"`
	EffectiveGasPrice string          `json:"effectiveGasPriceWei"`
	PriorityFeePerGas string          `json:"priorityFeePerGasWei"`
	TransactionFeeWei string          `json:"transactionFeeWei"`
	ProducerTipWei    string          `json:"producerTipWei"`
	TotalBlockTipsWei string          `json:"totalBlockTipsWei"`
	BurnedBaseFeeWei  string          `json:"burnedBaseFeeWei"`
	BalanceAccounting string          `json:"balanceAccounting"`
	SenderNonceBefore uint64          `json:"senderNonceBefore"`
	SenderNonceAfter  uint64          `json:"senderNonceAfter"`
	BlockTransactions int             `json:"blockTransactions"`
	Balances          []balanceResult `json:"balances"`
	Nodes             []nodeResult    `json:"nodes"`
	Errors            []string        `json:"errors,omitempty"`
}

func main() {
	opts := parseFlags()
	ctx, cancel := context.WithTimeout(context.Background(), opts.timeout)
	defer cancel()

	report, err := run(ctx, opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ERRO:", err)
		os.Exit(1)
	}
	if err := writeJSON(opts.jsonPath, report); err != nil {
		fmt.Fprintln(os.Stderr, "ERROR writing JSON:", err)
		os.Exit(1)
	}
	if err := writeMarkdown(opts.markdownPath, report); err != nil {
		fmt.Fprintln(os.Stderr, "ERROR writing summary:", err)
		os.Exit(1)
	}

	fmt.Println("TRANSACTION AUDIT COMPLETED")
	fmt.Println("Status:", report.Status)
	fmt.Println("Transaction:", report.TransactionHash)
	fmt.Printf("Block: %d | producer: %s\n", report.BlockNumber, report.Producer)
	fmt.Println("Valor (wei):", report.ValueWei)
	fmt.Println("Total fee (wei):", report.TransactionFeeWei)
	fmt.Println("Producer tip (wei):", report.ProducerTipWei)
	fmt.Println("Base fee queimada (wei):", report.BurnedBaseFeeWei)
	fmt.Printf("Passing nodes: %d/%d\n", passingNodes(report.Nodes), len(report.Nodes))
	if report.Status != "PASS" {
		os.Exit(2)
	}
}

func parseFlags() options {
	var opts options
	flag.StringVar(&opts.base, "base", "/tmp/rabbit-20nodes", "base node directory")
	flag.IntVar(&opts.nodes, "nodes", 20, "number of nodes")
	flag.IntVar(&opts.senderNode, "sender-node", 20, "node containing the funded sender account")
	flag.IntVar(&opts.recipientNode, "recipient-node", 2, "node whose account will receive the transfer")
	flag.StringVar(&opts.valueWei, "value", "1000000000000000000", "transferred value in wei")
	flag.StringVar(&opts.tipWei, "tip", "2000000000", "maxPriorityFeePerGas in wei")
	flag.DurationVar(&opts.timeout, "timeout", 3*time.Minute, "maximum test duration")
	flag.StringVar(&opts.jsonPath, "json", "rabbit-tx-audit.json", "JSON report")
	flag.StringVar(&opts.markdownPath, "summary", "rabbit-tx-audit.md", "resumo Markdown")
	flag.StringVar(&opts.verifyNodes, "verify-nodes", "", "nodes that must confirm the transaction, comma-separated (default: all)")
	flag.Parse()
	return opts
}

func run(ctx context.Context, opts options) (*report, error) {
	if opts.nodes < 1 || opts.senderNode < 1 || opts.senderNode > opts.nodes || opts.recipientNode < 1 || opts.recipientNode > opts.nodes {
		return nil, errors.New("invalid node configuration")
	}
	selectedNodes, err := parseNodeSelection(opts.verifyNodes, opts.nodes)
	if err != nil {
		return nil, err
	}
	value, ok := new(big.Int).SetString(opts.valueWei, 10)
	if !ok || value.Sign() <= 0 {
		return nil, fmt.Errorf("invalid value: %s", opts.valueWei)
	}
	tipCap, ok := new(big.Int).SetString(opts.tipWei, 10)
	if !ok || tipCap.Sign() <= 0 {
		return nil, fmt.Errorf("invalid tip: %s", opts.tipWei)
	}

	queryRPC, queryETH, err := dialNode(ctx, opts.base, 1)
	if err != nil {
		return nil, err
	}
	defer queryRPC.Close()
	senderRPC, senderETH, err := dialNode(ctx, opts.base, opts.senderNode)
	if err != nil {
		return nil, err
	}
	defer senderRPC.Close()
	recipientRPC, _, err := dialNode(ctx, opts.base, opts.recipientNode)
	if err != nil {
		return nil, err
	}
	defer recipientRPC.Close()

	sender, err := firstAccount(ctx, senderRPC)
	if err != nil {
		return nil, fmt.Errorf("sender account: %w", err)
	}
	recipient, err := firstAccount(ctx, recipientRPC)
	if err != nil {
		return nil, fmt.Errorf("recipient account: %w", err)
	}
	if sender == recipient {
		return nil, errors.New("sender and recipient are identical")
	}
	chainID, err := queryETH.ChainID(ctx)
	if err != nil {
		return nil, fmt.Errorf("chain ID: %w", err)
	}
	head, err := queryETH.HeaderByNumber(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("latest header: %w", err)
	}
	if head.BaseFee == nil {
		return nil, errors.New("latest block has no base fee on a London chain")
	}
	feeCap := new(big.Int).Mul(head.BaseFee, big.NewInt(2))
	feeCap.Add(feeCap, tipCap)
	gas := uint64(21_000)
	senderBalance, err := queryETH.BalanceAt(ctx, sender, nil)
	if err != nil {
		return nil, fmt.Errorf("latest sender balance: %w", err)
	}
	maximumCost := new(big.Int).Add(
		new(big.Int).Set(value),
		new(big.Int).Mul(new(big.Int).SetUint64(gas), feeCap),
	)
	if senderBalance.Cmp(maximumCost) < 0 {
		return nil, fmt.Errorf("sender %s has %s wei, but transaction can require up to %s wei; restart the transaction-enabled lab", sender, senderBalance, maximumCost)
	}
	nonce, err := senderETH.PendingNonceAt(ctx, sender)
	if err != nil {
		return nil, fmt.Errorf("pending sender nonce: %w", err)
	}
	unsigned := types.NewTx(&types.DynamicFeeTx{
		ChainID:   new(big.Int).Set(chainID),
		Nonce:     nonce,
		GasTipCap: new(big.Int).Set(tipCap),
		GasFeeCap: new(big.Int).Set(feeCap),
		Gas:       gas,
		To:        &recipient,
		Value:     new(big.Int).Set(value),
	})
	signed, err := signTransaction(opts.base, opts.senderNode, sender, unsigned, chainID)
	if err != nil {
		return nil, fmt.Errorf("sign transaction from %s: %w", sender, err)
	}
	txHash := signed.Hash()
	if err := senderETH.SendTransaction(ctx, signed); err != nil {
		return nil, fmt.Errorf("send signed transaction from %s: %w", sender, err)
	}
	fmt.Println("Signed transaction sent:", txHash.Hex())
	receipt, err := waitReceipt(ctx, queryETH, txHash)
	if err != nil {
		return nil, err
	}
	block, err := queryETH.BlockByHash(ctx, receipt.BlockHash)
	if err != nil {
		return nil, fmt.Errorf("inclusion block: %w", err)
	}
	parent, err := queryETH.BlockByHash(ctx, block.ParentHash())
	if err != nil {
		return nil, fmt.Errorf("parent block: %w", err)
	}
	if block.BaseFee() == nil || receipt.EffectiveGasPrice == nil {
		return nil, errors.New("missing fee fields in inclusion block or receipt")
	}

	result := &report{
		AuditVersion:      auditVersion,
		GeneratedAt:       time.Now().UTC().Format(time.RFC3339),
		Status:            "PASS",
		ChainID:           chainID.String(),
		Sender:            sender.Hex(),
		Recipient:         recipient.Hex(),
		Producer:          block.Coinbase().Hex(),
		TransactionHash:   txHash.Hex(),
		BlockNumber:       block.NumberU64(),
		BlockHash:         block.Hash().Hex(),
		ParentHash:        parent.Hash().Hex(),
		TransactionIndex:  receipt.TransactionIndex,
		ReceiptStatus:     receipt.Status,
		GasUsed:           receipt.GasUsed,
		ValueWei:          value.String(),
		BaseFeePerGasWei:  block.BaseFee().String(),
		EffectiveGasPrice: receipt.EffectiveGasPrice.String(),
		BlockTransactions: len(block.Transactions()),
	}
	if receipt.Status != types.ReceiptStatusSuccessful {
		result.Errors = append(result.Errors, "transaction receipt failed")
	}
	if !blockContains(block, txHash) {
		result.Errors = append(result.Errors, "transaction is absent from its reported block")
	}

	priorityPerGas := new(big.Int).Sub(new(big.Int).Set(receipt.EffectiveGasPrice), block.BaseFee())
	if priorityPerGas.Sign() < 0 {
		result.Errors = append(result.Errors, "effective gas price is below base fee")
		priorityPerGas.SetInt64(0)
	}
	txFee := new(big.Int).Mul(new(big.Int).SetUint64(receipt.GasUsed), receipt.EffectiveGasPrice)
	txTip := new(big.Int).Mul(new(big.Int).SetUint64(receipt.GasUsed), priorityPerGas)
	burned := new(big.Int).Mul(new(big.Int).SetUint64(receipt.GasUsed), block.BaseFee())
	totalBlockTips, err := blockTips(ctx, queryETH, block)
	if err != nil {
		return nil, err
	}
	result.PriorityFeePerGas = priorityPerGas.String()
	result.TransactionFeeWei = txFee.String()
	result.ProducerTipWei = txTip.String()
	result.TotalBlockTipsWei = totalBlockTips.String()
	result.BurnedBaseFeeWei = burned.String()
	result.BalanceAccounting = "transaction-only prestateTracer diff; block rewards reported separately by rabbit-audit"

	transactionDeltas, err := transactionBalanceDeltas(ctx, queryRPC, block.NumberU64(), receipt.TransactionIndex)
	if err != nil {
		return nil, err
	}

	beforeNonce, err := queryETH.NonceAtHash(ctx, sender, parent.Hash())
	if err != nil {
		return nil, fmt.Errorf("sender nonce at parent: %w", err)
	}
	afterNonce, err := queryETH.NonceAtHash(ctx, sender, block.Hash())
	if err != nil {
		return nil, fmt.Errorf("sender nonce at block: %w", err)
	}
	result.SenderNonceBefore = beforeNonce
	result.SenderNonceAfter = afterNonce
	if afterNonce != beforeNonce+1 {
		result.Errors = append(result.Errors, fmt.Sprintf("sender nonce: got %d want %d", afterNonce, beforeNonce+1))
	}

	expected := make(map[common.Address]*big.Int)
	roles := make(map[common.Address][]string)
	addDelta(expected, sender, new(big.Int).Neg(new(big.Int).Add(new(big.Int).Set(value), txFee)))
	addRole(roles, sender, "sender")
	addDelta(expected, recipient, value)
	addRole(roles, recipient, "recipient")
	addDelta(expected, block.Coinbase(), txTip)
	addRole(roles, block.Coinbase(), "producer")
	addresses := make([]common.Address, 0, len(expected))
	for address := range expected {
		addresses = append(addresses, address)
	}
	sort.Slice(addresses, func(i, j int) bool {
		return addresses[i].Hex() < addresses[j].Hex()
	})
	for _, address := range addresses {
		want := expected[address]
		before, err := queryETH.BalanceAtHash(ctx, address, parent.Hash())
		if err != nil {
			return nil, fmt.Errorf("balance %s at parent: %w", address, err)
		}
		after, err := queryETH.BalanceAtHash(ctx, address, block.Hash())
		if err != nil {
			return nil, fmt.Errorf("balance %s at block: %w", address, err)
		}
		blockObserved := new(big.Int).Sub(after, before)
		observed := new(big.Int)
		if delta := transactionDeltas[address]; delta != nil {
			observed.Set(delta)
		}
		match := observed.Cmp(want) == 0
		result.Balances = append(result.Balances, balanceResult{
			Address:       address.Hex(),
			Role:          joinRoles(roles[address]),
			BeforeWei:     before.String(),
			AfterWei:      after.String(),
			ExpectedDelta: want.String(),
			ObservedDelta: observed.String(),
			BlockDelta:    blockObserved.String(),
			Match:         match,
		})
		if !match {
			result.Errors = append(result.Errors, fmt.Sprintf("balance mismatch for %s", address))
		}
	}
	result.Nodes = verifyNodes(ctx, opts, selectedNodes, block, txHash)
	for _, node := range result.Nodes {
		if node.Status != "PASS" {
			result.Errors = append(result.Errors, fmt.Sprintf("node%d did not confirm canonical transaction", node.Node))
		}
	}
	if len(result.Errors) > 0 {
		result.Status = "FAIL"
	}
	return result, nil
}

func dialNode(ctx context.Context, base string, node int) (*rpc.Client, *ethclient.Client, error) {
	path := filepath.Join(base, fmt.Sprintf("node%d", node), "geth.ipc")
	client, err := rpc.DialIPC(ctx, path)
	if err != nil {
		return nil, nil, fmt.Errorf("dial node%d (%s): %w", node, path, err)
	}
	return client, ethclient.NewClient(client), nil
}

func firstAccount(ctx context.Context, client *rpc.Client) (common.Address, error) {
	var accounts []common.Address
	if err := client.CallContext(ctx, &accounts, "eth_accounts"); err != nil {
		return common.Address{}, err
	}
	if len(accounts) == 0 {
		return common.Address{}, errors.New("node has no account")
	}
	return accounts[0], nil
}

func signTransaction(base string, node int, sender common.Address, tx *types.Transaction, chainID *big.Int) (*types.Transaction, error) {
	passwordBytes, err := os.ReadFile(filepath.Join(base, "password.txt"))
	if err != nil {
		return nil, fmt.Errorf("read laboratory password file: %w", err)
	}
	keyDir := filepath.Join(base, fmt.Sprintf("node%d", node), "keystore")
	store := keystore.NewKeyStore(keyDir, keystore.LightScryptN, keystore.LightScryptP)
	account, err := store.Find(accounts.Account{Address: sender})
	if err != nil {
		return nil, fmt.Errorf("find sender in %s: %w", keyDir, err)
	}
	signed, err := store.SignTxWithPassphrase(account, strings.TrimSpace(string(passwordBytes)), tx, chainID)
	if err != nil {
		return nil, err
	}
	return signed, nil
}

func waitReceipt(ctx context.Context, client *ethclient.Client, hash common.Hash) (*types.Receipt, error) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		receipt, err := client.TransactionReceipt(ctx, hash)
		if err == nil {
			return receipt, nil
		}
		if !errors.Is(err, ethereum.NotFound) {
			return nil, fmt.Errorf("transaction receipt: %w", err)
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("wait transaction receipt: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

func blockContains(block *types.Block, hash common.Hash) bool {
	for _, tx := range block.Transactions() {
		if tx.Hash() == hash {
			return true
		}
	}
	return false
}

func blockTips(ctx context.Context, client *ethclient.Client, block *types.Block) (*big.Int, error) {
	total := new(big.Int)
	for _, tx := range block.Transactions() {
		receipt, err := client.TransactionReceipt(ctx, tx.Hash())
		if err != nil {
			return nil, fmt.Errorf("receipt %s: %w", tx.Hash(), err)
		}
		tip, err := tx.EffectiveGasTip(block.BaseFee())
		if err != nil {
			return nil, fmt.Errorf("effective tip %s: %w", tx.Hash(), err)
		}
		total.Add(total, new(big.Int).Mul(new(big.Int).SetUint64(receipt.GasUsed), tip))
	}
	return total, nil
}

func transactionBalanceDeltas(ctx context.Context, client *rpc.Client, block uint64, transactionIndex uint) (map[common.Address]*big.Int, error) {
	var traces []traceEnvelope
	config := map[string]interface{}{
		"tracer": "prestateTracer",
		"tracerConfig": map[string]interface{}{
			"diffMode":       true,
			"disableCode":    true,
			"disableStorage": true,
		},
		"timeout": "60s",
	}
	if err := client.CallContext(ctx, &traces, "debug_traceBlockByNumber", hexutil.EncodeUint64(block), config); err != nil {
		return nil, fmt.Errorf("trace transaction balances in block %d: %w", block, err)
	}
	if uint(len(traces)) <= transactionIndex {
		return nil, fmt.Errorf("trace transaction %d in block %d: only %d traces returned", transactionIndex, block, len(traces))
	}
	envelope := traces[transactionIndex]
	if envelope.Error != "" {
		return nil, fmt.Errorf("trace transaction %d in block %d: %s", transactionIndex, block, envelope.Error)
	}
	return decodeBalanceDeltas(envelope.Result)
}

func decodeBalanceDeltas(raw json.RawMessage) (map[common.Address]*big.Int, error) {
	var diff prestateDiff
	if err := json.Unmarshal(raw, &diff); err != nil {
		return nil, fmt.Errorf("decode transaction prestate diff: %w", err)
	}
	addresses := make(map[common.Address]struct{}, len(diff.Pre)+len(diff.Post))
	for address := range diff.Pre {
		addresses[address] = struct{}{}
	}
	for address := range diff.Post {
		addresses[address] = struct{}{}
	}
	deltas := make(map[common.Address]*big.Int)
	for address := range addresses {
		pre, existedBefore := diff.Pre[address]
		post, existsAfter := diff.Post[address]
		before := new(big.Int)
		if existedBefore && pre.Balance != nil {
			before.Set((*big.Int)(pre.Balance))
		}
		after := new(big.Int)
		switch {
		case existsAfter && post.Balance != nil:
			after.Set((*big.Int)(post.Balance))
		case existsAfter && existedBefore:
			after.Set(before)
		}
		delta := new(big.Int).Sub(after, before)
		if delta.Sign() != 0 {
			deltas[address] = delta
		}
	}
	return deltas, nil
}

func addDelta(deltas map[common.Address]*big.Int, address common.Address, amount *big.Int) {
	if deltas[address] == nil {
		deltas[address] = new(big.Int)
	}
	deltas[address].Add(deltas[address], amount)
}

func addRole(roles map[common.Address][]string, address common.Address, role string) {
	roles[address] = append(roles[address], role)
}

func joinRoles(roles []string) string {
	if len(roles) == 0 {
		return ""
	}
	result := roles[0]
	for _, role := range roles[1:] {
		result += "+" + role
	}
	return result
}

func parseNodeSelection(spec string, total int) ([]int, error) {
	if total < 1 {
		return nil, errors.New("node count must be positive")
	}
	if strings.TrimSpace(spec) == "" {
		nodes := make([]int, total)
		for i := range nodes {
			nodes[i] = i + 1
		}
		return nodes, nil
	}
	seen := make(map[int]bool)
	var nodes []int
	for _, item := range strings.Split(spec, ",") {
		item = strings.TrimSpace(item)
		value, ok := new(big.Int).SetString(item, 10)
		if !ok || !value.IsInt64() {
			return nil, fmt.Errorf("invalid verify node %q", item)
		}
		node := int(value.Int64())
		if node < 1 || node > total {
			return nil, fmt.Errorf("verify node %d is outside 1..%d", node, total)
		}
		if seen[node] {
			return nil, fmt.Errorf("verify node %d is duplicated", node)
		}
		seen[node] = true
		nodes = append(nodes, node)
	}
	if len(nodes) == 0 {
		return nil, errors.New("verify node selection is empty")
	}
	sort.Ints(nodes)
	return nodes, nil
}

func verifyNodes(ctx context.Context, opts options, selectedNodes []int, block *types.Block, txHash common.Hash) []nodeResult {
	results := make([]nodeResult, 0, len(selectedNodes))
	for _, node := range selectedNodes {
		result := nodeResult{Node: node, IPC: filepath.Join(opts.base, fmt.Sprintf("node%d", node), "geth.ipc"), Status: "FAIL"}
		clientRPC, clientETH, err := dialNode(ctx, opts.base, node)
		if err != nil {
			result.Error = err.Error()
			results = append(results, result)
			continue
		}
		deadline := time.Now().Add(45 * time.Second)
		for {
			candidate, blockErr := clientETH.BlockByNumber(ctx, block.Number())
			receipt, receiptErr := clientETH.TransactionReceipt(ctx, txHash)
			if blockErr == nil && receiptErr == nil && candidate.Hash() == block.Hash() && receipt.BlockHash == block.Hash() {
				result.BlockHash = candidate.Hash().Hex()
				result.ReceiptHash = receipt.BlockHash.Hex()
				result.Status = "PASS"
				break
			}
			if time.Now().After(deadline) || ctx.Err() != nil {
				result.Error = fmt.Sprintf("blockErr=%v receiptErr=%v", blockErr, receiptErr)
				break
			}
			time.Sleep(time.Second)
		}
		clientRPC.Close()
		results = append(results, result)
	}
	return results
}

func passingNodes(nodes []nodeResult) int {
	count := 0
	for _, node := range nodes {
		if node.Status == "PASS" {
			count++
		}
	}
	return count
}

func writeJSON(path string, value any) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func writeMarkdown(path string, report *report) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	fmt.Fprintln(file, "# Transaction audit — Rabbit Chain")
	fmt.Fprintln(file)
	fmt.Fprintf(file, "**Status: %s**\n\n", report.Status)
	fmt.Fprintf(file, "- Transaction: `%s`\n", report.TransactionHash)
	fmt.Fprintf(file, "- Block: `%d`\n", report.BlockNumber)
	fmt.Fprintf(file, "- Producer: `%s`\n", report.Producer)
	fmt.Fprintf(file, "- Valor: `%s wei`\n", report.ValueWei)
	fmt.Fprintf(file, "- Gas usado: `%d`\n", report.GasUsed)
	fmt.Fprintf(file, "- Total fee: `%s wei`\n", report.TransactionFeeWei)
	fmt.Fprintf(file, "- Tip for this transaction: `%s wei`\n", report.ProducerTipWei)
	fmt.Fprintf(file, "- Total block tips: `%s wei`\n", report.TotalBlockTipsWei)
	fmt.Fprintf(file, "- Base fee queimada: `%s wei`\n", report.BurnedBaseFeeWei)
	fmt.Fprintf(file, "- Nodes on the same chain: `%d/%d`\n\n", passingNodes(report.Nodes), len(report.Nodes))
	fmt.Fprintln(file, "## Balances")
	fmt.Fprintln(file)
	fmt.Fprintln(file, "The transaction delta is isolated by trace. The full-block delta also includes immediate producer/committee rewards.")
	fmt.Fprintln(file)
	fmt.Fprintln(file, "| Role | Address | Expected for transaction (wei) | Observed for transaction (wei) | Full block (wei) | Matches |")
	fmt.Fprintln(file, "| --- | --- | ---: | ---: | ---: | --- |")
	for _, balance := range report.Balances {
		fmt.Fprintf(file, "| %s | `%s` | %s | %s | %s | %t |\n", balance.Role, balance.Address, balance.ExpectedDelta, balance.ObservedDelta, balance.BlockDelta, balance.Match)
	}
	if len(report.Errors) > 0 {
		fmt.Fprintln(file)
		fmt.Fprintln(file, "## Inconsistencies")
		fmt.Fprintln(file)
		for _, item := range report.Errors {
			fmt.Fprintf(file, "- %s\n", item)
		}
	}
	return nil
}
