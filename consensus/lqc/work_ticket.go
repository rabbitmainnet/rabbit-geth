package lqc

import (
	"bytes"
	"crypto/subtle"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"golang.org/x/crypto/argon2"
)

const (
	WorkTicketProtocolVersion     uint8  = 1
	WorkTicketMemoryKiB           uint32 = 8 * 1024
	WorkTicketIterations          uint32 = 1
	WorkTicketParallelism         uint8  = 1
	WorkTicketOutputBytes         uint32 = 32
	MaxWorkTicketsPerBlock               = 64
	WorkTicketVerificationWorkers        = 2
)

var (
	ErrInvalidWorkTicketVersion   = errors.New("invalid lqc work ticket version")
	ErrInvalidWorkTicketChain     = errors.New("invalid lqc work ticket chain")
	ErrInvalidWorkTicketAnchor    = errors.New("invalid lqc work ticket anchor")
	ErrInvalidWorkTicketAddress   = errors.New("invalid lqc work ticket address")
	ErrInvalidWorkTicketSequence  = errors.New("invalid lqc work ticket sequence")
	ErrInvalidWorkTicketPrevious  = errors.New("invalid lqc work ticket previous proof")
	ErrInvalidWorkTicketProof     = errors.New("invalid lqc work ticket proof")
	ErrInvalidWorkTicketSignature = errors.New("invalid lqc work ticket signature")
	ErrUnknownWorkTicketLane      = errors.New("unknown lqc work ticket lane")
	ErrTooManyWorkTickets         = errors.New("too many lqc work tickets")
	ErrNonCanonicalWorkTickets    = errors.New("non-canonical lqc work ticket order")
)

var (
	workTicketProofDomain     = []byte("RABBIT-LQC-WORK-TICKET-PROOF-V1")
	workTicketSaltDomain      = []byte("RABBIT-LQC-WORK-TICKET-SALT-V1")
	workTicketSigningDomain   = []byte("RABBIT-LQC-WORK-TICKET-SIGN-V1")
	workTicketLaneStateDomain = []byte("RABBIT-LQC-WORK-TICKET-LANE-V1")
)

// WorkTicket is a portable, participant-bound unit of sequential work.
//
// This file defines cryptographic primitives only. The active LQC engine does
// not yet accept tickets, select producers from them or commit them in headers.
type WorkTicket struct {
	Version     uint8
	Epoch       uint64
	Anchor      common.Hash
	Participant common.Address
	Sequence    uint64
	Previous    common.Hash
	Proof       common.Hash
	Signature   []byte
}

// WorkTicketLaneState is the minimum canonical state required to validate the
// next ticket in one participant lane.
type WorkTicketLaneState struct {
	Epoch        uint64
	NextSequence uint64
	Previous     common.Hash
}

type workTicketProofPayload struct {
	Domain      []byte
	Version     uint8
	ChainID     *big.Int
	Epoch       uint64
	Anchor      common.Hash
	Participant common.Address
	Sequence    uint64
	Previous    common.Hash
}

type workTicketSaltPayload struct {
	Domain  []byte
	ChainID *big.Int
	Epoch   uint64
	Anchor  common.Hash
}

type workTicketSigningPayload struct {
	Domain      []byte
	Version     uint8
	ChainID     *big.Int
	Epoch       uint64
	Anchor      common.Hash
	Participant common.Address
	Sequence    uint64
	Previous    common.Hash
	Proof       common.Hash
}

type workTicketLanePayload struct {
	Domain      []byte
	ChainID     *big.Int
	Epoch       uint64
	Anchor      common.Hash
	Participant common.Address
}

func validWorkTicketChainID(chainID *big.Int) bool {
	return chainID != nil && chainID.Sign() > 0
}

func workTicketChainID(chainID *big.Int) *big.Int {
	if chainID == nil {
		return new(big.Int)
	}
	return new(big.Int).Set(chainID)
}

