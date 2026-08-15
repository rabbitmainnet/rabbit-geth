package lqc

import (
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
)

var (
	ErrInvalidRandomXWorkChainID    = errors.New("invalid lqc randomx work chain id")
	ErrInvalidRandomXWorkAnchor     = errors.New("invalid lqc randomx work anchor")
	ErrInvalidRandomXWorkSignature  = errors.New("invalid lqc randomx work signature")
	ErrInvalidRandomXWorkDifficulty = errors.New("invalid lqc randomx work difficulty")
	ErrRandomXWorkTargetNotMet      = errors.New("lqc randomx work target not met")
)

var (
	randomXWorkEpochKeyDomainV1 = []byte("RABBIT-LQC-RANDOMX-EPOCH-KEY-V1")
	randomXWorkInputDomainV1    = []byte("RABBIT-LQC-RANDOMX-INPUT-V1")
	randomXWorkSignDomainV1     = []byte("RABBIT-LQC-RANDOMX-SIGN-V1")
)

// SignedRandomXWorkTicketV1 carries one signature only after a nonce has
// produced a qualifying RandomX hash. Failed attempts need no signature.
//
// The signature prevents a third party from mining valid tickets for a victim
// address and thereby forcing unwanted producer/fallback/committee seats onto
// that victim.
type SignedRandomXWorkTicketV1 struct {
	Ticket    RandomXWorkTicketV1
	Signature []byte
}

type randomXWorkEpochKeyPayloadV1 struct {
	Domain  []byte
	Version uint8
	ChainID *big.Int
	Epoch   uint64
	Anchor  common.Hash
}

type randomXWorkInputPayloadV1 struct {
	Domain      []byte
	Version     uint8
	ChainID     *big.Int
	Epoch       uint64
	Anchor      common.Hash
	Participant common.Address
	Nonce       uint64
}

type randomXWorkSignPayloadV1 struct {
	Domain      []byte
	Version     uint8
	ChainID     *big.Int
	Epoch       uint64
	Anchor      common.Hash
	Participant common.Address
	Nonce       uint64
	ProofHash   common.Hash
}

func validateRandomXWorkContextV1(
	chainID *big.Int,
	anchor common.Hash,
	ticket RandomXWorkTicketV1,
) error {
	if chainID == nil || chainID.Sign() <= 0 {
		return ErrInvalidRandomXWorkChainID
	}
	if anchor == (common.Hash{}) {
		return ErrInvalidRandomXWorkAnchor
	}
	return validateRandomXWorkTicketV1(ticket)
}

// RandomXWorkEpochKeyV1 returns the global dataset/cache key for one epoch.
// Participant identity is deliberately absent: splitting work across addresses
// must not allocate extra datasets or change the work economics.
func RandomXWorkEpochKeyV1(
	chainID *big.Int,
	epoch uint64,
	anchor common.Hash,
) (common.Hash, error) {
	if chainID == nil || chainID.Sign() <= 0 {
		return common.Hash{}, ErrInvalidRandomXWorkChainID
	}
	if epoch == 0 {
		return common.Hash{}, ErrInvalidRandomXWorkTicket
	}
	if anchor == (common.Hash{}) {
		return common.Hash{}, ErrInvalidRandomXWorkAnchor
	}

	encoded, err := rlp.EncodeToBytes(randomXWorkEpochKeyPayloadV1{
		Domain:  randomXWorkEpochKeyDomainV1,
		Version: RandomXWorkProtocolVersion,
		ChainID: new(big.Int).Set(chainID),
		Epoch:   epoch,
		Anchor:  anchor,
	})
	if err != nil {
		return common.Hash{}, err
	}
	return crypto.Keccak256Hash(encoded), nil
}

// RandomXWorkInputV1 is the exact canonical byte string that a future RandomX
// engine must hash. It binds chain, epoch, anchor, participant and nonce.
func RandomXWorkInputV1(
	chainID *big.Int,
	anchor common.Hash,
	ticket RandomXWorkTicketV1,
) ([]byte, error) {
	if err := validateRandomXWorkContextV1(chainID, anchor, ticket); err != nil {
		return nil, err
	}

	return rlp.EncodeToBytes(randomXWorkInputPayloadV1{
		Domain:      randomXWorkInputDomainV1,
		Version:     ticket.Version,
		ChainID:     new(big.Int).Set(chainID),
		Epoch:       ticket.Epoch,
		Anchor:      anchor,
		Participant: ticket.Participant,
		Nonce:       ticket.Nonce,
	})
}

