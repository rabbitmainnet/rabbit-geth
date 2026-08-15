package ethconfig

import (
	"testing"

	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/params"
)

func TestCreateConsensusEngineSelectsCanonicalLQC(t *testing.T) {
	engine, err := CreateConsensusEngine(&params.ChainConfig{
		LQC: &params.LQCConfig{},
	}, rawdb.NewMemoryDatabase())
	if err != nil {
		t.Fatalf("create consensus engine: %v", err)
	}
	if _, ok := engine.(*lqc.LQC); !ok {
		t.Fatalf("active engine is %T, want *lqc.LQC", engine)
	}
}
