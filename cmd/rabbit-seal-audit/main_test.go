package main

import (
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

type fakeHeaderReader struct {
	chainID *big.Int
	headers map[uint64]*types.Header
}

func (f *fakeHeaderReader) ChainID(context.Context) (*big.Int, error) {
	return new(big.Int).Set(f.chainID), nil
}

func (f *fakeHeaderReader) HeaderByNumber(_ context.Context, number *big.Int) (*types.Header, error) {
	if number == nil {
		var highest uint64
		for candidate := range f.headers {
			if candidate > highest {
				highest = candidate
			}
		}
		return types.CopyHeader(f.headers[highest]), nil
	}
	return types.CopyHeader(f.headers[number.Uint64()]), nil
}

func signedTestHeader(t *testing.T, chainID *big.Int, number uint64) *types.Header {
	t.Helper()
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	extra, err := lqc.EncodeRegistryHeaderExtra(number, common.HexToHash("0x1234"), nil)
	if err != nil {
		t.Fatal(err)
	}
	header := &types.Header{
		ParentHash: common.BigToHash(new(big.Int).SetUint64(number - 1)),
		Coinbase:   crypto.PubkeyToAddress(key.PublicKey),
		Number:     new(big.Int).SetUint64(number),
		Difficulty: big.NewInt(0),
		GasLimit:   30_000_000,
		Time:       number,
		Extra:      extra,
	}
	sealed, err := lqc.New(nil, nil).SealHeader(chainID, header, func(_ common.Address, payload []byte) ([]byte, error) {
		return crypto.Sign(crypto.Keccak256(payload), key)
	})
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func TestAuditAcceptsCanonicalProducerSeals(t *testing.T) {
	chainID := big.NewInt(928)
	reader := &fakeHeaderReader{
		chainID: chainID,
		headers: map[uint64]*types.Header{
			1: signedTestHeader(t, chainID, 1),
			2: signedTestHeader(t, chainID, 2),
		},
	}
	result, err := audit(context.Background(), reader, options{rpcURL: "test", from: 1, to: 2})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "PASS" || result.ValidSeals != 2 || len(result.Failures) != 0 {
		t.Fatalf("unexpected report: %+v", result)
	}
}

func TestAuditRejectsMutatedHeader(t *testing.T) {
	chainID := big.NewInt(928)
	header := signedTestHeader(t, chainID, 1)
	header.Root[0] ^= 1
	reader := &fakeHeaderReader{chainID: chainID, headers: map[uint64]*types.Header{1: header}}
	result, err := audit(context.Background(), reader, options{rpcURL: "test", from: 1, to: 1})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "FAIL" || result.ValidSeals != 0 || len(result.Failures) != 1 {
		t.Fatalf("mutated header report: %+v", result)
	}
}
