package main

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math/big"
	"os"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rpc"
)

const clientVersion = "rabbit-registry/1.0.1"

type options struct {
	rpcURL       string
	rawKeyPath   string
	keystorePath string
	passwordPath string
	action       string
	validFor     uint64
	timeout      time.Duration
	dryRun       bool
	jsonPath     string
}

type registryParameters struct {
	ChainID              *hexutil.Big   `json:"chainId"`
	ActivationBlock      hexutil.Uint64 `json:"activationBlock"`
	CurrentBlock         hexutil.Uint64 `json:"currentBlock"`
	NextBlock            hexutil.Uint64 `json:"nextBlock"`
	ProofDifficulty      hexutil.Uint64 `json:"proofDifficulty"`
	ActivationDelay      hexutil.Uint64 `json:"activationDelay"`
	HeartbeatWindow      hexutil.Uint64 `json:"heartbeatWindow"`
	HeartbeatGrace       hexutil.Uint64 `json:"heartbeatGrace"`
	MaxOperationLifetime hexutil.Uint64 `json:"maxOperationLifetime"`
	PoolCapacity         hexutil.Uint64 `json:"poolCapacity"`
	RegistryRoot         common.Hash    `json:"registryRoot"`
	ParticipantCount     hexutil.Uint64 `json:"participantCount"`
	ActiveForNextBlock   bool           `json:"activeForNextBlock"`
}

type registryParticipant struct {
	Address        common.Address `json:"address"`
	Exists         bool           `json:"exists"`
	Active         bool           `json:"active"`
	EligibleNext   bool           `json:"eligibleNext"`
	CanonicalBlock hexutil.Uint64 `json:"canonicalBlock"`
	RegistryRoot   common.Hash    `json:"registryRoot"`
	RegisteredAt   hexutil.Uint64 `json:"registeredAt"`
	LastHeartbeat  hexutil.Uint64 `json:"lastHeartbeat"`
	Sequence       hexutil.Uint64 `json:"sequence"`
}

type registryOperationArgs struct {
	Version    hexutil.Uint64 `json:"version"`
	Action     hexutil.Uint64 `json:"action"`
	Address    common.Address `json:"address"`
	Sequence   hexutil.Uint64 `json:"sequence"`
	ValidUntil hexutil.Uint64 `json:"validUntil"`
	ProofNonce hexutil.Uint64 `json:"proofNonce"`
	Signature  hexutil.Bytes  `json:"signature"`
}

type result struct {
	ClientVersion        string         `json:"clientVersion"`
	Status               string         `json:"status"`
	Submitted            bool           `json:"submitted"`
	OperationHash        common.Hash    `json:"operationHash"`
	Action               string         `json:"action"`
	Address              common.Address `json:"address"`
	Sequence             uint64         `json:"sequence"`
	ValidUntil           uint64         `json:"validUntil"`
	ProofNonce           uint64         `json:"proofNonce"`
	ProofAttempts        uint64         `json:"proofAttempts"`
	ProofDifficulty      uint64         `json:"proofDifficulty"`
	CurrentBlock         uint64         `json:"currentBlock"`
	NextBlock            uint64         `json:"nextBlock"`
	RegistryRoot         common.Hash    `json:"registryRoot"`
	ParticipantExisted   bool           `json:"participantExisted"`
	ParticipantWasActive bool           `json:"participantWasActive"`
}

func main() {
	opts := parseFlags()
	ctx, cancel := context.WithTimeout(context.Background(), opts.timeout)
	defer cancel()

	output, err := run(ctx, opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ERRO:", err)
		os.Exit(1)
	}
	if opts.jsonPath != "" {
		if err := writeJSON(opts.jsonPath, output); err != nil {
			fmt.Fprintln(os.Stderr, "ERRO ao gravar JSON:", err)
			os.Exit(1)
		}
	}
	fmt.Println("OPERAÇÃO DO CADASTRO LQC PREPARADA")
	fmt.Println("Status:", output.Status)
	fmt.Println("Ação:", output.Action)
	fmt.Println("Endereço:", output.Address.Hex())
	fmt.Println("Hash:", output.OperationHash.Hex())
	fmt.Printf("Sequência: %d | válida até: %d\n", output.Sequence, output.ValidUntil)
	if output.Action == "REGISTER" {
		fmt.Printf("LightHash: nonce %d | tentativas %d | dificuldade %d\n", output.ProofNonce, output.ProofAttempts, output.ProofDifficulty)
	}
}

