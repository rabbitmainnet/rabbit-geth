package miner

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
)

func TestLQCWorkUsesHeaderProducerAsFeeRecipient(t *testing.T) {
	requested := common.HexToAddress("0x0000000000000000000000000000000000000001")
	producer := common.HexToAddress("0x0000000000000000000000000000000000000002")
	header := &types.Header{Coinbase: producer}
	config := &params.ChainConfig{LQC: new(params.LQCConfig)}

	if got := workFeeRecipient(config, header, requested); got != producer {
		t.Fatalf("wrong LQC fee recipient: got %s want header producer %s", got, producer)
	}
}

func TestNonLQCWorkPreservesRequestedFeeRecipient(t *testing.T) {
	requested := common.HexToAddress("0x0000000000000000000000000000000000000001")
	producer := common.HexToAddress("0x0000000000000000000000000000000000000002")
	header := &types.Header{Coinbase: producer}

	if got := workFeeRecipient(new(params.ChainConfig), header, requested); got != requested {
		t.Fatalf("non-LQC recipient changed: got %s want %s", got, requested)
	}
}
