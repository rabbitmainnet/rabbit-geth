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
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
)

const auditVersion = "rabbit-tx-stress/1.0.0"

type options struct {
	base         string
	nodes        int
	senderNode   int
	rounds       int
	perRound     int
	valueWei     string
	tipWei       string
	timeout      time.Duration
	jsonPath     string
	markdownPath string
}

type txResult struct {
	Hash             string `json:"hash"`
	Nonce            uint64 `json:"nonce"`
	Round            int    `json:"round"`
	Recipient        string `json:"recipient"`
	BlockNumber      uint64 `json:"blockNumber"`
	BlockHash        string `json:"blockHash"`
	Producer         string `json:"producer"`
	GasUsed          uint64 `json:"gasUsed"`
	ValueWei         string `json:"valueWei"`
	FeeWei           string `json:"feeWei"`
	TipWei           string `json:"tipWei"`
	BurnWei          string `json:"burnWei"`
	ReceiptStatus    uint64 `json:"receiptStatus"`
	TransactionIndex uint   `json:"transactionIndex"`
}

type blockResult struct {
	Number          uint64 `json:"number"`
	Hash            string `json:"hash"`
	Producer        string `json:"producer"`
	Transactions    int    `json:"transactions"`
	AuditedTxs      int    `json:"auditedTransactions"`
	AllTransactions bool   `json:"allTransactionsBelongToAudit"`
	AuditedTipsWei  string `json:"auditedTipsWei"`
}

type balanceResult struct {
	Address       string `json:"address"`
	Roles         string `json:"roles"`
	BeforeWei     string `json:"beforeWei"`
	AfterWei      string `json:"afterWei"`
	ExpectedDelta string `json:"expectedDeltaWei"`
	ObservedDelta string `json:"observedDeltaWei"`
	Match         bool   `json:"match"`
}

type invalidResult struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

type nodeResult struct {
	Node    int    `json:"node"`
	Status  string `json:"status"`
	Height  uint64 `json:"height,omitempty"`
	Pending string `json:"pending,omitempty"`
	Queued  string `json:"queued,omitempty"`
	Blocks  int    `json:"blocksConfirmed"`
	Error   string `json:"error,omitempty"`
}

type report struct {
	AuditVersion       string          `json:"auditVersion"`
	GeneratedAt        string          `json:"generatedAt"`
	Status             string          `json:"status"`
	ChainID            string          `json:"chainId"`
	Sender             string          `json:"sender"`
	StartBlock         uint64          `json:"startBlock"`
	StartHash          string          `json:"startHash"`
	FinalBlock         uint64          `json:"finalBlock"`
	FinalHash          string          `json:"finalHash"`
	Rounds             int             `json:"rounds"`
	TransactionsSent   int             `json:"transactionsSent"`
	TransactionsPassed int             `json:"transactionsPassed"`
	ValueWei           string          `json:"totalValueWei"`
	FeesWei            string          `json:"totalFeesWei"`
	TipsWei            string          `json:"totalTipsWei"`
	BurnWei            string          `json:"totalBurnWei"`
	SenderNonceBefore  uint64          `json:"senderNonceBefore"`
	SenderNonceAfter   uint64          `json:"senderNonceAfter"`
	Transactions       []txResult      `json:"transactions"`
	Blocks             []blockResult   `json:"blocks"`
	Balances           []balanceResult `json:"balances"`
	InvalidTests       []invalidResult `json:"invalidTransactionTests"`
	Nodes              []nodeResult    `json:"nodes"`
	Errors             []string        `json:"errors,omitempty"`
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
		fmt.Fprintln(os.Stderr, "ERRO ao gravar JSON:", err)
		os.Exit(1)
	}
	if err := writeMarkdown(opts.markdownPath, report); err != nil {
		fmt.Fprintln(os.Stderr, "ERRO ao gravar resumo:", err)
		os.Exit(1)
	}

	fmt.Println("AUDITORIA DE CARGA CONCLUÍDA")
	fmt.Println("Status:", report.Status)
	fmt.Printf("Transações: %d/%d | blocos: %d | nós: %d/%d\n",
		report.TransactionsPassed, report.TransactionsSent, len(report.Blocks), passingNodes(report.Nodes), len(report.Nodes))
	fmt.Println("Taxas totais (wei):", report.FeesWei)
	fmt.Println("Tips totais (wei):", report.TipsWei)
	fmt.Println("Burn total (wei):", report.BurnWei)
	if report.Status != "PASS" {
		os.Exit(2)
	}
}

