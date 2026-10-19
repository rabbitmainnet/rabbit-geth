package lqc

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/params"
)

func TestLivenessV5AuthorizationPreservesLegacyV4BeforeFork(t *testing.T) {
	ordered := make([]HybridParticipant, 8)
	for i := range ordered {
		ordered[i].Address = common.BigToAddress(big.NewInt(int64(i + 1)))
	}

	selection := HybridSelection{
		Ordered:   ordered,
		Producer:  &ordered[0],
		Fallbacks: ordered[1:3],
	}

	engine := &LQC{config: &params.LQCConfig{
		ConsensusLivenessV3Block: 1,
		ConsensusLivenessV4Block: 100,
		ConsensusLivenessV5Block: 200,
	}}

	if allowed, pos := engine.isAuthorAllowedAt(199, selection, ordered[6].Address); !allowed || pos != 6 {
		t.Fatalf("legacy V4 behavior changed before V5: allowed=%v pos=%d", allowed, pos)
	}

	if allowed, pos := engine.isAuthorAllowedAt(200, selection, ordered[6].Address); allowed || pos != -1 {
		t.Fatalf("V5 authorized seat outside bounded fallbacks: allowed=%v pos=%d", allowed, pos)
	}

	if allowed, pos := engine.isAuthorAllowedAt(200, selection, ordered[2].Address); !allowed || pos != 2 {
		t.Fatalf("V5 rejected configured fallback: allowed=%v pos=%d", allowed, pos)
	}
}
