package lqc

import (
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
)

const CommitteeParticipationVersionV1 uint8 = 1

var (
	ErrInvalidCommitteeParticipationV1   = errors.New("invalid lqc committee participation v1")
	ErrInvalidCommitteeSignatureV1       = errors.New("invalid lqc committee participation signature v1")
	ErrCommitteeParticipantNotSelectedV1 = errors.New("lqc committee participant not selected v1")
)

var (
	committeeParticipationInputDomainV1 = []byte("RABBIT-LQC-COMMITTEE-PARTICIPATION-INPUT-V1")
	committeeParticipationSignDomainV1  = []byte("RABBIT-LQC-COMMITTEE-PARTICIPATION-SIGN-V1")
)

// CommitteeParticipationV1 is one selected WorkSeat's proof of active work for
// one block. ProofHash is recomputed by consensus and is not transmitted.
type CommitteeParticipationV1 struct {
	Version       uint8
	BlockNumber   uint64
	ParentHash    common.Hash
	SelectionRoot common.Hash
	TicketHash    common.Hash
	Participant   common.Address
	Signature     []byte
}

type committeeParticipationInputPayloadV1 struct {
	Domain        []byte
	Version       uint8
	ChainID       *big.Int
	BlockNumber   uint64
	ParentHash    common.Hash
	SelectionRoot common.Hash
	TicketHash    common.Hash
	Participant   common.Address
}

type committeeParticipationSignPayloadV1 struct {
	Domain        []byte
	Version       uint8
	ChainID       *big.Int
	BlockNumber   uint64
	ParentHash    common.Hash
	SelectionRoot common.Hash
	TicketHash    common.Hash
	Participant   common.Address
	ProofHash     common.Hash
}

func validateCommitteeParticipationContextV1(
	chainID *big.Int,
	proof CommitteeParticipationV1,
) error {
	if chainID == nil || chainID.Sign() <= 0 ||
		proof.Version != CommitteeParticipationVersionV1 ||
		proof.BlockNumber == 0 ||
		proof.ParentHash == (common.Hash{}) ||
		proof.SelectionRoot == (common.Hash{}) ||
		proof.TicketHash == (common.Hash{}) ||
		proof.Participant == (common.Address{}) {
		return ErrInvalidCommitteeParticipationV1
	}
	return nil
}

// CommitteeParticipationInputV1 is hashed exactly once by RandomX. There is no
// nonce or target race: every selected wallet performs the same bounded work,
// so additional hash power cannot increase its reward weight.
func CommitteeParticipationInputV1(
	chainID *big.Int,
	proof CommitteeParticipationV1,
) ([]byte, error) {
	if err := validateCommitteeParticipationContextV1(chainID, proof); err != nil {
		return nil, err
	}
	return rlp.EncodeToBytes(committeeParticipationInputPayloadV1{
		Domain:        committeeParticipationInputDomainV1,
		Version:       proof.Version,
		ChainID:       new(big.Int).Set(chainID),
		BlockNumber:   proof.BlockNumber,
		ParentHash:    proof.ParentHash,
		SelectionRoot: proof.SelectionRoot,
		TicketHash:    proof.TicketHash,
		Participant:   proof.Participant,
	})
}

func CommitteeParticipationSigningHashV1(
	chainID *big.Int,
	proof CommitteeParticipationV1,
	proofHash common.Hash,
) (common.Hash, error) {
	encoded, err := CommitteeParticipationSigningDataV1(chainID, proof, proofHash)
	if err != nil {
		return common.Hash{}, err
	}
	return crypto.Keccak256Hash(encoded), nil
}

// CommitteeParticipationSigningDataV1 returns the canonical payload supplied
// to accounts.Wallet.SignData. The wallet hashes this payload once before
// signing, matching CommitteeParticipationSigningHashV1 exactly.
func CommitteeParticipationSigningDataV1(
	chainID *big.Int,
	proof CommitteeParticipationV1,
	proofHash common.Hash,
) ([]byte, error) {
	if err := validateCommitteeParticipationContextV1(chainID, proof); err != nil ||
		proofHash == (common.Hash{}) {
		return nil, ErrInvalidCommitteeParticipationV1
	}
	return rlp.EncodeToBytes(committeeParticipationSignPayloadV1{
		Domain:        committeeParticipationSignDomainV1,
		Version:       proof.Version,
		ChainID:       new(big.Int).Set(chainID),
		BlockNumber:   proof.BlockNumber,
		ParentHash:    proof.ParentHash,
		SelectionRoot: proof.SelectionRoot,
		TicketHash:    proof.TicketHash,
		Participant:   proof.Participant,
		ProofHash:     proofHash,
	})
}

func VerifyCommitteeParticipationV1(
	chainID *big.Int,
	selected WorkSeatV1,
	proof CommitteeParticipationV1,
	proofHash common.Hash,
) error {
	if selected.Participant != proof.Participant ||
		selected.TicketHash != proof.TicketHash {
		return ErrCommitteeParticipantNotSelectedV1
	}
	if len(proof.Signature) != crypto.SignatureLength {
		return ErrInvalidCommitteeSignatureV1
	}
	signingHash, err := CommitteeParticipationSigningHashV1(chainID, proof, proofHash)
	if err != nil {
		return err
	}
	publicKey, err := crypto.SigToPub(signingHash[:], proof.Signature)
	if err != nil || crypto.PubkeyToAddress(*publicKey) != proof.Participant {
		return ErrInvalidCommitteeSignatureV1
	}
	return nil
}
