package main

import (
	"crypto/ecdsa"
	"encoding/binary"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/crypto"
)

const auditorVersion = "rabbit-lqc-sybil-auditor/2.0.0"

type options struct {
	honest         int
	sybilScenarios string
	blocks         uint64
	fallbacks      uint64
	committeeMin   uint64
	committeeMax   uint64
	difficulty     uint64
	proofSamples   int
	chainID        uint64
	outputDir      string
}

type proofBenchmark struct {
	Samples                    int     `json:"samples"`
	Difficulty                 uint64  `json:"difficulty"`
	Attempts                   uint64  `json:"attempts"`
	ElapsedMilliseconds        int64   `json:"elapsedMilliseconds"`
	HashesPerSecond            float64 `json:"hashesPerSecond"`
	ExpectedHashesPerIdentity  uint64  `json:"expectedHashesPerIdentity"`
	ValidatedSignedOperations  int     `json:"validatedSignedOperations"`
	AppliedCanonicalOperations int     `json:"appliedCanonicalOperations"`
}

type scenarioResult struct {
	HonestIdentities                int     `json:"honestIdentities"`
	AttackerIdentities              int     `json:"attackerIdentities"`
	TotalIdentities                 int     `json:"totalIdentities"`
	Blocks                          uint64  `json:"blocks"`
	CommitteeSize                   uint64  `json:"committeeSize"`
	TheoreticalAttackerSharePercent float64 `json:"theoreticalAttackerSharePercent"`
	ProducerWins                    uint64  `json:"producerWins"`
	ProducerSharePercent            float64 `json:"producerSharePercent"`
	FallbackSeats                   uint64  `json:"fallbackSeats"`
	AttackerFallbackSeats           uint64  `json:"attackerFallbackSeats"`
	FallbackSharePercent            float64 `json:"fallbackSharePercent"`
	CommitteeSeats                  uint64  `json:"committeeSeats"`
	AttackerCommitteeSeats          uint64  `json:"attackerCommitteeSeats"`
	CommitteeSharePercent           float64 `json:"committeeSharePercent"`
	CommitteeMajorityBlocks         uint64  `json:"committeeMajorityBlocks"`
	CommitteeMajorityPercent        float64 `json:"committeeMajorityPercent"`
	ProducerAndFallbackTakeovers    uint64  `json:"producerAndFallbackTakeovers"`
	ProducerAndFallbackPercent      float64 `json:"producerAndFallbackPercent"`
	ExpectedLightHashAttempts       uint64  `json:"expectedLightHashAttempts"`
	EstimatedProofSeconds           float64 `json:"estimatedProofSeconds"`
	DominatesProducerSelection      bool    `json:"dominatesProducerSelection"`
	DominatesCommittee              bool    `json:"dominatesCommittee"`
}

type report struct {
	AuditorVersion  string           `json:"auditorVersion"`
	GeneratedAt     string           `json:"generatedAt"`
	AuditExecution  string           `json:"auditExecution"`
	SybilResistance string           `json:"sybilResistance"`
	LaunchGate      string           `json:"launchGate"`
	Method          string           `json:"method"`
	ChainID         uint64           `json:"chainId"`
	FallbackCount   uint64           `json:"fallbackCount"`
	CommitteeMin    uint64           `json:"committeeMin"`
	CommitteeMax    uint64           `json:"committeeMax"`
	Proof           proofBenchmark   `json:"proofBenchmark"`
	Scenarios       []scenarioResult `json:"scenarios"`
	Findings        []string         `json:"findings"`
}

func main() {
	opts := parseFlags()
	if err := run(opts); err != nil {
		fmt.Fprintln(os.Stderr, "ERRO:", err)
		os.Exit(1)
	}
}