func parseFlags() options {
	var opts options
	flag.StringVar(&opts.base, "base", "/tmp/rabbit-20nodes", "diretório base dos nós")
	flag.IntVar(&opts.nodes, "nodes", 20, "quantidade de nós")
	flag.IntVar(&opts.senderNode, "sender-node", 20, "nó remetente financiado no laboratório")
	flag.IntVar(&opts.rounds, "rounds", 5, "quantidade de lotes")
	flag.IntVar(&opts.perRound, "per-round", 25, "transações por lote")
	flag.StringVar(&opts.valueWei, "value", "1000000000000000", "valor de cada transferência em wei")
	flag.StringVar(&opts.tipWei, "tip", "2000000000", "maxPriorityFeePerGas em wei")
	flag.DurationVar(&opts.timeout, "timeout", 15*time.Minute, "tempo máximo do teste")
	flag.StringVar(&opts.jsonPath, "json", "rabbit-tx-stress.json", "relatório JSON")
	flag.StringVar(&opts.markdownPath, "summary", "rabbit-tx-stress.md", "resumo Markdown")
	flag.Parse()
	return opts
}

func run(ctx context.Context, opts options) (*report, error) {
	if opts.nodes < 2 || opts.senderNode < 1 || opts.senderNode > opts.nodes || opts.rounds < 1 || opts.perRound < 1 {
		return nil, errors.New("configuração inválida")
	}
	value, ok := new(big.Int).SetString(opts.valueWei, 10)
	if !ok || value.Sign() <= 0 {
		return nil, fmt.Errorf("valor inválido: %s", opts.valueWei)
	}
	tipCap, ok := new(big.Int).SetString(opts.tipWei, 10)
	if !ok || tipCap.Sign() <= 0 {
		return nil, fmt.Errorf("tip inválido: %s", opts.tipWei)
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

	sender, err := firstAccount(ctx, senderRPC)
	if err != nil {
		return nil, fmt.Errorf("conta remetente: %w", err)
	}
	chainID, err := queryETH.ChainID(ctx)
	if err != nil {
		return nil, fmt.Errorf("chain ID: %w", err)
	}
	privateKey, err := laboratoryKey(opts.base, opts.senderNode, sender)
	if err != nil {
		return nil, err
	}
	recipients, err := laboratoryRecipients(ctx, opts, sender)
	if err != nil {
		return nil, err
	}
	start, err := queryETH.BlockByNumber(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("bloco inicial: %w", err)
	}
	initialNonce, err := queryETH.NonceAtHash(ctx, sender, start.Hash())
	if err != nil {
		return nil, fmt.Errorf("nonce inicial: %w", err)
	}
	initialBalance, err := queryETH.BalanceAtHash(ctx, sender, start.Hash())
	if err != nil {
		return nil, fmt.Errorf("saldo inicial: %w", err)
	}

	totalTransactions := opts.rounds * opts.perRound
	minimumRequired := new(big.Int).Mul(new(big.Int).Set(value), big.NewInt(int64(totalTransactions)))
	minimumRequired.Add(minimumRequired, new(big.Int).Mul(big.NewInt(int64(totalTransactions*21_000)), tipCap))
	if initialBalance.Cmp(minimumRequired) <= 0 {
		return nil, fmt.Errorf("saldo insuficiente para o teste: saldo=%s mínimo=%s", initialBalance, minimumRequired)
	}

	result := &report{
		AuditVersion:      auditVersion,
		GeneratedAt:       time.Now().UTC().Format(time.RFC3339),
		Status:            "PASS",
		ChainID:           chainID.String(),
		Sender:            sender.Hex(),
		StartBlock:        start.NumberU64(),
		StartHash:         start.Hash().Hex(),
		Rounds:            opts.rounds,
		SenderNonceBefore: initialNonce,
	}

	signedByHash := make(map[common.Hash]*types.Transaction)
	recipientForHash := make(map[common.Hash]common.Address)
	receipts := make(map[common.Hash]*types.Receipt)
	signer := types.LatestSignerForChainID(chainID)
	nextNonce := initialNonce

	for round := 1; round <= opts.rounds; round++ {
		head, err := queryETH.HeaderByNumber(ctx, nil)
		if err != nil {
			return nil, fmt.Errorf("cabeçalho antes do lote %d: %w", round, err)
		}
		if head.BaseFee == nil {
			return nil, errors.New("cadeia London sem base fee")
		}
		feeCap := new(big.Int).Mul(new(big.Int).Set(head.BaseFee), big.NewInt(3))
		feeCap.Add(feeCap, tipCap)
		var roundHashes []common.Hash
		fmt.Printf("Lote %d/%d: assinando e enviando %d transações...\n", round, opts.rounds, opts.perRound)
		for index := 0; index < opts.perRound; index++ {
			recipient := recipients[((round-1)*opts.perRound+index)%len(recipients)]
			unsigned := types.NewTx(&types.DynamicFeeTx{
				ChainID:   new(big.Int).Set(chainID),
				Nonce:     nextNonce,
				GasTipCap: new(big.Int).Set(tipCap),
				GasFeeCap: new(big.Int).Set(feeCap),
				Gas:       21_000,
				To:        &recipient,
				Value:     new(big.Int).Set(value),
			})
			signed, err := types.SignTx(unsigned, signer, privateKey.PrivateKey)
			if err != nil {
				return nil, fmt.Errorf("assinar nonce %d: %w", nextNonce, err)
			}
			if err := senderETH.SendTransaction(ctx, signed); err != nil {
				return nil, fmt.Errorf("enviar nonce %d: %w", nextNonce, err)
			}
			hash := signed.Hash()
			signedByHash[hash] = signed
			recipientForHash[hash] = recipient
			roundHashes = append(roundHashes, hash)
			nextNonce++
		}
		if err := waitReceipts(ctx, queryETH, roundHashes, receipts); err != nil {
			return nil, fmt.Errorf("lote %d: %w", round, err)
		}
		fmt.Printf("Lote %d/%d confirmado.\n", round, opts.rounds)
	}

	blockMap := make(map[common.Hash]*types.Block)
	totalValue := new(big.Int)
	totalFees := new(big.Int)
	totalTips := new(big.Int)
	totalBurn := new(big.Int)
	expected := make(map[common.Address]*big.Int)
	roles := make(map[common.Address]map[string]bool)
	addRole(roles, sender, "sender")

	hashes := make([]common.Hash, 0, len(signedByHash))
	for hash := range signedByHash {
		hashes = append(hashes, hash)
	}
	sort.Slice(hashes, func(i, j int) bool { return signedByHash[hashes[i]].Nonce() < signedByHash[hashes[j]].Nonce() })
	for _, hash := range hashes {
		tx := signedByHash[hash]
		receipt := receipts[hash]
		block := blockMap[receipt.BlockHash]
		if block == nil {
			block, err = queryETH.BlockByHash(ctx, receipt.BlockHash)
			if err != nil {
				return nil, fmt.Errorf("bloco %s: %w", receipt.BlockHash, err)
			}
			blockMap[receipt.BlockHash] = block
		}
		if receipt.Status != types.ReceiptStatusSuccessful {
			result.Errors = append(result.Errors, fmt.Sprintf("receipt falhou: %s", hash))
		}
		priority := new(big.Int).Sub(new(big.Int).Set(receipt.EffectiveGasPrice), block.BaseFee())
		if priority.Sign() < 0 {
			priority.SetInt64(0)
			result.Errors = append(result.Errors, fmt.Sprintf("gas efetivo abaixo da base fee: %s", hash))
		}
		fee := new(big.Int).Mul(new(big.Int).SetUint64(receipt.GasUsed), receipt.EffectiveGasPrice)
		tip := new(big.Int).Mul(new(big.Int).SetUint64(receipt.GasUsed), priority)
		burn := new(big.Int).Mul(new(big.Int).SetUint64(receipt.GasUsed), block.BaseFee())
		if new(big.Int).Add(new(big.Int).Set(tip), burn).Cmp(fee) != 0 {
			result.Errors = append(result.Errors, fmt.Sprintf("fee != tip + burn: %s", hash))
		}
		recipient := recipientForHash[hash]
		totalValue.Add(totalValue, value)
		totalFees.Add(totalFees, fee)
		totalTips.Add(totalTips, tip)
		totalBurn.Add(totalBurn, burn)
		addDelta(expected, sender, new(big.Int).Neg(new(big.Int).Add(new(big.Int).Set(value), fee)))
		addDelta(expected, recipient, value)
		addDelta(expected, block.Coinbase(), tip)
		addRole(roles, recipient, "recipient")
		addRole(roles, block.Coinbase(), "producer")
		result.Transactions = append(result.Transactions, txResult{
			Hash:             hash.Hex(),
			Nonce:            tx.Nonce(),
			Round:            int((tx.Nonce()-initialNonce)/uint64(opts.perRound)) + 1,
			Recipient:        recipient.Hex(),
			BlockNumber:      block.NumberU64(),
			BlockHash:        block.Hash().Hex(),
			Producer:         block.Coinbase().Hex(),
			GasUsed:          receipt.GasUsed,
			ValueWei:         value.String(),
			FeeWei:           fee.String(),
			TipWei:           tip.String(),
			BurnWei:          burn.String(),
			ReceiptStatus:    receipt.Status,
			TransactionIndex: receipt.TransactionIndex,
		})
	}

	result.TransactionsSent = len(result.Transactions)
	result.TransactionsPassed = result.TransactionsSent
	for _, tx := range result.Transactions {
		if tx.ReceiptStatus != types.ReceiptStatusSuccessful {
			result.TransactionsPassed--
		}
	}
	result.ValueWei = totalValue.String()
	result.FeesWei = totalFees.String()
	result.TipsWei = totalTips.String()
	result.BurnWei = totalBurn.String()

	blocks := make([]*types.Block, 0, len(blockMap))
	for _, block := range blockMap {
		blocks = append(blocks, block)
	}
	sort.Slice(blocks, func(i, j int) bool { return blocks[i].NumberU64() < blocks[j].NumberU64() })
	for _, block := range blocks {
		audited := 0
		blockTips := new(big.Int)
		for _, tx := range block.Transactions() {
			if _, exists := signedByHash[tx.Hash()]; exists {
				audited++
				for _, item := range result.Transactions {
					if item.Hash == tx.Hash().Hex() {
						blockTips.Add(blockTips, mustBig(item.TipWei))
						break
					}
				}
			}
		}
		allTransactions := audited == len(block.Transactions())
		if !allTransactions {
			result.Errors = append(result.Errors, fmt.Sprintf("bloco %d contém transações externas ao teste", block.NumberU64()))
		}
		result.Blocks = append(result.Blocks, blockResult{
			Number:          block.NumberU64(),
			Hash:            block.Hash().Hex(),
			Producer:        block.Coinbase().Hex(),
			Transactions:    len(block.Transactions()),
			AuditedTxs:      audited,
			AllTransactions: allTransactions,
			AuditedTipsWei:  blockTips.String(),
		})
	}
	if len(blocks) == 0 {
		return nil, errors.New("nenhum bloco de inclusão")
	}
	finalBlock := blocks[len(blocks)-1]
	result.FinalBlock = finalBlock.NumberU64()
	result.FinalHash = finalBlock.Hash().Hex()
	finalNonce, err := queryETH.NonceAtHash(ctx, sender, finalBlock.Hash())
	if err != nil {
		return nil, fmt.Errorf("nonce final: %w", err)
	}
	result.SenderNonceAfter = finalNonce
	if finalNonce != initialNonce+uint64(totalTransactions) {
		result.Errors = append(result.Errors, fmt.Sprintf("nonce final=%d esperado=%d", finalNonce, initialNonce+uint64(totalTransactions)))
	}

	addresses := make([]common.Address, 0, len(expected))
	for address := range expected {
		addresses = append(addresses, address)
	}
	sort.Slice(addresses, func(i, j int) bool { return addresses[i].Hex() < addresses[j].Hex() })
	for _, address := range addresses {
		before, err := queryETH.BalanceAtHash(ctx, address, start.Hash())
		if err != nil {
			return nil, fmt.Errorf("saldo inicial %s: %w", address, err)
		}
		after, err := queryETH.BalanceAtHash(ctx, address, finalBlock.Hash())
		if err != nil {
			return nil, fmt.Errorf("saldo final %s: %w", address, err)
		}
		observed := new(big.Int).Sub(after, before)
		want := expected[address]
		match := observed.Cmp(want) == 0
		if !match {
			result.Errors = append(result.Errors, fmt.Sprintf("saldo divergente: %s", address))
		}
		result.Balances = append(result.Balances, balanceResult{
			Address:       address.Hex(),
			Roles:         roleString(roles[address]),
			BeforeWei:     before.String(),
			AfterWei:      after.String(),
			ExpectedDelta: want.String(),
			ObservedDelta: observed.String(),
			Match:         match,
		})
	}

	syncCtx, cancelSync := context.WithTimeout(ctx, 2*time.Minute)
	syncErr := waitAllNodesCanonical(syncCtx, opts, finalBlock.NumberU64(), finalBlock.Hash())
	cancelSync()
	if syncErr != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("20 nós não confirmaram o bloco final: %v", syncErr))
		result.InvalidTests = []invalidResult{{Name: "canonical checkpoint before invalid tests", Status: "FAIL", Error: syncErr.Error()}}
	} else {
		result.InvalidTests = runInvalidTests(ctx, senderETH, privateKey, signer, chainID, sender, recipients[0], finalNonce, initialNonce, initialBalance, tipCap, signedByHash[hashes[0]])
	}
	for _, test := range result.InvalidTests {
		if test.Status != "PASS" {
			result.Errors = append(result.Errors, fmt.Sprintf("transação inválida não rejeitada: %s", test.Name))
		}
	}
	result.Nodes = verifyAllNodes(ctx, opts, result.Blocks, result.Transactions)
	for _, node := range result.Nodes {
		if node.Status != "PASS" {
			result.Errors = append(result.Errors, fmt.Sprintf("node%d falhou: %s", node.Node, node.Error))
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
		return nil, nil, fmt.Errorf("conectar node%d: %w", node, err)
	}
	return client, ethclient.NewClient(client), nil
}

