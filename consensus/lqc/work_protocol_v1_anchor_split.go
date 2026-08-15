package lqc

import (
	"errors"
	"math"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

var ErrInvalidWorkAnchorScheduleV1 = errors.New("invalid lqc work anchor schedule v1")

func WorkDatasetAnchorBlockV1(epoch, epochLength uint64) (uint64, error) {
	if epoch == 0 || epochLength == 0 {
		return 0, ErrInvalidWorkAnchorScheduleV1
	}
	if epoch <= 2 {
		return 0, nil
	}
	n := epoch - 2
	if n > math.MaxUint64/epochLength {
		return 0, ErrInvalidWorkAnchorScheduleV1
	}
	return n * epochLength, nil
}

func WorkChallengeAnchorBlockV1(epoch, epochLength uint64) (uint64, error) {
	if epoch == 0 || epochLength == 0 {
		return 0, ErrInvalidWorkAnchorScheduleV1
	}
	if epoch == 1 {
		return 1, nil
	}
	n := epoch - 1
	if n > math.MaxUint64/epochLength {
		return 0, ErrInvalidWorkAnchorScheduleV1
	}
	return n * epochLength, nil
}

func RandomXWorkDatasetKeyV1(chainID *big.Int, epoch uint64, datasetAnchor common.Hash) (common.Hash, error) {
	return RandomXWorkEpochKeyV1(chainID, epoch, datasetAnchor)
}

func RandomXWorkChallengeInputV1(chainID *big.Int, challengeAnchor common.Hash, ticket RandomXWorkTicketV1) ([]byte, error) {
	return RandomXWorkInputV1(chainID, challengeAnchor, ticket)
}

type WorkRelayPrecheckedAnchorsV1 struct {
	ChainID         *big.Int
	Epoch           uint64
	DatasetAnchor   common.Hash
	ChallengeAnchor common.Hash
	Difficulty      *big.Int
	Candidate       WorkCommitCandidateV1
	EpochKey        common.Hash
	Input           []byte
}

func PrecheckRelayedWorkWithAnchorsV1(chainID *big.Int, expectedEpoch uint64, datasetAnchor, challengeAnchor common.Hash, difficulty *big.Int, candidate WorkCommitCandidateV1, eligibility WorkRelayEligibilityCheckV1) (WorkRelayPrecheckedAnchorsV1, error) {
	if chainID == nil || chainID.Sign() <= 0 || expectedEpoch == 0 || datasetAnchor == (common.Hash{}) || challengeAnchor == (common.Hash{}) || difficulty == nil || difficulty.Sign() <= 0 || candidate.ProofHash == (common.Hash{}) || eligibility == nil {
		return WorkRelayPrecheckedAnchorsV1{}, ErrInvalidWorkRelayV1
	}
	if err := validateWorkTicketHeaderShapeV3(candidate.Signed); err != nil {
		return WorkRelayPrecheckedAnchorsV1{}, err
	}
	if candidate.Signed.Ticket.Epoch != expectedEpoch {
		return WorkRelayPrecheckedAnchorsV1{}, ErrWorkCommitEpochMismatchV1
	}
	ok, err := RandomXWorkHashMeetsTargetV1(candidate.ProofHash, difficulty)
	if err != nil {
		return WorkRelayPrecheckedAnchorsV1{}, err
	}
	if !ok {
		return WorkRelayPrecheckedAnchorsV1{}, ErrWorkRelayClaimTargetV1
	}
	if err := VerifyRandomXWorkSignatureV1(chainID, challengeAnchor, candidate.Signed, candidate.ProofHash); err != nil {
		return WorkRelayPrecheckedAnchorsV1{}, err
	}
	if err := eligibility(candidate.Signed.Ticket.Participant); err != nil {
		return WorkRelayPrecheckedAnchorsV1{}, err
	}
	key, err := RandomXWorkDatasetKeyV1(chainID, expectedEpoch, datasetAnchor)
	if err != nil {
		return WorkRelayPrecheckedAnchorsV1{}, err
	}
	input, err := RandomXWorkChallengeInputV1(chainID, challengeAnchor, candidate.Signed.Ticket)
	if err != nil {
		return WorkRelayPrecheckedAnchorsV1{}, err
	}
	return WorkRelayPrecheckedAnchorsV1{ChainID: new(big.Int).Set(chainID), Epoch: expectedEpoch, DatasetAnchor: datasetAnchor, ChallengeAnchor: challengeAnchor, Difficulty: new(big.Int).Set(difficulty), Candidate: cloneWorkCommitCandidateV1(candidate), EpochKey: key, Input: append([]byte(nil), input...)}, nil
}

func VerifyPrecheckedRelayedWorkWithAnchorsV1(p WorkRelayPrecheckedAnchorsV1, hasher WorkRelayHasherV1) (VerifiedRandomXWorkTicketV1, error) {
	if hasher == nil || p.ChainID == nil || p.ChainID.Sign() <= 0 || p.Epoch == 0 || p.DatasetAnchor == (common.Hash{}) || p.ChallengeAnchor == (common.Hash{}) || p.Difficulty == nil || p.Difficulty.Sign() <= 0 || p.Candidate.ProofHash == (common.Hash{}) || p.EpochKey == (common.Hash{}) || len(p.Input) == 0 {
		return VerifiedRandomXWorkTicketV1{}, ErrInvalidWorkRelayV1
	}
	hash, err := hasher(p.EpochKey, append([]byte(nil), p.Input...))
	if err != nil {
		return VerifiedRandomXWorkTicketV1{}, err
	}
	if hash != p.Candidate.ProofHash {
		return VerifiedRandomXWorkTicketV1{}, ErrWorkRelayClaimMismatchV1
	}
	return ValidateRecomputedRandomXWorkV1(p.ChainID, p.ChallengeAnchor, p.Difficulty, p.Candidate.Signed, hash)
}

func ValidateAndAdmitRelayedWorkWithAnchorsV1(chainID *big.Int, expectedEpoch uint64, datasetAnchor, challengeAnchor common.Hash, difficulty *big.Int, candidate WorkCommitCandidateV1, eligibility WorkRelayEligibilityCheckV1, hasher WorkRelayHasherV1, limiter *WorkRelayVerificationLimiterV1, pool *WorkCommitPoolV1) error {
	if pool == nil || limiter == nil {
		return ErrInvalidWorkRelayV1
	}
	p, err := PrecheckRelayedWorkWithAnchorsV1(chainID, expectedEpoch, datasetAnchor, challengeAnchor, difficulty, candidate, eligibility)
	if err != nil {
		return err
	}
	if pool.WorkCommitPoolContainsCandidateV1(p.Candidate) {
		return ErrWorkRelayAlreadyKnownV1
	}
	if !limiter.TryAcquireV1() {
		return ErrWorkRelayVerificationBusyV1
	}
	defer limiter.ReleaseV1()
	verified, err := VerifyPrecheckedRelayedWorkWithAnchorsV1(p, hasher)
	if err != nil {
		return err
	}
	linked, err := NewVerifiedWorkCommitCandidateV1(p.Candidate.Signed, verified)
	if err != nil {
		return err
	}
	return pool.AddVerifiedV1(linked)
}