// InitialWorkTicketPrevious derives the mandatory predecessor of sequence 1.
// A new address therefore receives no reusable proof from another network,
// epoch, anchor or participant.
func InitialWorkTicketPrevious(chainID *big.Int, anchor common.Hash, epoch uint64, participant common.Address) common.Hash {
	payload := workTicketLanePayload{
		Domain:      workTicketLaneStateDomain,
		ChainID:     workTicketChainID(chainID),
		Epoch:       epoch,
		Anchor:      anchor,
		Participant: participant,
	}
	encoded, err := rlp.EncodeToBytes(payload)
	if err != nil {
		panic(err)
	}
	return crypto.Keccak256Hash(encoded)
}

func NewWorkTicketLaneState(chainID *big.Int, anchor common.Hash, epoch uint64, participant common.Address) WorkTicketLaneState {
	return WorkTicketLaneState{
		Epoch:        epoch,
		NextSequence: 1,
		Previous:     InitialWorkTicketPrevious(chainID, anchor, epoch, participant),
	}
}

func workTicketChallenge(chainID *big.Int, ticket WorkTicket) common.Hash {
	payload := workTicketProofPayload{
		Domain:      workTicketProofDomain,
		Version:     ticket.Version,
		ChainID:     workTicketChainID(chainID),
		Epoch:       ticket.Epoch,
		Anchor:      ticket.Anchor,
		Participant: ticket.Participant,
		Sequence:    ticket.Sequence,
		Previous:    ticket.Previous,
	}
	encoded, err := rlp.EncodeToBytes(payload)
	if err != nil {
		panic(err)
	}
	return crypto.Keccak256Hash(encoded)
}

func workTicketSalt(chainID *big.Int, anchor common.Hash, epoch uint64) [16]byte {
	payload := workTicketSaltPayload{
		Domain:  workTicketSaltDomain,
		ChainID: workTicketChainID(chainID),
		Epoch:   epoch,
		Anchor:  anchor,
	}
	encoded, err := rlp.EncodeToBytes(payload)
	if err != nil {
		panic(err)
	}
	hash := crypto.Keccak256Hash(encoded)
	var salt [16]byte
	copy(salt[:], hash[:len(salt)])
	return salt
}

// GenerateWorkTicketProof is the portable reference implementation. It uses
// x/crypto Argon2id so Linux, Windows and macOS nodes calculate identical
// bytes without cgo or a platform library.
func GenerateWorkTicketProof(chainID *big.Int, ticket WorkTicket) (common.Hash, error) {
	if err := validateWorkTicketFields(chainID, ticket, false); err != nil {
		return common.Hash{}, err
	}
	challenge := workTicketChallenge(chainID, ticket)
	salt := workTicketSalt(chainID, ticket.Anchor, ticket.Epoch)
	proof := argon2.IDKey(
		challenge[:],
		salt[:],
		WorkTicketIterations,
		WorkTicketMemoryKiB,
		WorkTicketParallelism,
		WorkTicketOutputBytes,
	)
	return common.BytesToHash(proof), nil
}

func WorkTicketSigningHash(chainID *big.Int, ticket WorkTicket) common.Hash {
	payload := workTicketSigningPayload{
		Domain:      workTicketSigningDomain,
		Version:     ticket.Version,
		ChainID:     workTicketChainID(chainID),
		Epoch:       ticket.Epoch,
		Anchor:      ticket.Anchor,
		Participant: ticket.Participant,
		Sequence:    ticket.Sequence,
		Previous:    ticket.Previous,
		Proof:       ticket.Proof,
	}
	encoded, err := rlp.EncodeToBytes(payload)
	if err != nil {
		panic(err)
	}
	return crypto.Keccak256Hash(encoded)
}