func firstAccount(ctx context.Context, client *rpc.Client) (common.Address, error) {
	var list []common.Address
	if err := client.CallContext(ctx, &list, "eth_accounts"); err != nil {
		return common.Address{}, err
	}
	if len(list) == 0 {
		return common.Address{}, errors.New("nó sem conta")
	}
	return list[0], nil
}

func laboratoryRecipients(ctx context.Context, opts options, sender common.Address) ([]common.Address, error) {
	var recipients []common.Address
	for node := 1; node <= opts.nodes; node++ {
		if node == opts.senderNode {
			continue
		}
		client, _, err := dialNode(ctx, opts.base, node)
		if err != nil {
			return nil, err
		}
		address, err := firstAccount(ctx, client)
		client.Close()
		if err != nil {
			return nil, fmt.Errorf("conta node%d: %w", node, err)
		}
		if address != sender {
			recipients = append(recipients, address)
		}
	}
	if len(recipients) == 0 {
		return nil, errors.New("nenhum destinatário")
	}
	return recipients, nil
}

func laboratoryKey(base string, node int, sender common.Address) (*keystore.Key, error) {
	passwordBytes, err := os.ReadFile(filepath.Join(base, "password.txt"))
	if err != nil {
		return nil, fmt.Errorf("senha do laboratório: %w", err)
	}
	store := keystore.NewKeyStore(filepath.Join(base, fmt.Sprintf("node%d", node), "keystore"), keystore.LightScryptN, keystore.LightScryptP)
	account, err := store.Find(accounts.Account{Address: sender})
	if err != nil {
		return nil, fmt.Errorf("keystore remetente: %w", err)
	}
	keyJSON, err := os.ReadFile(account.URL.Path)
	if err != nil {
		return nil, err
	}
	key, err := keystore.DecryptKey(keyJSON, strings.TrimSpace(string(passwordBytes)))
	if err != nil {
		return nil, fmt.Errorf("descriptografar chave do laboratório: %w", err)
	}
	return key, nil
}