func parseFlags() options {
	var opts options
	flag.StringVar(&opts.rpcURL, "rpc", "", "endpoint RPC HTTP, WebSocket ou IPC")
	flag.StringVar(&opts.rawKeyPath, "key", "", "arquivo de chave ECDSA bruta (64 caracteres hexadecimais)")
	flag.StringVar(&opts.keystorePath, "keystore", "", "arquivo JSON de keystore do geth")
	flag.StringVar(&opts.passwordPath, "password-file", "", "arquivo que contém a senha do keystore")
	flag.StringVar(&opts.action, "action", "register", "operação: register, heartbeat ou exit")
	flag.Uint64Var(&opts.validFor, "valid-for", 64, "quantidade de blocos de validade (1..256)")
	flag.DurationVar(&opts.timeout, "timeout", 2*time.Minute, "tempo máximo para RPC e prova LightHash")
	flag.BoolVar(&opts.dryRun, "dry-run", false, "assina e valida localmente sem enviar")
	flag.StringVar(&opts.jsonPath, "json", "", "arquivo JSON opcional com o resultado")
	flag.Parse()
	return opts
}

func run(ctx context.Context, opts options) (*result, error) {
	if strings.TrimSpace(opts.rpcURL) == "" {
		return nil, errors.New("informe --rpc")
	}
	client, err := rpc.DialContext(ctx, opts.rpcURL)
	if err != nil {
		return nil, fmt.Errorf("conectar ao RPC: %w", err)
	}
	defer client.Close()

	privateKey, err := loadPrivateKey(opts)
	if err != nil {
		return nil, err
	}
	defer clearPrivateKey(privateKey)
	address := crypto.PubkeyToAddress(privateKey.PublicKey)

	parameters, participant, err := fetchSigningState(ctx, client, address)
	if err != nil {
		return nil, err
	}
	action, err := parseAction(opts.action)
	if err != nil {
		return nil, err
	}
	operation, attempts, err := buildOperation(ctx, parameters, participant, action, opts.validFor, privateKey)
	if err != nil {
		return nil, err
	}
	chainID := new(big.Int).Set((*big.Int)(parameters.ChainID))
	operationHash := lqc.RegistryOperationHash(chainID, operation)
	submitted := false
	if !opts.dryRun {
		var acceptedHash common.Hash
		if err := client.CallContext(ctx, &acceptedHash, "lqc_submitRegistryOperation", operationArgs(operation)); err != nil {
			return nil, fmt.Errorf("enviar operação %s: %w", operationHash.Hex(), err)
		}
		if acceptedHash != operationHash {
			return nil, fmt.Errorf("RPC retornou hash %s, esperado %s", acceptedHash.Hex(), operationHash.Hex())
		}
		submitted = true
	}

	status := "DRY-RUN"
	if submitted {
		status = "SUBMITTED"
	}
	return &result{
		ClientVersion:        clientVersion,
		Status:               status,
		Submitted:            submitted,
		OperationHash:        operationHash,
		Action:               actionName(action),
		Address:              address,
		Sequence:             operation.Sequence,
		ValidUntil:           operation.ValidUntil,
		ProofNonce:           operation.ProofNonce,
		ProofAttempts:        attempts,
		ProofDifficulty:      uint64(parameters.ProofDifficulty),
		CurrentBlock:         uint64(parameters.CurrentBlock),
		NextBlock:            uint64(parameters.NextBlock),
		RegistryRoot:         parameters.RegistryRoot,
		ParticipantExisted:   participant.Exists,
		ParticipantWasActive: participant.Active,
	}, nil
}