func parseFlags() options {
	var opts options
	flag.IntVar(&opts.honest, "honest", 20, "quantidade de identidades honestas")
	flag.StringVar(&opts.sybilScenarios, "sybils", "1,10,100,1000,5000", "cenários de identidades do atacante")
	flag.Uint64Var(&opts.blocks, "blocks", 2000, "blocos determinísticos por cenário")
	flag.Uint64Var(&opts.fallbacks, "fallbacks", 5, "quantidade de fallbacks LCQ")
	flag.Uint64Var(&opts.committeeMin, "committee-min", 32, "committee mínimo")
	flag.Uint64Var(&opts.committeeMax, "committee-max", 128, "committee máximo")
	flag.Uint64Var(&opts.difficulty, "difficulty", 100000, "dificuldade LightHash")
	flag.IntVar(&opts.proofSamples, "proof-samples", 3, "provas LightHash reais a executar")
	flag.Uint64Var(&opts.chainID, "chain-id", 928, "chain ID Rabbit")
	flag.StringVar(&opts.outputDir, "output", "", "diretório para relatorio.json e resumo.md")
	flag.Parse()
	return opts
}

func run(opts options) error {
	if opts.honest <= 0 || opts.blocks == 0 || opts.difficulty == 0 || opts.proofSamples <= 0 {
		return errors.New("honest, blocks, difficulty e proof-samples devem ser maiores que zero")
	}
	if opts.committeeMax > 0 && opts.committeeMin > opts.committeeMax {
		return errors.New("committee-min não pode exceder committee-max")
	}
	sybils, err := parseScenarios(opts.sybilScenarios)
	if err != nil {
		return err
	}

	proof, err := benchmarkProofs(opts)
	if err != nil {
		return fmt.Errorf("benchmark de LightHash: %w", err)
	}

	results := make([]scenarioResult, 0, len(sybils))
	securityFailed := false
	for _, count := range sybils {
		result := analyzeScenario(opts, count, proof.HashesPerSecond)
		results = append(results, result)
		if result.DominatesProducerSelection || result.DominatesCommittee {
			securityFailed = true
		}
	}

	resistance := "PASS"
	gate := "PASS"
	findings := []string{
		"Registro de endereço não cria vaga de producer, fallback ou committee.",
		"Producer, fallbacks e committee são atribuídos por WorkSeat canônico; endereços repetidos ou divididos não alteram a quantidade de trabalho.",
		"Cada cenário mantém exatamente o mesmo conjunto de ticket hashes do atacante e varia somente a quantidade de identidades controladas.",
	}
	if securityFailed {
		resistance = "FAIL"
		gate = "BLOCKED"
		findings = append(findings, "Ao menos um cenário dá ao atacante maioria de producer selection ou de committee; a mainnet não deve ser lançada com esta regra.")
	}

	rep := report{
		AuditorVersion:  auditorVersion,
		GeneratedAt:     time.Now().UTC().Format(time.RFC3339),
		AuditExecution:  "PASS",
		SybilResistance: resistance,
		LaunchGate:      gate,
		Method:          "consensus/lqc.BuildWorkSelectionV1 com trabalho fixo dividido entre identidades + operações REGISTER secp256k1/LightHash reais",
		ChainID:         opts.chainID,
		FallbackCount:   opts.fallbacks,
		CommitteeMin:    opts.committeeMin,
		CommitteeMax:    opts.committeeMax,
		Proof:           proof,
		Scenarios:       results,
		Findings:        findings,
	}

	printReport(rep)
	if opts.outputDir != "" {
		if err := writeReport(opts.outputDir, rep); err != nil {
			return err
		}
	}
	if securityFailed {
		return errors.New("Sybil launch gate blocked")
	}
	return nil
}

