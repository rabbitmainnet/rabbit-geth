package lqc

import (
	"errors"
	"fmt"
	"math/big"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
)

const (
	RegistryProtocolVersion      uint8  = 1
	MaxRegistryOperationLifetime uint64 = 256
)

const (
	RegistryActionRegister RegistryAction = iota + 1
	RegistryActionHeartbeat
	RegistryActionExit
)

var (
	ErrInvalidRegistryVersion   = errors.New("invalid lqc registry protocol version")
	ErrInvalidRegistryAction    = errors.New("invalid lqc registry action")
	ErrInvalidRegistryAddress   = errors.New("invalid lqc registry address")
	ErrInvalidRegistrySequence  = errors.New("invalid lqc registry sequence")
	ErrExpiredRegistryOperation = errors.New("expired lqc registry operation")
	ErrRegistryOperationTooFar  = errors.New("lqc registry operation validity is too far in the future")
	ErrInvalidRegistrySignature = errors.New("invalid lqc registry signature")
	ErrInvalidLightHashProof    = errors.New("invalid lqc lighthash proof")
	ErrParticipantNotActive     = errors.New("lqc participant is not active")
	ErrParticipantAlreadyActive = errors.New("lqc participant is already active")
)

var (
	registrySigningDomain = []byte("RABBIT-LQC-REGISTRY-V1")
	registryRootDomain    = []byte("RABBIT-LQC-REGISTRY-ROOT-V1")
	twoTo256              = new(big.Int).Lsh(big.NewInt(1), 256)
)

type RegistryAction uint8

type RegistryOperation struct {
	Version    uint8
	Action     RegistryAction
	Address    common.Address
	Sequence   uint64
	ValidUntil uint64
	ProofNonce uint64
	Signature  []byte
}

type CanonicalParticipant struct {
	Address       common.Address
	RegisteredAt  uint64
	LastHeartbeat uint64
	MissedTurns   uint64
	JailedUntil   uint64
	Sequence      uint64
	Active        bool
}

type CanonicalRegistry struct {
	entries map[common.Address]CanonicalParticipant
}

type registrySigningPayload struct {
	Domain     []byte
	Version    uint8
	ChainID    *big.Int
	Action     RegistryAction
	Address    common.Address
	Sequence   uint64
	ValidUntil uint64
	ProofNonce uint64
}

type registryRootPayload struct {
	Domain  []byte
	Entries []CanonicalParticipant
}

func NewCanonicalRegistry() *CanonicalRegistry {
	return &CanonicalRegistry{entries: make(map[common.Address]CanonicalParticipant)}
}

func (r *CanonicalRegistry) Clone() *CanonicalRegistry {
	out := NewCanonicalRegistry()
	if r == nil {
		return out
	}
	for address, participant := range r.entries {
		out.entries[address] = participant
	}
	return out
}

func (r *CanonicalRegistry) Participant(address common.Address) (CanonicalParticipant, bool) {
	if r == nil {
		return CanonicalParticipant{}, false
	}
	participant, ok := r.entries[address]
	return participant, ok
}

func (r *CanonicalRegistry) ActivatePermissionlessProducer(address common.Address, blockNumber uint64) error {
	if r == nil || address == (common.Address{}) {
		return ErrInvalidRegistryAddress
	}

	participant, exists := r.entries[address]
	if !exists {
		participant = CanonicalParticipant{
			Address:       address,
			RegisteredAt:  blockNumber,
			LastHeartbeat: blockNumber,
			Sequence:      0,
			Active:        true,
		}
	} else {
		participant.Active = true
		participant.RegisteredAt = blockNumber
		participant.LastHeartbeat = blockNumber
		participant.MissedTurns = 0
		participant.JailedUntil = 0
		participant.Sequence = 0
	}

	r.entries[address] = participant
	return nil
}

func (r *CanonicalRegistry) Participants() []CanonicalParticipant {
	if r == nil {
		return nil
	}
	out := make([]CanonicalParticipant, 0, len(r.entries))
	for _, participant := range r.entries {
		out = append(out, participant)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Address.Cmp(out[j].Address) < 0
	})
	return out
}

func (r *CanonicalRegistry) ActiveParticipants(blockNumber, heartbeatWindow, heartbeatGrace uint64) []CanonicalParticipant {
	participants := r.Participants()
	out := make([]CanonicalParticipant, 0, len(participants))
	for _, participant := range participants {
		if !participant.Active {
			continue
		}
		out = append(out, participant)
	}
	return out
}