// fetchSigningState retries if a block is produced between the two read-only
// RPC calls. This prevents signing with a participant sequence from one head
// and protocol metadata from another.
func fetchSigningState(ctx context.Context, client *rpc.Client, address common.Address) (registryParameters, registryParticipant, error) {
	for attempt := 0; attempt < 4; attempt++ {
		var parameters registryParameters
		if err := client.CallContext(ctx, &parameters, "lqc_registryParameters"); err != nil {
			return registryParameters{}, registryParticipant{}, fmt.Errorf("consultar parâmetros do cadastro: %w", err)
		}
		if parameters.ChainID == nil || (*big.Int)(parameters.ChainID).Sign() <= 0 {
			return registryParameters{}, registryParticipant{}, errors.New("RPC retornou chain ID inválido")
		}
		if !parameters.ActiveForNextBlock {
			return registryParameters{}, registryParticipant{}, fmt.Errorf("cadastro canônico ainda não está ativo para o próximo bloco %d", uint64(parameters.NextBlock))
		}
		if uint64(parameters.ProofDifficulty) == 0 || uint64(parameters.MaxOperationLifetime) == 0 {
			return registryParameters{}, registryParticipant{}, errors.New("RPC retornou parâmetros inseguros do cadastro")
		}
		var participant registryParticipant
		if err := client.CallContext(ctx, &participant, "lqc_registryParticipant", address); err != nil {
			return registryParameters{}, registryParticipant{}, fmt.Errorf("consultar participante %s: %w", address.Hex(), err)
		}
		if uint64(participant.CanonicalBlock) == uint64(parameters.CurrentBlock) && participant.RegistryRoot == parameters.RegistryRoot {
			return parameters, participant, nil
		}
		select {
		case <-ctx.Done():
			return registryParameters{}, registryParticipant{}, ctx.Err()
		default:
		}
	}
	return registryParameters{}, registryParticipant{}, errors.New("a cabeça canônica mudou repetidamente durante a consulta; tente novamente")
}

func buildOperation(ctx context.Context, parameters registryParameters, participant registryParticipant, action lqc.RegistryAction, validFor uint64, privateKey *ecdsa.PrivateKey) (lqc.RegistryOperation, uint64, error) {
	if privateKey == nil {
		return lqc.RegistryOperation{}, 0, errors.New("chave privada ausente")
	}
	maxLifetime := uint64(parameters.MaxOperationLifetime)
	if validFor == 0 || validFor > maxLifetime {
		return lqc.RegistryOperation{}, 0, fmt.Errorf("--valid-for deve estar entre 1 e %d", maxLifetime)
	}
	nextBlock := uint64(parameters.NextBlock)
	validityOffset := validFor - 1
	if ^uint64(0)-nextBlock < validityOffset {
		return lqc.RegistryOperation{}, 0, errors.New("altura de validade excede uint64")
	}
	address := crypto.PubkeyToAddress(privateKey.PublicKey)
	sequence, err := nextSequence(participant, action)
	if err != nil {
		return lqc.RegistryOperation{}, 0, err
	}
	operation := lqc.RegistryOperation{
		Version:    lqc.RegistryProtocolVersion,
		Action:     action,
		Address:    address,
		Sequence:   sequence,
		ValidUntil: nextBlock + validityOffset,
	}
	chainID := new(big.Int).Set((*big.Int)(parameters.ChainID))
	attempts := uint64(0)
	if action == lqc.RegistryActionRegister {
		operation.ProofNonce, attempts, err = findLightHashNonce(ctx, chainID, operation, uint64(parameters.ProofDifficulty))
		if err != nil {
			return lqc.RegistryOperation{}, attempts, err
		}
	}
	hash := lqc.RegistryOperationSigningHash(chainID, operation)
	operation.Signature, err = crypto.Sign(hash[:], privateKey)
	if err != nil {
		return lqc.RegistryOperation{}, attempts, fmt.Errorf("assinar operação: %w", err)
	}
	if err := lqc.ValidateRegistryOperation(chainID, nextBlock, uint64(parameters.ProofDifficulty), operation); err != nil {
		return lqc.RegistryOperation{}, attempts, fmt.Errorf("autoverificação da operação: %w", err)
	}
	return operation, attempts, nil
}

func findLightHashNonce(ctx context.Context, chainID *big.Int, operation lqc.RegistryOperation, difficulty uint64) (uint64, uint64, error) {
	if difficulty == 0 {
		return 0, 0, errors.New("dificuldade LightHash zero")
	}
	for nonce := uint64(0); ; nonce++ {
		operation.ProofNonce = nonce
		hash := lqc.RegistryOperationSigningHash(chainID, operation)
		if lqc.LightHashMeetsDifficulty(hash, difficulty) {
			return nonce, nonce + 1, nil
		}
		if nonce == ^uint64(0) {
			return 0, ^uint64(0), errors.New("espaço de nonce LightHash esgotado")
		}
		if nonce&4095 == 4095 {
			select {
			case <-ctx.Done():
				return 0, nonce + 1, fmt.Errorf("prova LightHash interrompida: %w", ctx.Err())
			default:
			}
		}
	}
}