func parseScenarios(value string) ([]int, error) {
	seen := make(map[int]bool)
	var out []int
	for _, field := range strings.Split(value, ",") {
		count, err := strconv.Atoi(strings.TrimSpace(field))
		if err != nil || count <= 0 {
			return nil, fmt.Errorf("cenário Sybil inválido %q", field)
		}
		if !seen[count] {
			seen[count] = true
			out = append(out, count)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("informe ao menos um cenário Sybil")
	}
	return out, nil
}

func analyzeScenario(opts options, sybils int, hashesPerSecond float64) scenarioResult {
	attackerSeats := opts.honest / 4
	if attackerSeats < 1 {
		attackerSeats = 1
	}
	seats := make([]lqc.WorkSeatV1, 0, opts.honest+attackerSeats)
	attacker := make(map[common.Address]bool, sybils)
	for i := 0; i < opts.honest; i++ {
		participant := auditParticipant("honest", i)
		seats = append(seats, lqc.WorkSeatV1{
			TicketHash:  crypto.Keccak256Hash([]byte("RABBIT-SYBIL-HONEST-WORK"), uint64Bytes(uint64(i))),
			Participant: participant.Address,
		})
	}
	attackerAddresses := make([]common.Address, 0, sybils)
	for i := 0; i < sybils; i++ {
		participant := auditParticipant("attacker", i)
		attacker[participant.Address] = true
		attackerAddresses = append(attackerAddresses, participant.Address)
	}
	for i := 0; i < attackerSeats; i++ {
		seats = append(seats, lqc.WorkSeatV1{
			TicketHash:  crypto.Keccak256Hash([]byte("RABBIT-SYBIL-ATTACKER-WORK"), uint64Bytes(uint64(i))),
			Participant: attackerAddresses[i%len(attackerAddresses)],
		})
	}

	total := uint64(len(seats))
	committeeSize := lqc.ComputeCommitteeSizeWithBounds(total, opts.committeeMin, opts.committeeMax)
	availableCommittee := uint64(0)
	if total > 1+opts.fallbacks {
		availableCommittee = total - 1 - opts.fallbacks
	}
	if committeeSize > availableCommittee {
		committeeSize = availableCommittee
	}

	result := scenarioResult{
		HonestIdentities:                opts.honest,
		AttackerIdentities:              sybils,
		TotalIdentities:                 opts.honest + sybils,
		Blocks:                          opts.blocks,
		CommitteeSize:                   committeeSize,
		TheoreticalAttackerSharePercent: percent(float64(attackerSeats), float64(len(seats))),
	}
	for block := uint64(1); block <= opts.blocks; block++ {
		selectionSeed := crypto.Keccak256Hash(
			[]byte("RABBIT-LQC-SYBIL-AUDIT-V2"),
			uint64Bytes(block),
		)
		selection, err := lqc.BuildWorkSelectionV1(
			seats,
			selectionSeed,
			opts.fallbacks,
			committeeSize,
		)
		if err != nil || selection.Producer == nil {
			panic(fmt.Sprintf("auditor WorkSeat selection failed: %v", err))
		}
		if attacker[selection.Producer.Participant] {
			result.ProducerWins++
		}
		allFrontAttacker := attacker[selection.Producer.Participant]
		for _, seat := range selection.Fallbacks {
			result.FallbackSeats++
			if attacker[seat.Participant] {
				result.AttackerFallbackSeats++
			} else {
				allFrontAttacker = false
			}
		}
		if allFrontAttacker {
			result.ProducerAndFallbackTakeovers++
		}
		attackerCommittee := uint64(0)
		for _, seat := range selection.Committee {
			result.CommitteeSeats++
			if attacker[seat.Participant] {
				result.AttackerCommitteeSeats++
				attackerCommittee++
			}
		}
		seatsThisBlock := uint64(len(selection.Committee))
		if seatsThisBlock > 0 && attackerCommittee*2 > seatsThisBlock {
			result.CommitteeMajorityBlocks++
		}
	}

	result.ProducerSharePercent = percent(float64(result.ProducerWins), float64(opts.blocks))
	result.FallbackSharePercent = percent(float64(result.AttackerFallbackSeats), float64(result.FallbackSeats))
	result.CommitteeSharePercent = percent(float64(result.AttackerCommitteeSeats), float64(result.CommitteeSeats))
	result.CommitteeMajorityPercent = percent(float64(result.CommitteeMajorityBlocks), float64(opts.blocks))
	result.ProducerAndFallbackPercent = percent(float64(result.ProducerAndFallbackTakeovers), float64(opts.blocks))
	result.ExpectedLightHashAttempts = saturatingMultiply(uint64(attackerSeats), opts.difficulty)
	if hashesPerSecond > 0 {
		result.EstimatedProofSeconds = float64(result.ExpectedLightHashAttempts) / hashesPerSecond
	}
	result.DominatesProducerSelection = result.ProducerSharePercent > 50
	result.DominatesCommittee = result.CommitteeMajorityPercent > 50
	return result
}

func benchmarkProofs(opts options) (proofBenchmark, error) {
	started := time.Now()
	chainID := new(big.Int).SetUint64(opts.chainID)
	registry := lqc.NewCanonicalRegistry()
	totalAttempts := uint64(0)
	for i := 0; i < opts.proofSamples; i++ {
		key, err := deterministicPrivateKey(i)
		if err != nil {
			return proofBenchmark{}, err
		}
		operation := lqc.RegistryOperation{
			Version:    lqc.RegistryProtocolVersion,
			Action:     lqc.RegistryActionRegister,
			Address:    crypto.PubkeyToAddress(key.PublicKey),
			Sequence:   1,
			ValidUntil: lqc.MaxRegistryOperationLifetime,
		}
		for {
			hash := lqc.RegistryOperationSigningHash(chainID, operation)
			totalAttempts++
			if lqc.LightHashMeetsDifficulty(hash, opts.difficulty) {
				operation.Signature, err = crypto.Sign(hash[:], key)
				if err != nil {
					return proofBenchmark{}, err
				}
				break
			}
			operation.ProofNonce++
		}
		if err := lqc.ValidateRegistryOperation(chainID, 1, opts.difficulty, operation); err != nil {
			return proofBenchmark{}, fmt.Errorf("operação assinada %d foi rejeitada: %w", i, err)
		}
		if err := registry.ApplyOperation(chainID, 1, opts.difficulty, operation); err != nil {
			return proofBenchmark{}, fmt.Errorf("aplicar operação canônica %d: %w", i, err)
		}
	}
	elapsed := time.Since(started)
	seconds := elapsed.Seconds()
	if seconds <= 0 {
		seconds = 1e-9
	}
	return proofBenchmark{
		Samples:                    opts.proofSamples,
		Difficulty:                 opts.difficulty,
		Attempts:                   totalAttempts,
		ElapsedMilliseconds:        elapsed.Milliseconds(),
		HashesPerSecond:            float64(totalAttempts) / seconds,
		ExpectedHashesPerIdentity:  opts.difficulty,
		ValidatedSignedOperations:  opts.proofSamples,
		AppliedCanonicalOperations: len(registry.ActiveParticipants(2, 64, 16)),
	}, nil
}

func deterministicPrivateKey(index int) (*ecdsa.PrivateKey, error) {
	for salt := 0; salt < 1024; salt++ {
		candidate := crypto.Keccak256([]byte(fmt.Sprintf("RABBIT-LQC-SYBIL-PROOF-%d-%d", index, salt)))
		key, err := crypto.ToECDSA(candidate)
		if err == nil {
			return key, nil
		}
	}
	return nil, errors.New("não foi possível derivar chave determinística de auditoria")
}

func auditParticipant(group string, index int) lqc.HybridParticipant {
	hash := crypto.Keccak256Hash([]byte(fmt.Sprintf("RABBIT-LQC-SYBIL-%s-%d", group, index)))
	address := common.BytesToAddress(hash[12:])
	return lqc.HybridParticipant{Address: address, Payout: address, Bond: new(big.Int), Status: lqc.ParticipantActiveCandidate}
}

func uint64Bytes(value uint64) []byte {
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], value)
	return encoded[:]
}