func waitReceipts(ctx context.Context, client *ethclient.Client, hashes []common.Hash, output map[common.Hash]*types.Receipt) error {
	pending := make(map[common.Hash]bool, len(hashes))
	for _, hash := range hashes {
		pending[hash] = true
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for len(pending) > 0 {
		for hash := range pending {
			receipt, err := client.TransactionReceipt(ctx, hash)
			if err == nil {
				output[hash] = receipt
				delete(pending, hash)
				continue
			}
			if !errors.Is(err, ethereum.NotFound) {
				return fmt.Errorf("receipt %s: %w", hash, err)
			}
		}
		if len(pending) == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("aguardar %d receipts: %w", len(pending), ctx.Err())
		case <-ticker.C:
		}
	}
	return nil
}

func runInvalidTests(ctx context.Context, client *ethclient.Client, key *keystore.Key, signer types.Signer, chainID *big.Int, sender, recipient common.Address, nextNonce, staleNonce uint64, initialBalance, tipCap *big.Int, first *types.Transaction) []invalidResult {
	tests := make([]invalidResult, 0, 4)
	tests = append(tests, rejected("duplicate mined transaction", client.SendTransaction(ctx, first), "nonce too low", "already known", "already imported", "known transaction"))

	stale := types.NewTx(&types.DynamicFeeTx{ChainID: chainID, Nonce: staleNonce, GasTipCap: tipCap, GasFeeCap: new(big.Int).Mul(tipCap, big.NewInt(3)), Gas: 21_000, To: &recipient, Value: big.NewInt(2)})
	stale, err := types.SignTx(stale, signer, key.PrivateKey)
	if err != nil {
		tests = append(tests, invalidResult{Name: "stale nonce", Status: "FAIL", Error: err.Error()})
	} else {
		tests = append(tests, rejected("stale nonce", client.SendTransaction(ctx, stale), "nonce too low"))
	}

	tooMuch := new(big.Int).Add(new(big.Int).Set(initialBalance), big.NewInt(1))
	insufficient := types.NewTx(&types.DynamicFeeTx{ChainID: chainID, Nonce: nextNonce, GasTipCap: tipCap, GasFeeCap: new(big.Int).Mul(tipCap, big.NewInt(3)), Gas: 21_000, To: &recipient, Value: tooMuch})
	insufficient, err = types.SignTx(insufficient, signer, key.PrivateKey)
	if err != nil {
		tests = append(tests, invalidResult{Name: "insufficient funds", Status: "FAIL", Error: err.Error()})
	} else {
		tests = append(tests, rejected("insufficient funds", client.SendTransaction(ctx, insufficient), "insufficient funds"))
	}

	wrongID := new(big.Int).Add(new(big.Int).Set(chainID), big.NewInt(1))
	wrongSigner := types.LatestSignerForChainID(wrongID)
	wrongChain := types.NewTx(&types.DynamicFeeTx{ChainID: wrongID, Nonce: nextNonce, GasTipCap: tipCap, GasFeeCap: new(big.Int).Mul(tipCap, big.NewInt(3)), Gas: 21_000, To: &recipient, Value: big.NewInt(1)})
	wrongChain, err = types.SignTx(wrongChain, wrongSigner, key.PrivateKey)
	if err != nil {
		tests = append(tests, invalidResult{Name: "wrong chain ID", Status: "FAIL", Error: err.Error()})
	} else {
		tests = append(tests, rejected("wrong chain ID", client.SendTransaction(ctx, wrongChain), "invalid sender", "chain id", "chainid", "replay-protected"))
	}
	_ = sender
	return tests
}