func RecoverWorkTicketSigner(chainID *big.Int, ticket WorkTicket) (common.Address, error) {
	if len(ticket.Signature) != crypto.SignatureLength {
		return common.Address{}, ErrInvalidWorkTicketSignature
	}
	r := new(big.Int).SetBytes(ticket.Signature[:32])
	s := new(big.Int).SetBytes(ticket.Signature[32:64])
	v := ticket.Signature[64]
	if !crypto.ValidateSignatureValues(v, r, s, true) {
		return common.Address{}, ErrInvalidWorkTicketSignature
	}
	hash := WorkTicketSigningHash(chainID, ticket)
	publicKey, err := crypto.SigToPub(hash[:], ticket.Signature)
	if err != nil {
		return common.Address{}, ErrInvalidWorkTicketSignature
	}
	return crypto.PubkeyToAddress(*publicKey), nil
}

func validateWorkTicketStructure(ticket WorkTicket, requireProof bool) error {
	if ticket.Version != WorkTicketProtocolVersion {
		return ErrInvalidWorkTicketVersion
	}
	if ticket.Anchor == (common.Hash{}) {
		return ErrInvalidWorkTicketAnchor
	}
	if ticket.Participant == (common.Address{}) {
		return ErrInvalidWorkTicketAddress
	}
	if ticket.Sequence == 0 {
		return ErrInvalidWorkTicketSequence
	}
	if ticket.Previous == (common.Hash{}) {
		return ErrInvalidWorkTicketPrevious
	}
	if requireProof && ticket.Proof == (common.Hash{}) {
		return ErrInvalidWorkTicketProof
	}
	if requireProof {
		if len(ticket.Signature) != crypto.SignatureLength {
			return ErrInvalidWorkTicketSignature
		}
		r := new(big.Int).SetBytes(ticket.Signature[:32])
		s := new(big.Int).SetBytes(ticket.Signature[32:64])
		if !crypto.ValidateSignatureValues(ticket.Signature[64], r, s, true) {
			return ErrInvalidWorkTicketSignature
		}
	}
	return nil
}

func validateWorkTicketFields(chainID *big.Int, ticket WorkTicket, requireProof bool) error {
	if !validWorkTicketChainID(chainID) {
		return ErrInvalidWorkTicketChain
	}
	return validateWorkTicketStructure(ticket, requireProof)
}

func ValidateWorkTicketCryptography(chainID *big.Int, ticket WorkTicket) error {
	if err := validateWorkTicketFields(chainID, ticket, true); err != nil {
		return err
	}
	signer, err := RecoverWorkTicketSigner(chainID, ticket)
	if err != nil || signer != ticket.Participant {
		return ErrInvalidWorkTicketSignature
	}
	expected, err := GenerateWorkTicketProof(chainID, ticket)
	if err != nil {
		return err
	}
	if subtle.ConstantTimeCompare(expected[:], ticket.Proof[:]) != 1 {
		return ErrInvalidWorkTicketProof
	}
	return nil
}

func ValidateWorkTicket(chainID *big.Int, anchor common.Hash, epoch uint64, state WorkTicketLaneState, ticket WorkTicket) error {
	if ticket.Anchor != anchor || anchor == (common.Hash{}) || ticket.Epoch != epoch || state.Epoch != epoch {
		return ErrInvalidWorkTicketAnchor
	}
	if ticket.Sequence != state.NextSequence {
		return ErrInvalidWorkTicketSequence
	}
	if ticket.Previous != state.Previous {
		return ErrInvalidWorkTicketPrevious
	}
	return ValidateWorkTicketCryptography(chainID, ticket)
}

// CanonicalWorkTickets removes relay arrival order from consensus inputs.
// Tickets are grouped by participant and then by sequence so lane continuity
// can be verified before expensive Argon2id work begins.
func CanonicalWorkTickets(tickets []WorkTicket) []WorkTicket {
	out := make([]WorkTicket, len(tickets))
	for index, ticket := range tickets {
		out[index] = ticket
		out[index].Signature = bytes.Clone(ticket.Signature)
	}
	sort.Slice(out, func(i, j int) bool {
		left, right := out[i], out[j]
		if order := left.Participant.Cmp(right.Participant); order != 0 {
			return order < 0
		}
		if left.Sequence != right.Sequence {
			return left.Sequence < right.Sequence
		}
		if order := bytes.Compare(left.Proof[:], right.Proof[:]); order != 0 {
			return order < 0
		}
		return bytes.Compare(left.Signature, right.Signature) < 0
	})
	return out
}