func percent(numerator, denominator float64) float64 {
	if denominator == 0 {
		return 0
	}
	return math.Round((numerator/denominator)*10000) / 100
}

func saturatingMultiply(a, b uint64) uint64 {
	if a != 0 && b > ^uint64(0)/a {
		return ^uint64(0)
	}
	return a * b
}

func printReport(rep report) {
	fmt.Println("Rabbit Chain — auditoria obrigatória de ataque Sybil contra LCQ")
	fmt.Println("Execução do auditor:", rep.AuditExecution)
	fmt.Println("Resistência Sybil atual:", rep.SybilResistance)
	fmt.Println("Gate de lançamento:", rep.LaunchGate)
	for _, scenario := range rep.Scenarios {
		fmt.Printf("Sybil %d: produtor %.2f%% | fallbacks %.2f%% | committee %.2f%% | maioria committee %.2f%%\n",
			scenario.AttackerIdentities,
			scenario.ProducerSharePercent,
			scenario.FallbackSharePercent,
			scenario.CommitteeSharePercent,
			scenario.CommitteeMajorityPercent,
		)
	}
	if rep.SybilResistance == "FAIL" {
		fmt.Println("RESULTADO: o teste funcionou e encontrou uma vulnerabilidade estrutural; NÃO lançar a mainnet ainda.")
	} else {
		fmt.Println("RESULTADO: identidades adicionais sem WorkSeats adicionais não criaram poder de consenso.")
	}
}

