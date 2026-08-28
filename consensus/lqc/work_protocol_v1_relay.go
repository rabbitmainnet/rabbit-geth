package lqc

import (
	"errors"
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum/common"
)

var (
	ErrInvalidWorkRelayV1          = errors.New("invalid lqc work relay v1")
	ErrWorkRelayClaimTargetV1      = errors.New("lqc work relay claimed hash misses target v1")
	ErrWorkRelayClaimMismatchV1    = errors.New("lqc work relay recomputed hash mismatch v1")
	ErrWorkRelayVerificationBusyV1 = errors.New("lqc work relay verification budget busy v1")
	ErrWorkRelayAlreadyKnownV1     = errors.New("lqc work relay candidate already known v1")
)

// WorkRelayHasherV1 is the future runtime boundary to canonical RandomX.
//
// Implementations MUST return RandomX(input, epochKey) under Rabbit's exact
// frozen RandomX configuration. This foundation deliberately does not embed or
// activate RandomX inside consensus yet.
type WorkRelayHasherV1 func(
	epochKey common.Hash,
	input []byte,
) (common.Hash, error)

// WorkRelayEligibilityCheckV1 is a cheap caller-supplied registry/lifecycle
// check performed BEFORE expensive RandomX recomputation.
type WorkRelayEligibilityCheckV1 func(
	participant common.Address,
) error

// WorkRelayPrecheckedV1 is local transient state after cheap checks succeed.
type WorkRelayPrecheckedV1 struct {
	ChainID    *big.Int
	Epoch      uint64
	Anchor     common.Hash
	Difficulty *big.Int
	Candidate  WorkCommitCandidateV1
	EpochKey   common.Hash
	Input      []byte
}

// WorkRelayVerificationLimiterV1 bounds EXPENSIVE RandomX recomputation.
//
// This is a local non-consensus resource guard. maxInFlight is intentionally
// runtime-configurable and does not affect block validity.
type WorkRelayVerificationLimiterV1 struct {
	mu          sync.Mutex
	maxInFlight uint64
	inFlight    uint64
}

func NewWorkRelayVerificationLimiterV1(
	maxInFlight uint64,
) (*WorkRelayVerificationLimiterV1, error) {
	if maxInFlight == 0 {
		return nil, ErrInvalidWorkRelayV1
	}
	return &WorkRelayVerificationLimiterV1{
		maxInFlight: maxInFlight,
	}, nil
}