// RandomXWorkSigningHashV1 signs only successful work. ProofHash must be the
// hash recomputed from RandomXWorkInputV1 by the consensus RandomX verifier.
func RandomXWorkSigningHashV1(
	chainID *big.Int,
	anchor common.Hash,
	ticket RandomXWorkTicketV1,
	proofHash common.Hash,
) (common.Hash, error) {
	if err := validateRandomXWorkContextV1(chainID, anchor, ticket); err != nil {
		return common.Hash{}, err
	}

	encoded, err := rlp.EncodeToBytes(randomXWorkSignPayloadV1{
		Domain:      randomXWorkSignDomainV1,
		Version:     ticket.Version,
		ChainID:     new(big.Int).Set(chainID),
		Epoch:       ticket.Epoch,
		Anchor:      anchor,
		Participant: ticket.Participant,
		Nonce:       ticket.Nonce,
		ProofHash:   proofHash,
	})
	if err != nil {
		return common.Hash{}, err
	}
	return crypto.Keccak256Hash(encoded), nil
}

func RecoverRandomXWorkSignerV1(
	chainID *big.Int,
	anchor common.Hash,
	signed SignedRandomXWorkTicketV1,
	proofHash common.Hash,
) (common.Address, error) {
	if len(signed.Signature) != crypto.SignatureLength {
		return common.Address{}, ErrInvalidRandomXWorkSignature
	}

	r := new(big.Int).SetBytes(signed.Signature[:32])
	s := new(big.Int).SetBytes(signed.Signature[32:64])
	v := signed.Signature[64]
	if !crypto.ValidateSignatureValues(v, r, s, true) {
		return common.Address{}, ErrInvalidRandomXWorkSignature
	}

	hash, err := RandomXWorkSigningHashV1(
		chainID,
		anchor,
		signed.Ticket,
		proofHash,
	)
	if err != nil {
		return common.Address{}, err
	}

	publicKey, err := crypto.SigToPub(hash[:], signed.Signature)
	if err != nil {
		return common.Address{}, ErrInvalidRandomXWorkSignature
	}
	return crypto.PubkeyToAddress(*publicKey), nil
}

func VerifyRandomXWorkSignatureV1(
	chainID *big.Int,
	anchor common.Hash,
	signed SignedRandomXWorkTicketV1,
	proofHash common.Hash,
) error {
	signer, err := RecoverRandomXWorkSignerV1(
		chainID,
		anchor,
		signed,
		proofHash,
	)
	if err != nil {
		return err
	}
	if signer != signed.Ticket.Participant {
		return ErrInvalidRandomXWorkSignature
	}
	return nil
}

// RandomXWorkTargetV1 uses the conventional 256-bit target relation:
// target = (2^256 - 1) / difficulty.
func RandomXWorkTargetV1(difficulty *big.Int) (*big.Int, error) {
	if difficulty == nil || difficulty.Sign() <= 0 {
		return nil, ErrInvalidRandomXWorkDifficulty
	}

	max := new(big.Int).Lsh(big.NewInt(1), 256)
	max.Sub(max, big.NewInt(1))

	target := new(big.Int).Div(max, difficulty)
	if target.Sign() <= 0 {
		return nil, ErrInvalidRandomXWorkDifficulty
	}
	return target, nil
}

func RandomXWorkHashMeetsTargetV1(
	proofHash common.Hash,
	difficulty *big.Int,
) (bool, error) {
	target, err := RandomXWorkTargetV1(difficulty)
	if err != nil {
		return false, err
	}
	value := new(big.Int).SetBytes(proofHash[:])
	return value.Cmp(target) <= 0, nil
}

// ValidateRecomputedRandomXWorkV1 is intentionally NOT a RandomX implementation.
// The caller must first recompute proofHash using the canonical RandomX engine
// over RandomXWorkInputV1. This helper then enforces target + ownership and
// converts the result into a verified ticket that can become one work seat.
func ValidateRecomputedRandomXWorkV1(
	chainID *big.Int,
	anchor common.Hash,
	difficulty *big.Int,
	signed SignedRandomXWorkTicketV1,
	proofHash common.Hash,
) (VerifiedRandomXWorkTicketV1, error) {
	meets, err := RandomXWorkHashMeetsTargetV1(proofHash, difficulty)
	if err != nil {
		return VerifiedRandomXWorkTicketV1{}, err
	}
	if !meets {
		return VerifiedRandomXWorkTicketV1{}, ErrRandomXWorkTargetNotMet
	}
	if err := VerifyRandomXWorkSignatureV1(
		chainID,
		anchor,
		signed,
		proofHash,
	); err != nil {
		return VerifiedRandomXWorkTicketV1{}, err
	}

	return VerifiedRandomXWorkTicketV1{
		Ticket: signed.Ticket,
		Hash:   proofHash,
	}, nil
}