func rejected(name string, err error, expected ...string) invalidResult {
	if err == nil {
		return invalidResult{Name: name, Status: "FAIL", Error: "transação foi aceita"}
	}
	message := strings.ToLower(err.Error())
	for _, fragment := range expected {
		if strings.Contains(message, strings.ToLower(fragment)) {
			return invalidResult{Name: name, Status: "PASS", Error: err.Error()}
		}
	}
	return invalidResult{Name: name, Status: "FAIL", Error: "rejeição inesperada: " + err.Error()}
}

func waitAllNodesCanonical(ctx context.Context, opts options, number uint64, hash common.Hash) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		var pending []string
		for node := 1; node <= opts.nodes; node++ {
			rpcClient, ethClient, err := dialNode(ctx, opts.base, node)
			if err != nil {
				pending = append(pending, fmt.Sprintf("node%d: %v", node, err))
				continue
			}
			candidate, blockErr := ethClient.BlockByNumber(ctx, new(big.Int).SetUint64(number))
			rpcClient.Close()
			if blockErr != nil {
				pending = append(pending, fmt.Sprintf("node%d: %v", node, blockErr))
				continue
			}
			if candidate == nil || candidate.Hash() != hash {
				pending = append(pending, fmt.Sprintf("node%d: hash divergente", node))
			}
		}
		if len(pending) == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("%w (%s)", ctx.Err(), strings.Join(pending, "; "))
		case <-ticker.C:
		}
	}
}