func writeReport(directory string, rep report) error {
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	jsonBlob, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	jsonBlob = append(jsonBlob, '\n')
	if err := os.WriteFile(filepath.Join(directory, "relatorio.json"), jsonBlob, 0o644); err != nil {
		return err
	}
	var text strings.Builder
	fmt.Fprintln(&text, "# Auditoria obrigatória de ataque Sybil — Rabbit Chain")
	fmt.Fprintln(&text)
	fmt.Fprintf(&text, "- Execução do auditor: **%s**\n", rep.AuditExecution)
	fmt.Fprintf(&text, "- Resistência Sybil do consenso atual: **%s**\n", rep.SybilResistance)
	fmt.Fprintf(&text, "- Gate da mainnet: **%s**\n", rep.LaunchGate)
	fmt.Fprintf(&text, "- Auditor: `%s`\n", rep.AuditorVersion)
	fmt.Fprintln(&text)
	fmt.Fprintln(&text, "## Resultado por cenário")
	fmt.Fprintln(&text)
	fmt.Fprintln(&text, "| Honest | Sybils | Produtor atacante | Fallbacks atacante | Committee atacante | Blocos com maioria no committee | Controle producer+fallbacks |")
	fmt.Fprintln(&text, "|---:|---:|---:|---:|---:|---:|---:|")
	for _, scenario := range rep.Scenarios {
		fmt.Fprintf(&text, "| %d | %d | %.2f%% | %.2f%% | %.2f%% | %.2f%% | %.2f%% |\n",
			scenario.HonestIdentities,
			scenario.AttackerIdentities,
			scenario.ProducerSharePercent,
			scenario.FallbackSharePercent,
			scenario.CommitteeSharePercent,
			scenario.CommitteeMajorityPercent,
			scenario.ProducerAndFallbackPercent,
		)
	}
	fmt.Fprintln(&text)
	fmt.Fprintln(&text, "## Evidência criptográfica")
	fmt.Fprintln(&text)
	fmt.Fprintf(&text, "Foram produzidas e validadas **%d** operações REGISTER reais, assinadas com secp256k1, com LightHash de dificuldade **%d**. Todas foram aceitas pelo `CanonicalRegistry`.\n", rep.Proof.ValidatedSignedOperations, rep.Proof.Difficulty)
	fmt.Fprintln(&text)
	fmt.Fprintln(&text, "## Conclusão")
	fmt.Fprintln(&text)
	for _, finding := range rep.Findings {
		fmt.Fprintf(&text, "- %s\n", finding)
	}
	if rep.LaunchGate == "BLOCKED" {
		fmt.Fprintln(&text)
		fmt.Fprintln(&text, "**MAINNET BLOQUEADA:** corrigir a resistência Sybil e repetir esta auditoria antes do lançamento.")
	}
	return os.WriteFile(filepath.Join(directory, "resumo.md"), []byte(text.String()), 0o644)
}