func workTicketsEqual(left, right []WorkTicket) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].Version != right[index].Version ||
			left[index].Epoch != right[index].Epoch ||
			left[index].Anchor != right[index].Anchor ||
			left[index].Participant != right[index].Participant ||
			left[index].Sequence != right[index].Sequence ||
			left[index].Previous != right[index].Previous ||
			left[index].Proof != right[index].Proof ||
			!bytes.Equal(left[index].Signature, right[index].Signature) {
			return false
		}
	}
	return true
}

func cloneWorkTicketLaneStates(states map[common.Address]WorkTicketLaneState) map[common.Address]WorkTicketLaneState {
	out := make(map[common.Address]WorkTicketLaneState, len(states))
	for address, state := range states {
		out[address] = state
	}
	return out
}

// ValidateWorkTicketBatch validates canonical lane continuity first and then
// verifies independent proofs with a fixed two-worker ceiling. The caller's
// state is never mutated; state advances only when every ticket is valid.
func ValidateWorkTicketBatch(chainID *big.Int, anchor common.Hash, epoch uint64, states map[common.Address]WorkTicketLaneState, tickets []WorkTicket) (map[common.Address]WorkTicketLaneState, error) {
	if !validWorkTicketChainID(chainID) {
		return nil, ErrInvalidWorkTicketChain
	}
	if anchor == (common.Hash{}) {
		return nil, ErrInvalidWorkTicketAnchor
	}
	if len(tickets) > MaxWorkTicketsPerBlock {
		return nil, ErrTooManyWorkTickets
	}
	canonical := CanonicalWorkTickets(tickets)
	if !workTicketsEqual(canonical, tickets) {
		return nil, ErrNonCanonicalWorkTickets
	}
	next := cloneWorkTicketLaneStates(states)
	for index, ticket := range tickets {
		if err := validateWorkTicketFields(chainID, ticket, true); err != nil {
			return nil, fmt.Errorf("ticket %d: %w", index, err)
		}
		state, ok := next[ticket.Participant]
		if !ok {
			return nil, fmt.Errorf("ticket %d: %w", index, ErrUnknownWorkTicketLane)
		}
		if ticket.Anchor != anchor || ticket.Epoch != epoch || state.Epoch != epoch {
			return nil, fmt.Errorf("ticket %d: %w", index, ErrInvalidWorkTicketAnchor)
		}
		if ticket.Sequence != state.NextSequence {
			return nil, fmt.Errorf("ticket %d: %w", index, ErrInvalidWorkTicketSequence)
		}
		if ticket.Previous != state.Previous {
			return nil, fmt.Errorf("ticket %d: %w", index, ErrInvalidWorkTicketPrevious)
		}
		state.NextSequence++
		if state.NextSequence == 0 {
			return nil, fmt.Errorf("ticket %d: %w", index, ErrInvalidWorkTicketSequence)
		}
		state.Previous = ticket.Proof
		next[ticket.Participant] = state
	}

	errorsByIndex := make([]error, len(tickets))
	workerCount := WorkTicketVerificationWorkers
	if len(tickets) < workerCount {
		workerCount = len(tickets)
	}
	jobs := make(chan int)
	var wait sync.WaitGroup
	for worker := 0; worker < workerCount; worker++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for index := range jobs {
				errorsByIndex[index] = ValidateWorkTicketCryptography(chainID, tickets[index])
			}
		}()
	}
	for index := range tickets {
		jobs <- index
	}
	close(jobs)
	wait.Wait()
	for index, err := range errorsByIndex {
		if err != nil {
			return nil, fmt.Errorf("ticket %d: %w", index, err)
		}
	}
	return next, nil
}