func (l *WorkRelayVerificationLimiterV1) TryAcquireV1() bool {
	if l == nil {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.inFlight >= l.maxInFlight {
		return false
	}
	l.inFlight++
	return true
}

func (l *WorkRelayVerificationLimiterV1) ReleaseV1() {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.inFlight > 0 {
		l.inFlight--
	}
}

func (l *WorkRelayVerificationLimiterV1) InFlightV1() uint64 {
	if l == nil {
		return 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.inFlight
}

// WorkCommitPoolContainsCandidateV1 checks duplicate proof and semantic ticket
// identity WITHOUT doing RandomX work.
func (p *WorkCommitPoolV1) WorkCommitPoolContainsCandidateV1(
	candidate WorkCommitCandidateV1,
) bool {
	if p == nil {
		return false
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

	if _, exists := p.byProof[candidate.ProofHash]; exists {
		return true
	}
	for _, existing := range p.byProof {
		if existing.Signed.Ticket.Epoch == candidate.Signed.Ticket.Epoch &&
			existing.Signed.Ticket.Participant == candidate.Signed.Ticket.Participant {
			return true
		}
	}

	key := workTicketSemanticKeyV3{
		Epoch:       candidate.Signed.Ticket.Epoch,
		Participant: candidate.Signed.Ticket.Participant,
		Nonce:       candidate.Signed.Ticket.Nonce,
	}
	_, exists := p.semantic[key]
	return exists
}

// PrecheckRelayedWorkV1 performs every safe cheap rejection BEFORE RandomX:
//
//   - context
//   - ticket shape / canonical secp256k1 signature shape
//   - expected commit epoch
//   - claimed proof target
//   - cryptographic ownership signature over the claimed proof hash
//   - canonical participant eligibility supplied by the caller
//
// NOTE: a malicious peer can still invent an arbitrary low claimed hash and
// sign it, so these checks cannot prove RandomX work. The global verification
// limiter below is still required to bound CPU DoS.
func PrecheckRelayedWorkV1(
	chainID *big.Int,
	expectedEpoch uint64,
	anchor common.Hash,
	difficulty *big.Int,
	candidate WorkCommitCandidateV1,
	eligibility WorkRelayEligibilityCheckV1,
) (WorkRelayPrecheckedV1, error) {
	if chainID == nil ||
		chainID.Sign() <= 0 ||
		expectedEpoch == 0 ||
		anchor == (common.Hash{}) ||
		difficulty == nil ||
		difficulty.Sign() <= 0 ||
		candidate.ProofHash == (common.Hash{}) ||
		eligibility == nil {
		return WorkRelayPrecheckedV1{}, ErrInvalidWorkRelayV1
	}

	if err := validateWorkTicketHeaderShapeV3(
		candidate.Signed,
	); err != nil {
		return WorkRelayPrecheckedV1{}, err
	}

	if candidate.Signed.Ticket.Epoch != expectedEpoch {
		return WorkRelayPrecheckedV1{}, ErrWorkCommitEpochMismatchV1
	}

	meets, err := RandomXWorkHashMeetsTargetV1(
		candidate.ProofHash,
		difficulty,
	)
	if err != nil {
		return WorkRelayPrecheckedV1{}, err
	}
	if !meets {
		return WorkRelayPrecheckedV1{}, ErrWorkRelayClaimTargetV1
	}

	if err := VerifyRandomXWorkSignatureV1(
		chainID,
		anchor,
		candidate.Signed,
		candidate.ProofHash,
	); err != nil {
		return WorkRelayPrecheckedV1{}, err
	}

	if err := eligibility(
		candidate.Signed.Ticket.Participant,
	); err != nil {
		return WorkRelayPrecheckedV1{}, err
	}

	epochKey, err := RandomXWorkEpochKeyV1(
		chainID,
		expectedEpoch,
		anchor,
	)
	if err != nil {
		return WorkRelayPrecheckedV1{}, err
	}

	input, err := RandomXWorkInputV1(
		chainID,
		anchor,
		candidate.Signed.Ticket,
	)
	if err != nil {
		return WorkRelayPrecheckedV1{}, err
	}

	return WorkRelayPrecheckedV1{
		ChainID:    new(big.Int).Set(chainID),
		Epoch:      expectedEpoch,
		Anchor:     anchor,
		Difficulty: new(big.Int).Set(difficulty),
		Candidate:  cloneWorkCommitCandidateV1(candidate),
		EpochKey:   epochKey,
		Input:      append([]byte(nil), input...),
	}, nil
}

// VerifyPrecheckedRelayedWorkV1 performs exactly ONE expensive canonical
// RandomX recomputation through hasher, then requires exact equality with the
// signed claimed proof hash.
func VerifyPrecheckedRelayedWorkV1(
	prechecked WorkRelayPrecheckedV1,
	hasher WorkRelayHasherV1,
) (VerifiedRandomXWorkTicketV1, error) {
	if hasher == nil ||
		prechecked.ChainID == nil ||
		prechecked.ChainID.Sign() <= 0 ||
		prechecked.Epoch == 0 ||
		prechecked.Anchor == (common.Hash{}) ||
		prechecked.Difficulty == nil ||
		prechecked.Difficulty.Sign() <= 0 ||
		prechecked.Candidate.ProofHash == (common.Hash{}) ||
		prechecked.EpochKey == (common.Hash{}) ||
		len(prechecked.Input) == 0 {
		return VerifiedRandomXWorkTicketV1{}, ErrInvalidWorkRelayV1
	}

	recomputed, err := hasher(
		prechecked.EpochKey,
		append([]byte(nil), prechecked.Input...),
	)
	if err != nil {
		return VerifiedRandomXWorkTicketV1{}, err
	}

	if recomputed != prechecked.Candidate.ProofHash {
		return VerifiedRandomXWorkTicketV1{}, ErrWorkRelayClaimMismatchV1
	}

	return ValidateRecomputedRandomXWorkV1(
		prechecked.ChainID,
		prechecked.Anchor,
		prechecked.Difficulty,
		prechecked.Candidate.Signed,
		recomputed,
	)
}

// ValidateAndAdmitRelayedWorkV1 is the complete inactive ingress pipeline.
//
// Duplicate detection is repeated after RandomX by pool.AddVerifiedV1 to remain
// correct under concurrent arrivals.
func ValidateAndAdmitRelayedWorkV1(
	chainID *big.Int,
	expectedEpoch uint64,
	anchor common.Hash,
	difficulty *big.Int,
	candidate WorkCommitCandidateV1,
	eligibility WorkRelayEligibilityCheckV1,
	hasher WorkRelayHasherV1,
	limiter *WorkRelayVerificationLimiterV1,
	pool *WorkCommitPoolV1,
) error {
	if pool == nil || limiter == nil {
		return ErrInvalidWorkRelayV1
	}

	prechecked, err := PrecheckRelayedWorkV1(
		chainID,
		expectedEpoch,
		anchor,
		difficulty,
		candidate,
		eligibility,
	)
	if err != nil {
		return err
	}

	if pool.WorkCommitPoolContainsCandidateV1(
		prechecked.Candidate,
	) {
		return ErrWorkRelayAlreadyKnownV1
	}

	if !limiter.TryAcquireV1() {
		return ErrWorkRelayVerificationBusyV1
	}
	defer limiter.ReleaseV1()

	verified, err := VerifyPrecheckedRelayedWorkV1(
		prechecked,
		hasher,
	)
	if err != nil {
		return err
	}

	linked, err := NewVerifiedWorkCommitCandidateV1(
		prechecked.Candidate.Signed,
		verified,
	)
	if err != nil {
		return err
	}

	return pool.AddVerifiedV1(linked)
}
