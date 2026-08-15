package lqc

import (
	"bytes"
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

func producerSealTestHeader(t *testing.T, producer common.Address) *types.Header {
	t.Helper()
	extra, err := EncodeRegistryHeaderExtra(1, NewCanonicalRegistry().Root(), nil)
	if err != nil {
		t.Fatal(err)
	}
	return &types.Header{
		ParentHash:  common.HexToHash("0x1001"),
		UncleHash:   types.EmptyUncleHash,
		Coinbase:    producer,
		Root:        common.HexToHash("0x1002"),
		TxHash:      common.HexToHash("0x1003"),
		ReceiptHash: common.HexToHash("0x1004"),
		Difficulty:  big.NewInt(0),
		Number:      big.NewInt(1),
		GasLimit:    30_000_000,
		GasUsed:     21_000,
		Time:        1_800_000_000,
		Extra:       extra,
		BaseFee:     big.NewInt(7),
	}
}

func sealTestProducerHeader(t *testing.T, chainID *big.Int, header *types.Header, keyIndex int) *types.Header {
	t.Helper()
	engine := New(nil, nil)
	key := testParticipantKey(t, keyIndex)
	sealed, err := engine.SealHeader(chainID, header, func(address common.Address, payload []byte) ([]byte, error) {
		if address != crypto.PubkeyToAddress(key.PublicKey) {
			t.Fatalf("sign request address = %s, want %s", address, crypto.PubkeyToAddress(key.PublicKey))
		}
		return crypto.Sign(crypto.Keccak256(payload), key)
	})
	if err != nil {
		t.Fatalf("seal header: %v", err)
	}
	return sealed
}

func TestProducerSealRoundTrip(t *testing.T) {
	chainID := big.NewInt(928)
	producer := testParticipants(t, 1)[0]
	header := producerSealTestHeader(t, producer)
	sealed := sealTestProducerHeader(t, chainID, header, 0)

	if err := VerifyProducerSeal(chainID, sealed); err != nil {
		t.Fatalf("valid producer seal rejected: %v", err)
	}
	if bytes.Equal(header.Extra, sealed.Extra) {
		t.Fatal("zero seal placeholder was not replaced")
	}
	if !bytes.Equal(header.Extra[:len(header.Extra)-ProducerSealLength], sealed.Extra[:len(sealed.Extra)-ProducerSealLength]) {
		t.Fatal("sealing changed the canonical registry envelope")
	}
}

func TestProducerSealRejectsWrongKeyAndCoinbase(t *testing.T) {
	chainID := big.NewInt(928)
	participants := testParticipants(t, 2)
	header := producerSealTestHeader(t, participants[0])
	key := testParticipantKey(t, 1)
	engine := New(nil, nil)
	if _, err := engine.SealHeader(chainID, header, func(_ common.Address, payload []byte) ([]byte, error) {
		return crypto.Sign(crypto.Keccak256(payload), key)
	}); !errors.Is(err, ErrUnauthorizedProducerSeal) {
		t.Fatalf("wrong-key error = %v, want %v", err, ErrUnauthorizedProducerSeal)
	}

	sealed := sealTestProducerHeader(t, chainID, header, 0)
	sealed.Coinbase = participants[1]
	if err := VerifyProducerSeal(chainID, sealed); !errors.Is(err, ErrUnauthorizedProducerSeal) {
		t.Fatalf("altered-coinbase error = %v, want %v", err, ErrUnauthorizedProducerSeal)
	}
}

func TestProducerSealBindsConsensusAndExecutionHeaderFields(t *testing.T) {
	chainID := big.NewInt(928)
	producer := testParticipants(t, 1)[0]
	sealed := sealTestProducerHeader(t, chainID, producerSealTestHeader(t, producer), 0)

	tests := []struct {
		name   string
		mutate func(*types.Header)
	}{
		{"parent hash", func(h *types.Header) { h.ParentHash[0] ^= 0x01 }},
		{"uncle hash", func(h *types.Header) { h.UncleHash[0] ^= 0x01 }},
		{"state root", func(h *types.Header) { h.Root[0] ^= 0x01 }},
		{"transaction root", func(h *types.Header) { h.TxHash[0] ^= 0x01 }},
		{"receipt root", func(h *types.Header) { h.ReceiptHash[0] ^= 0x01 }},
		{"logs bloom", func(h *types.Header) { h.Bloom[0] ^= 0x01 }},
		{"difficulty", func(h *types.Header) { h.Difficulty = big.NewInt(1) }},
		{"block number", func(h *types.Header) { h.Number = new(big.Int).Add(h.Number, big.NewInt(1)) }},
		{"gas limit", func(h *types.Header) { h.GasLimit++ }},
		{"gas used", func(h *types.Header) { h.GasUsed++ }},
		{"timestamp", func(h *types.Header) { h.Time++ }},
		{"nonce", func(h *types.Header) { h.Nonce[0] ^= 0x01 }},
		{"mix digest", func(h *types.Header) { h.MixDigest[0] ^= 0x01 }},
		{"base fee", func(h *types.Header) { h.BaseFee = new(big.Int).Add(h.BaseFee, big.NewInt(1)) }},
		{"registry envelope", func(h *types.Header) { h.Extra[len(registryHeaderMagic)] ^= 0x01 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := types.CopyHeader(sealed)
			test.mutate(candidate)
			if err := VerifyProducerSeal(chainID, candidate); err == nil {
				t.Fatal("header mutation accepted with the old producer signature")
			}
		})
	}
}

func TestProducerSealRejectsCrossChainReplay(t *testing.T) {
	producer := testParticipants(t, 1)[0]
	sealed := sealTestProducerHeader(t, big.NewInt(928), producerSealTestHeader(t, producer), 0)
	if err := VerifyProducerSeal(big.NewInt(929), sealed); err == nil {
		t.Fatal("producer seal replayed on another chain ID")
	}
}

func TestProducerSealRejectsMissingMalformedAndHighS(t *testing.T) {
	chainID := big.NewInt(928)
	producer := testParticipants(t, 1)[0]
	header := producerSealTestHeader(t, producer)
	if err := VerifyProducerSeal(chainID, header); !errors.Is(err, ErrMissingProducerSeal) {
		t.Fatalf("zero-seal error = %v, want %v", err, ErrMissingProducerSeal)
	}

	malformed := types.CopyHeader(header)
	malformed.Extra = []byte("short")
	if err := VerifyProducerSeal(chainID, malformed); !errors.Is(err, ErrMissingProducerSeal) {
		t.Fatalf("short-seal error = %v, want %v", err, ErrMissingProducerSeal)
	}

	invalidV := sealTestProducerHeader(t, chainID, header, 0)
	invalidV.Extra[len(invalidV.Extra)-1] = 2
	if err := VerifyProducerSeal(chainID, invalidV); !errors.Is(err, ErrInvalidProducerSeal) {
		t.Fatalf("invalid-V error = %v, want %v", err, ErrInvalidProducerSeal)
	}

	highS := sealTestProducerHeader(t, chainID, header, 0)
	sealOffset := len(highS.Extra) - ProducerSealLength
	highSValue := new(big.Int).Sub(crypto.S256().Params().N, new(big.Int).SetBytes(highS.Extra[sealOffset+32:sealOffset+64]))
	highSValue.FillBytes(highS.Extra[sealOffset+32 : sealOffset+64])
	if err := VerifyProducerSeal(chainID, highS); !errors.Is(err, ErrInvalidProducerSeal) {
		t.Fatalf("high-S error = %v, want %v", err, ErrInvalidProducerSeal)
	}
}

func TestProducerSealPayloadExcludesOnlySignatureSuffix(t *testing.T) {
	chainID := big.NewInt(928)
	producer := testParticipants(t, 1)[0]
	header := producerSealTestHeader(t, producer)
	left, err := ProducerSealData(chainID, header)
	if err != nil {
		t.Fatal(err)
	}
	for index := len(header.Extra) - ProducerSealLength; index < len(header.Extra); index++ {
		header.Extra[index] = byte(index + 1)
	}
	right, err := ProducerSealData(chainID, header)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(left, right) {
		t.Fatal("signature suffix changed the producer signing payload")
	}
}

var _ consensus.HeaderSealer = (*LQC)(nil)
