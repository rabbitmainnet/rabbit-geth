package lqc

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
)

const ProducerSealLength = crypto.SignatureLength

var (
	producerSealDomain = []byte("RABBIT-LQC-BLOCK-SEAL-V1")

	ErrMissingProducerSeal      = errors.New("missing lqc producer seal")
	ErrInvalidProducerSeal      = errors.New("invalid lqc producer seal")
	ErrUnauthorizedProducerSeal = errors.New("lqc producer seal signer does not match coinbase")
)

type producerSealPayload struct {
	Domain  []byte
	ChainID *big.Int
	Header  *types.Header
}

func splitProducerSeal(extra []byte) ([]byte, []byte, error) {
	if len(extra) < ProducerSealLength {
		return nil, nil, ErrMissingProducerSeal
	}
	payloadLength := len(extra) - ProducerSealLength
	return extra[:payloadLength], extra[payloadLength:], nil
}

func appendEmptyProducerSeal(extra []byte) []byte {
	sealed := make([]byte, 0, len(extra)+ProducerSealLength)
	sealed = append(sealed, extra...)
	return append(sealed, make([]byte, ProducerSealLength)...)
}

// ProducerSealData returns the domain-separated, chain-specific RLP payload
// signed by an LQC producer. The fixed 65-byte seal suffix is excluded, while
// every other header field remains covered by the signature.
func ProducerSealData(chainID *big.Int, header *types.Header) ([]byte, error) {
	if chainID == nil || chainID.Sign() <= 0 {
		return nil, errors.New("missing lqc seal chain ID")
	}
	if header == nil || header.Number == nil || header.Number.Sign() <= 0 {
		return nil, errors.New("invalid lqc seal header")
	}
	extra, _, err := splitProducerSeal(header.Extra)
	if err != nil {
		return nil, err
	}
	unsigned := types.CopyHeader(header)
	unsigned.Extra = bytes.Clone(extra)
	payload, err := rlp.EncodeToBytes(producerSealPayload{
		Domain:  producerSealDomain,
		ChainID: new(big.Int).Set(chainID),
		Header:  unsigned,
	})
	if err != nil {
		return nil, fmt.Errorf("encode lqc producer seal: %w", err)
	}
	return payload, nil
}

func producerSealHash(chainID *big.Int, header *types.Header) (common.Hash, error) {
	payload, err := ProducerSealData(chainID, header)
	if err != nil {
		return common.Hash{}, err
	}
	return crypto.Keccak256Hash(payload), nil
}

func recoverProducerSeal(chainID *big.Int, header *types.Header) (common.Address, error) {
	_, signature, err := splitProducerSeal(header.Extra)
	if err != nil {
		return common.Address{}, err
	}
	if bytes.Equal(signature, make([]byte, ProducerSealLength)) {
		return common.Address{}, ErrMissingProducerSeal
	}
	if len(signature) != ProducerSealLength {
		return common.Address{}, ErrInvalidProducerSeal
	}
	r := new(big.Int).SetBytes(signature[:32])
	s := new(big.Int).SetBytes(signature[32:64])
	if !crypto.ValidateSignatureValues(signature[64], r, s, true) {
		return common.Address{}, ErrInvalidProducerSeal
	}
	hash, err := producerSealHash(chainID, header)
	if err != nil {
		return common.Address{}, err
	}
	publicKey, err := crypto.SigToPub(hash[:], signature)
	if err != nil {
		return common.Address{}, fmt.Errorf("%w: %v", ErrInvalidProducerSeal, err)
	}
	return crypto.PubkeyToAddress(*publicKey), nil
}

// VerifyProducerSeal proves that the private key controlling header.Coinbase
// signed this exact header for this exact chain.
func VerifyProducerSeal(chainID *big.Int, header *types.Header) error {
	if header == nil || header.Coinbase == (common.Address{}) {
		return ErrUnauthorizedProducerSeal
	}
	signer, err := recoverProducerSeal(chainID, header)
	if err != nil {
		return err
	}
	if signer != header.Coinbase {
		return fmt.Errorf("%w: have %s want %s", ErrUnauthorizedProducerSeal, signer, header.Coinbase)
	}
	return nil
}

// SealHeader signs after FinalizeAndAssemble, when state, transaction and
// receipt roots have reached their final values.
func (l *LQC) SealHeader(chainID *big.Int, header *types.Header, sign consensus.HeaderSignerFn) (*types.Header, error) {
	if sign == nil {
		return nil, errors.New("missing lqc header signer")
	}
	payload, err := ProducerSealData(chainID, header)
	if err != nil {
		return nil, err
	}
	signature, err := sign(header.Coinbase, payload)
	if err != nil {
		return nil, fmt.Errorf("sign lqc header: %w", err)
	}
	if len(signature) != ProducerSealLength {
		return nil, ErrInvalidProducerSeal
	}
	extra, _, err := splitProducerSeal(header.Extra)
	if err != nil {
		return nil, err
	}
	sealed := types.CopyHeader(header)
	sealed.Extra = make([]byte, 0, len(extra)+ProducerSealLength)
	sealed.Extra = append(sealed.Extra, extra...)
	sealed.Extra = append(sealed.Extra, signature...)
	if err := VerifyProducerSeal(chainID, sealed); err != nil {
		return nil, err
	}
	return sealed, nil
}
