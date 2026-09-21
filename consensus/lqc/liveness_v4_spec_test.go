package lqc

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/params"
)

func TestLivenessV4WholeQueueFailoverForkBoundary(t *testing.T) {
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
	}}

	if allowed, _ := engine.isAuthorAllowedAt(
		199, selection, ordered[6].Address,
	); allowed {
		t.Fatal("V3 authorized seat outside bounded fallbacks")
	}

	if allowed, pos := engine.isAuthorAllowedAt(
		200, selection, ordered[6].Address,
	); !allowed || pos != 6 {
		t.Fatalf(
			"V4 full-queue failover rejected: allowed=%v pos=%d",
			allowed, pos,
		)
	}
}