func verifyAllNodes(ctx context.Context, opts options, blocks []blockResult, transactions []txResult) []nodeResult {
	results := make([]nodeResult, 0, opts.nodes)
	for node := 1; node <= opts.nodes; node++ {
		item := nodeResult{Node: node, Status: "PASS"}
		rpcClient, ethClient, err := dialNode(ctx, opts.base, node)
		if err != nil {
			item.Status = "FAIL"
			item.Error = err.Error()
			results = append(results, item)
			continue
		}
		for _, block := range blocks {
			candidate, err := ethClient.BlockByNumber(ctx, new(big.Int).SetUint64(block.Number))
			if err != nil || candidate == nil || candidate.Hash().Hex() != block.Hash {
				item.Status = "FAIL"
				item.Error = fmt.Sprintf("bloco %d divergente: %v", block.Number, err)
				break
			}
			item.Blocks++
		}
		if item.Status == "PASS" && len(transactions) > 0 {
			for _, index := range []int{0, len(transactions) - 1} {
				tx := transactions[index]
				receipt, err := ethClient.TransactionReceipt(ctx, common.HexToHash(tx.Hash))
				if err != nil || receipt.BlockHash.Hex() != tx.BlockHash {
					item.Status = "FAIL"
					item.Error = fmt.Sprintf("receipt amostra %s divergente: %v", tx.Hash, err)
					break
				}
			}
		}
		var status map[string]string
		if err := rpcClient.CallContext(ctx, &status, "txpool_status"); err != nil {
			item.Status = "FAIL"
			item.Error = fmt.Sprintf("txpool_status: %v", err)
		} else {
			item.Pending = status["pending"]
			item.Queued = status["queued"]
			if item.Pending != "0x0" || item.Queued != "0x0" {
				item.Status = "FAIL"
				item.Error = fmt.Sprintf("txpool não vazio: pending=%s queued=%s", item.Pending, item.Queued)
			}
		}
		head, err := ethClient.BlockNumber(ctx)
		if err == nil {
			item.Height = head
		}
		rpcClient.Close()
		results = append(results, item)
	}
	return results
}