// EligibleParticipants returns participants that may appear in the deterministic
// queue for blockNumber. Liveness is derived from actual missed production
// opportunities, not from elapsed blocks since LastHeartbeat.
//
// A signed registration included at block N becomes eligible only at
// N+1+activationDelay. Genesis bootstrap participants use sequence zero and are
// immediately eligible.
func (r *CanonicalRegistry) EligibleParticipants(blockNumber, activationDelay, heartbeatWindow, heartbeatGrace uint64) []CanonicalParticipant {
	participants := r.Participants()
	out := make([]CanonicalParticipant, 0, len(participants))
	for _, participant := range participants {
		if !participant.Active {
			continue
		}
		if participant.Sequence != 0 {
			eligibleAt, ok := checkedRegistryBlockAdd(participant.RegisteredAt, 1, activationDelay)
			if !ok || blockNumber < eligibleAt {
				continue
			}
		}
		out = append(out, participant)
	}
	return out
}

// OrderedParticipantsForBlock builds the canonical production queue.
//
// Participants under a missed-turn penalty are moved behind healthy
// participants, but remain in the queue as emergency fallbacks. This prevents
// the network from reaching a state with zero authorized producers.
func (r *CanonicalRegistry) OrderedParticipantsForBlock(parentHash common.Hash, blockNumber, activationDelay, heartbeatWindow, heartbeatGrace uint64) []HybridParticipant {
	eligible := r.EligibleParticipants(
		blockNumber,
		activationDelay,
		heartbeatWindow,
		heartbeatGrace,
	)

	ready := make([]HybridParticipant, 0, len(eligible))
	penalized := make([]HybridParticipant, 0, len(eligible))

	for _, participant := range eligible {
		candidate := HybridParticipant{
			Address:       participant.Address,
			Payout:        participant.Address,
			Bond:          big.NewInt(0),
			RegisteredAt:  participant.RegisteredAt,
			LastHeartbeat: participant.LastHeartbeat,
			JailedUntil:   participant.JailedUntil,
			MissedTurns:   participant.MissedTurns,
			Status:        ParticipantActiveCandidate,
		}

		if participant.JailedUntil > blockNumber {
			penalized = append(penalized, candidate)
		} else {
			ready = append(ready, candidate)
		}
	}

	ordered := DeterministicallyOrderParticipants(ready, parentHash, blockNumber)
	if len(penalized) > 0 {
		ordered = append(
			ordered,
			DeterministicallyOrderParticipants(penalized, parentHash, blockNumber)...,
		)
	}
	return ordered
}

func (r *CanonicalRegistry) IsEligibleParticipant(address common.Address, blockNumber, activationDelay, heartbeatWindow, heartbeatGrace uint64) bool {
	for _, participant := range r.EligibleParticipants(blockNumber, activationDelay, heartbeatWindow, heartbeatGrace) {
		if participant.Address == address {
			return true
		}
	}
	return false
}

func checkedRegistryBlockAdd(values ...uint64) (uint64, bool) {
	var total uint64
	for _, value := range values {
		if ^uint64(0)-total < value {
			return 0, false
		}
		total += value
	}
	return total, true
}

func (r *CanonicalRegistry) Root() common.Hash {
	payload := registryRootPayload{
		Domain:  registryRootDomain,
		Entries: r.Participants(),
	}
	encoded, err := rlp.EncodeToBytes(payload)
	if err != nil {
		panic(err)
	}
	return crypto.Keccak256Hash(encoded)
}

func RegistryOperationSigningHash(chainID *big.Int, operation RegistryOperation) common.Hash {
	if chainID == nil {
		chainID = new(big.Int)
	}
	payload := registrySigningPayload{
		Domain:     registrySigningDomain,
		Version:    operation.Version,
		ChainID:    new(big.Int).Set(chainID),
		Action:     operation.Action,
		Address:    operation.Address,
		Sequence:   operation.Sequence,
		ValidUntil: operation.ValidUntil,
		ProofNonce: operation.ProofNonce,
	}
	encoded, err := rlp.EncodeToBytes(payload)
	if err != nil {
		panic(err)
	}
	return crypto.Keccak256Hash(encoded)
}

