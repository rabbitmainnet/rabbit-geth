package lqc

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/params"
)

func TestLivenessV4KeepsNormalAuthorizationBoundedAtForkBoundary(t *testing.T) {
	ordered := make([]HybridParticipant, 8)
	for i := range ordered {
		ordered[i].Address = common.BigToAddress(big.NewInt(int64(i + 1)))
	}
	selection := HybridSelection{
		Ordered:   ordered,
		Producer:  &ordered[0],
		Fallbacks: append([]HybridParticipant(nil), ordered[1:3]...),
	}
	engine := &LQC{config: &params.LQCConfig{
		ConsensusLivenessV3Block: 100,
		ConsensusLivenessV4Block: 200,
		ConsensusLivenessV5Block: 200,
	}}

	if allowed, _ := engine.isAuthorAllowedAt(
		199, selection, ordered[6].Address,
	); allowed {
		t.Fatal("V3 authorized seat outside bounded fallbacks")
	}

	if allowed, pos := engine.isAuthorAllowedAt(
		200, selection, ordered[6].Address,
	); allowed || pos != -1 {
		t.Fatalf(
			"V4 authorized seat outside bounded fallbacks: allowed=%v pos=%d",
			allowed, pos,
		)
	}

	if allowed, pos := engine.isAuthorAllowedAt(
		200, selection, ordered[2].Address,
	); !allowed || pos != 2 {
		t.Fatalf(
			"V4 rejected configured fallback: allowed=%v pos=%d",
			allowed, pos,
		)
	}
}