func addDelta(values map[common.Address]*big.Int, address common.Address, delta *big.Int) {
	if values[address] == nil {
		values[address] = new(big.Int)
	}
	values[address].Add(values[address], delta)
}

func addRole(roles map[common.Address]map[string]bool, address common.Address, role string) {
	if roles[address] == nil {
		roles[address] = make(map[string]bool)
	}
	roles[address][role] = true
}

func roleString(roles map[string]bool) string {
	var list []string
	for role := range roles {
		list = append(list, role)
	}
	sort.Strings(list)
	return strings.Join(list, "+")
}

func mustBig(value string) *big.Int {
	result, ok := new(big.Int).SetString(value, 10)
	if !ok {
		panic("invalid internal big integer")
	}
	return result
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
	fmt.Fprintln(file, "# Auditoria de carga e mempool — Rabbit Chain")
	fmt.Fprintln(file)
	fmt.Fprintf(file, "**Status: %s**\n\n", report.Status)
	fmt.Fprintf(file, "- Versão: `%s`\n", report.AuditVersion)
	fmt.Fprintf(file, "- Blocos: `%d` até `%d`\n", report.StartBlock, report.FinalBlock)
	fmt.Fprintf(file, "- Lotes: `%d`\n", report.Rounds)
	fmt.Fprintf(file, "- Transações aprovadas: `%d/%d`\n", report.TransactionsPassed, report.TransactionsSent)
	fmt.Fprintf(file, "- Blocos de inclusão: `%d`\n", len(report.Blocks))
	fmt.Fprintf(file, "- Valor total: `%s wei`\n", report.ValueWei)
	fmt.Fprintf(file, "- Taxas totais: `%s wei`\n", report.FeesWei)
	fmt.Fprintf(file, "- Tips totais: `%s wei`\n", report.TipsWei)
	fmt.Fprintf(file, "- Burn total: `%s wei`\n", report.BurnWei)
	fmt.Fprintf(file, "- Nós aprovados: `%d/%d`\n", passingNodes(report.Nodes), len(report.Nodes))
	fmt.Fprintln(file)
	fmt.Fprintln(file, "## Blocos")
	fmt.Fprintln(file)
	fmt.Fprintln(file, "| Bloco | Hash | Producer | Transações | Auditadas | Exclusivo |")
	fmt.Fprintln(file, "| ---: | --- | --- | ---: | ---: | --- |")
	for _, block := range report.Blocks {
		fmt.Fprintf(file, "| %d | `%s` | `%s` | %d | %d | %t |\n", block.Number, block.Hash, block.Producer, block.Transactions, block.AuditedTxs, block.AllTransactions)
	}
	fmt.Fprintln(file)
	fmt.Fprintln(file, "## Transações inválidas")
	fmt.Fprintln(file)
	for _, test := range report.InvalidTests {
		fmt.Fprintf(file, "- **%s** — %s (`%s`)\n", test.Status, test.Name, test.Error)
	}
	fmt.Fprintln(file)
	fmt.Fprintln(file, "## Saldos")
	fmt.Fprintln(file)
	fmt.Fprintln(file, "| Papel | Endereço | Esperado (wei) | Observado (wei) | Confere |")
	fmt.Fprintln(file, "| --- | --- | ---: | ---: | --- |")
	for _, balance := range report.Balances {
		fmt.Fprintf(file, "| %s | `%s` | %s | %s | %t |\n", balance.Roles, balance.Address, balance.ExpectedDelta, balance.ObservedDelta, balance.Match)
	}
	if len(report.Errors) > 0 {
		fmt.Fprintln(file)
		fmt.Fprintln(file, "## Inconsistências")
		fmt.Fprintln(file)
		for _, item := range report.Errors {
			fmt.Fprintf(file, "- %s\n", item)
		}
	}
	return nil
}
