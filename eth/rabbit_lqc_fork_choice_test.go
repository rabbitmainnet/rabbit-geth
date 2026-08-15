package eth

import (
	"bytes"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/eth/protocols/eth"
)

func TestShouldStartLQCSync(t *testing.T) {
	local := &types.Header{Number: big.NewInt(10), Extra: []byte("local")}
	localHash := local.Hash()
	lowerHash, higherHash := hashesAround(localHash)

	tests := []struct {
		name         string
		local        *types.Header
		remoteNumber uint64
		remoteHash   common.Hash
		want         bool
	}{
		{name: "zero remote hash", local: local, remoteNumber: 11, remoteHash: common.Hash{}, want: false},
		{name: "higher chain", local: local, remoteNumber: 11, remoteHash: higherHash, want: true},
		{name: "lower chain", local: local, remoteNumber: 9, remoteHash: lowerHash, want: false},
		{name: "same canonical head", local: local, remoteNumber: 10, remoteHash: localHash, want: false},
		{name: "equal height lower hash", local: local, remoteNumber: 10, remoteHash: lowerHash, want: true},
		{name: "equal height higher hash", local: local, remoteNumber: 10, remoteHash: higherHash, want: false},
		{name: "empty local chain", local: nil, remoteNumber: 1, remoteHash: higherHash, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := shouldStartLQCSync(test.local, test.remoteNumber, test.remoteHash); got != test.want {
				t.Fatalf("shouldStartLQCSync = %t, want %t", got, test.want)
			}
		})
	}
}

func TestBetterLQCSyncTarget(t *testing.T) {
	base := &types.Header{Number: big.NewInt(10), Extra: []byte("base")}
	lowerHashHeader, higherHashHeader := headersAroundHash(base)
	higherBlock := &types.Header{Number: big.NewInt(11), Extra: []byte("higher-block")}
	lowerBlock := &types.Header{Number: big.NewInt(9), Extra: []byte("lower-block")}

	tests := []struct {
		name      string
		candidate *lqcSyncTarget
		current   *lqcSyncTarget
		want      bool
	}{
		{name: "nil candidate", current: &lqcSyncTarget{header: base}, want: false},
		{name: "empty current", candidate: &lqcSyncTarget{header: base}, want: true},
		{name: "higher block", candidate: &lqcSyncTarget{header: higherBlock}, current: &lqcSyncTarget{header: base}, want: true},
		{name: "lower block", candidate: &lqcSyncTarget{header: lowerBlock}, current: &lqcSyncTarget{header: base}, want: false},
		{name: "same height lower hash", candidate: &lqcSyncTarget{header: lowerHashHeader}, current: &lqcSyncTarget{header: base}, want: true},
		{name: "same height higher hash", candidate: &lqcSyncTarget{header: higherHashHeader}, current: &lqcSyncTarget{header: base}, want: false},
		{name: "same target", candidate: &lqcSyncTarget{header: base}, current: &lqcSyncTarget{header: base}, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := betterLQCSyncTarget(test.candidate, test.current); got != test.want {
				t.Fatalf("betterLQCSyncTarget = %t, want %t", got, test.want)
			}
		})
	}
}

func TestLQCPeerRangeContainsTarget(t *testing.T) {
	targetNumber := uint64(404)
	targetHash := common.HexToHash("0x2f0c3d5d73b7b18491ca18e76497c77c6ad8d7ec6f84ab4f1cff015aae7fbb60")
	replacementHash := common.HexToHash("0xe972b2d95cbb3191431a2724bb869f6d1ed1d1e2efcf808142c14f6a3398a62e")

	tests := []struct {
		name      string
		peerRange *eth.BlockRangeUpdatePacket
		want      bool
	}{
		{name: "range unavailable", want: true},
		{
			name: "same target still advertised",
			peerRange: &eth.BlockRangeUpdatePacket{
				LatestBlock:     targetNumber,
				LatestBlockHash: targetHash,
			},
			want: true,
		},
		{
			name: "higher peer head may contain target",
			peerRange: &eth.BlockRangeUpdatePacket{
				LatestBlock:     targetNumber + 1,
				LatestBlockHash: replacementHash,
			},
			want: true,
		},
		{
			name: "peer rolled back below target",
			peerRange: &eth.BlockRangeUpdatePacket{
				LatestBlock:     targetNumber - 1,
				LatestBlockHash: replacementHash,
			},
			want: false,
		},
		{
			name: "peer replaced target at same height",
			peerRange: &eth.BlockRangeUpdatePacket{
				LatestBlock:     targetNumber,
				LatestBlockHash: replacementHash,
			},
			want: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := lqcPeerRangeContainsTarget(test.peerRange, targetNumber, targetHash); got != test.want {
				t.Fatalf("lqcPeerRangeContainsTarget = %t, want %t", got, test.want)
			}
		})
	}
}

func headersAroundHash(base *types.Header) (*types.Header, *types.Header) {
	baseHash := base.Hash()
	for nonce := uint64(0); ; nonce++ {
		candidate := &types.Header{Number: new(big.Int).Set(base.Number), Extra: []byte("candidate"), Nonce: types.EncodeNonce(nonce)}
		if bytes.Compare(candidate.Hash().Bytes(), baseHash.Bytes()) < 0 {
			lower := candidate
			for nonce++; ; nonce++ {
				candidate = &types.Header{Number: new(big.Int).Set(base.Number), Extra: []byte("candidate"), Nonce: types.EncodeNonce(nonce)}
				if bytes.Compare(candidate.Hash().Bytes(), baseHash.Bytes()) > 0 {
					return lower, candidate
				}
			}
		}
	}
}

func hashesAround(hash common.Hash) (common.Hash, common.Hash) {
	lower := hash
	for i := len(lower) - 1; i >= 0; i-- {
		if lower[i] > 0 {
			lower[i]--
			for j := i + 1; j < len(lower); j++ {
				lower[j] = 0xff
			}
			break
		}
	}
	higher := hash
	for i := len(higher) - 1; i >= 0; i-- {
		if higher[i] < 0xff {
			higher[i]++
			for j := i + 1; j < len(higher); j++ {
				higher[j] = 0
			}
			break
		}
	}
	return lower, higher
}