func LightHashMeetsDifficulty(hash common.Hash, difficulty uint64) bool {
	if difficulty == 0 {
		return false
	}
	target := new(big.Int).Div(new(big.Int).Set(twoTo256), new(big.Int).SetUint64(difficulty))
	value := new(big.Int).SetBytes(hash[:])
	return value.Cmp(target) < 0
}

func RecoverRegistryOperationSigner(chainID *big.Int, operation RegistryOperation) (common.Address, error) {
	if len(operation.Signature) != crypto.SignatureLength {
		return common.Address{}, ErrInvalidRegistrySignature
	}
	r := new(big.Int).SetBytes(operation.Signature[:32])
	s := new(big.Int).SetBytes(operation.Signature[32:64])
	v := operation.Signature[64]
	if !crypto.ValidateSignatureValues(v, r, s, true) {
		return common.Address{}, ErrInvalidRegistrySignature
	}
	hash := RegistryOperationSigningHash(chainID, operation)
	publicKey, err := crypto.SigToPub(hash[:], operation.Signature)
	if err != nil {
		return common.Address{}, ErrInvalidRegistrySignature
	}
	return crypto.PubkeyToAddress(*publicKey), nil
}

// RegistryOperationWalletMessage returns the canonical human-readable message
// that a standard EVM wallet may sign for a registry operation. The message is
// fully domain- and chain-bound and includes every field that can affect the
// canonical operation.
func RegistryOperationWalletMessage(chainID *big.Int, operation RegistryOperation) []byte {
	if chainID == nil {
		chainID = new(big.Int)
	}
	return []byte(fmt.Sprintf(
		"Rabbit Chain LQC Registry\n"+
			"Domain: %s\n"+
			"Chain ID: %s\n"+
			"Version: %d\n"+
			"Action: %d\n"+
			"Address: %s\n"+
			"Sequence: %d\n"+
			"Valid Until: %d\n"+
			"Proof Nonce: %d",
		string(registrySigningDomain),
		chainID.String(),
		operation.Version,
		operation.Action,
		operation.Address.Hex(),
		operation.Sequence,
		operation.ValidUntil,
		operation.ProofNonce,
	))
}

// RegistryOperationWalletSigningHash applies the EIP-191 personal-sign prefix
// to the canonical Rabbit registry message. This permits standard wallets to
// authorize registration without exposing a seed phrase or private key.
func RegistryOperationWalletSigningHash(chainID *big.Int, operation RegistryOperation) common.Hash {
	message := RegistryOperationWalletMessage(chainID, operation)
	prefix := []byte(fmt.Sprintf("\x19Ethereum Signed Message:\n%d", len(message)))
	return crypto.Keccak256Hash(prefix, message)
}

// RecoverRegistryOperationWalletSigner recovers a signer from an EIP-191
// personal-sign signature. Wallets commonly return V as 27/28, while go-ethereum
// recovery expects 0/1, so both encodings are accepted.
func RecoverRegistryOperationWalletSigner(chainID *big.Int, operation RegistryOperation) (common.Address, error) {
	if len(operation.Signature) != crypto.SignatureLength {
		return common.Address{}, ErrInvalidRegistrySignature
	}
	signature := append([]byte(nil), operation.Signature...)
	if signature[64] >= 27 {
		signature[64] -= 27
	}
	if signature[64] > 1 {
		return common.Address{}, ErrInvalidRegistrySignature
	}
	r := new(big.Int).SetBytes(signature[:32])
	s := new(big.Int).SetBytes(signature[32:64])
	v := signature[64]
	if !crypto.ValidateSignatureValues(v, r, s, true) {
		return common.Address{}, ErrInvalidRegistrySignature
	}
	hash := RegistryOperationWalletSigningHash(chainID, operation)
	publicKey, err := crypto.SigToPub(hash[:], signature)
	if err != nil {
		return common.Address{}, ErrInvalidRegistrySignature
	}
	return crypto.PubkeyToAddress(*publicKey), nil
}