func nextSequence(participant registryParticipant, action lqc.RegistryAction) (uint64, error) {
	switch action {
	case lqc.RegistryActionRegister:
		if participant.Exists && participant.Active {
			return 0, errors.New("participante já está ativo; use heartbeat ou exit")
		}
	case lqc.RegistryActionHeartbeat, lqc.RegistryActionExit:
		if !participant.Exists || !participant.Active {
			return 0, errors.New("participante não está ativo; use register")
		}
	default:
		return 0, lqc.ErrInvalidRegistryAction
	}
	if !participant.Exists {
		return 1, nil
	}
	sequence := uint64(participant.Sequence)
	if sequence == ^uint64(0) {
		return 0, lqc.ErrInvalidRegistrySequence
	}
	return sequence + 1, nil
}

func parseAction(value string) (lqc.RegistryAction, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "register":
		return lqc.RegistryActionRegister, nil
	case "heartbeat":
		return lqc.RegistryActionHeartbeat, nil
	case "exit":
		return lqc.RegistryActionExit, nil
	default:
		return 0, fmt.Errorf("ação inválida %q; use register, heartbeat ou exit", value)
	}
}

func actionName(action lqc.RegistryAction) string {
	switch action {
	case lqc.RegistryActionRegister:
		return "REGISTER"
	case lqc.RegistryActionHeartbeat:
		return "HEARTBEAT"
	case lqc.RegistryActionExit:
		return "EXIT"
	default:
		return "UNKNOWN"
	}
}

func operationArgs(operation lqc.RegistryOperation) registryOperationArgs {
	return registryOperationArgs{
		Version:    hexutil.Uint64(operation.Version),
		Action:     hexutil.Uint64(operation.Action),
		Address:    operation.Address,
		Sequence:   hexutil.Uint64(operation.Sequence),
		ValidUntil: hexutil.Uint64(operation.ValidUntil),
		ProofNonce: hexutil.Uint64(operation.ProofNonce),
		Signature:  append(hexutil.Bytes(nil), operation.Signature...),
	}
}

func loadPrivateKey(opts options) (*ecdsa.PrivateKey, error) {
	rawPath := strings.TrimSpace(opts.rawKeyPath)
	storePath := strings.TrimSpace(opts.keystorePath)
	if (rawPath == "") == (storePath == "") {
		return nil, errors.New("informe exatamente um entre --key e --keystore")
	}
	if rawPath != "" {
		if err := requirePrivateFile(rawPath, "chave privada"); err != nil {
			return nil, err
		}
		key, err := crypto.LoadECDSA(rawPath)
		if err != nil {
			return nil, fmt.Errorf("carregar chave privada: %w", err)
		}
		return key, nil
	}
	passwordPath := strings.TrimSpace(opts.passwordPath)
	if passwordPath == "" {
		return nil, errors.New("--keystore exige --password-file")
	}
	if err := requirePrivateFile(passwordPath, "arquivo de senha"); err != nil {
		return nil, err
	}
	keyJSON, err := os.ReadFile(storePath)
	if err != nil {
		return nil, fmt.Errorf("ler keystore: %w", err)
	}
	passwordBytes, err := os.ReadFile(passwordPath)
	if err != nil {
		return nil, fmt.Errorf("ler arquivo de senha: %w", err)
	}
	password := strings.TrimRight(string(passwordBytes), "\r\n")
	for index := range passwordBytes {
		passwordBytes[index] = 0
	}
	if password == "" {
		return nil, errors.New("arquivo de senha vazio")
	}
	key, err := keystore.DecryptKey(keyJSON, password)
	password = ""
	if err != nil {
		return nil, fmt.Errorf("descriptografar keystore: %w", err)
	}
	return key.PrivateKey, nil
}

func requirePrivateFile(path, label string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("consultar %s: %w", label, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s não é arquivo regular: %s", label, path)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("%s tem permissões inseguras %04o; execute chmod 600 %q", label, info.Mode().Perm(), path)
	}
	return nil
}

func clearPrivateKey(key *ecdsa.PrivateKey) {
	if key != nil && key.D != nil {
		key.D.SetInt64(0)
	}
}

func writeJSON(path string, value any) error {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	return os.WriteFile(path, encoded, 0o600)
}