func ValidateRegistryOperation(chainID *big.Int, blockNumber, proofDifficulty uint64, operation RegistryOperation) error {
	if operation.Version != RegistryProtocolVersion {
		return ErrInvalidRegistryVersion
	}
	if operation.Action < RegistryActionRegister || operation.Action > RegistryActionExit {
		return ErrInvalidRegistryAction
	}
	if operation.Address == (common.Address{}) {
		return ErrInvalidRegistryAddress
	}
	if operation.Sequence == 0 {
		return ErrInvalidRegistrySequence
	}
	if operation.ValidUntil < blockNumber {
		return ErrExpiredRegistryOperation
	}
	maxValidUntil, ok := checkedRegistryBlockAdd(blockNumber, MaxRegistryOperationLifetime)
	if !ok || operation.ValidUntil > maxValidUntil {
		return ErrRegistryOperationTooFar
	}
	signer, err := RecoverRegistryOperationSigner(chainID, operation)
	if err != nil || signer != operation.Address {
		signer, err = RecoverRegistryOperationWalletSigner(chainID, operation)
		if err != nil || signer != operation.Address {
			return ErrInvalidRegistrySignature
		}
	}
	if operation.Action == RegistryActionRegister {
		hash := RegistryOperationSigningHash(chainID, operation)
		if !LightHashMeetsDifficulty(hash, proofDifficulty) {
			return ErrInvalidLightHashProof
		}
	}
	return nil
}

func (r *CanonicalRegistry) ApplyOperation(chainID *big.Int, blockNumber, proofDifficulty uint64, operation RegistryOperation) error {
	if r == nil {
		return ErrInvalidRegistryAddress
	}
	if err := ValidateRegistryOperation(chainID, blockNumber, proofDifficulty, operation); err != nil {
		return err
	}

	participant, exists := r.entries[operation.Address]
	wantSequence := uint64(1)
	if exists {
		wantSequence = participant.Sequence + 1
		if wantSequence == 0 {
			return ErrInvalidRegistrySequence
		}
	}
	if operation.Sequence != wantSequence {
		return ErrInvalidRegistrySequence
	}

	switch operation.Action {
	case RegistryActionRegister:
		if exists && participant.Active {
			return ErrParticipantAlreadyActive
		}
		participant = CanonicalParticipant{
			Address:       operation.Address,
			RegisteredAt:  blockNumber,
			LastHeartbeat: blockNumber,
			Sequence:      operation.Sequence,
			Active:        true,
		}
	case RegistryActionHeartbeat:
		if !exists || !participant.Active {
			return ErrParticipantNotActive
		}
		participant.LastHeartbeat = blockNumber
		participant.Sequence = operation.Sequence
	case RegistryActionExit:
		if !exists || !participant.Active {
			return ErrParticipantNotActive
		}
		participant.Sequence = operation.Sequence
		participant.Active = false
	}

	r.entries[operation.Address] = participant
	return nil
}

func (r *CanonicalRegistry) MarkProducerHeartbeat(address common.Address, blockNumber uint64) error {
	if r == nil {
		return ErrParticipantNotActive
	}
	participant, exists := r.entries[address]
	if !exists || !participant.Active {
		return ErrParticipantNotActive
	}
	participant.LastHeartbeat = blockNumber
	participant.MissedTurns = 0
	participant.JailedUntil = 0
	r.entries[address] = participant
	return nil
}

// ApplyMissedTurn records a production opportunity that elapsed before a
// fallback producer published the canonical block.
//
// Penalized participants remain available at the end of the queue so that a
// temporary penalty can never make the chain unrecoverable.
func (r *CanonicalRegistry) ApplyMissedTurn(address common.Address, blockNumber, maxMissedTurns, jailBlocks uint64) error {
	if r == nil {
		return ErrParticipantNotActive
	}
	participant, exists := r.entries[address]
	if !exists || !participant.Active {
		return ErrParticipantNotActive
	}

	// Already penalized: do not continuously extend the penalty while the
	// participant is serving only as an emergency fallback.
	if participant.JailedUntil > blockNumber {
		return nil
	}

	if maxMissedTurns == 0 {
		maxMissedTurns = 3
	}
	if jailBlocks == 0 {
		jailBlocks = 256
	}

	if participant.MissedTurns != ^uint64(0) {
		participant.MissedTurns++
	}

	if participant.MissedTurns >= maxMissedTurns {
		participant.MissedTurns = 0
		until, ok := checkedRegistryBlockAdd(blockNumber, jailBlocks)
		if !ok {
			until = ^uint64(0)
		}
		participant.JailedUntil = until
	}

	r.entries[address] = participant
	return nil
}
