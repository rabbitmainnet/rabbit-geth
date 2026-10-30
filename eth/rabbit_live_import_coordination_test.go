package eth

import (
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/ethash"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
)

type rabbitHeldVerification struct {
	consensus.Engine
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (e *rabbitHeldVerification) VerifyHeaders(
	chain consensus.ChainHeaderReader, headers []*types.Header,
) (chan<- struct{}, <-chan error) {
	e.once.Do(func() { close(e.started) })
	<-e.release
	return e.Engine.VerifyHeaders(chain, headers)
}

func TestRabbitRecoveryWaitsForActualLiveImport(t *testing.T) {
	base := ethash.NewFullFaker()
	defer base.Close()
	genesis := &core.Genesis{Config: params.AllEthashProtocolChanges}
	db, blocks, _ := core.GenerateChainWithGenesis(genesis, base, 1, nil)
	defer db.Close()

	engine := &rabbitHeldVerification{
		Engine:  base,
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(engine.release) }) }

	chain, err := core.NewBlockChain(db, genesis, engine, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer chain.Stop()
	defer release()

	h := (*ethHandler)(&handler{
		chain:    chain,
		quitSync: make(chan struct{}),
	})
	imported := make(chan error, 1)
	go func() {
		_, err := h.insertLQCLiveBlock(blocks[0])
		imported <- err
	}()

	select {
	case <-engine.started:
	case <-time.After(5 * time.Second):
		t.Fatal("live import did not reach header verification")
	}

	recovered := make(chan error, 1)
	launched := make(chan struct{})
	go func() {
		close(launched)
		recovered <- h.startLQCRecoveryAfterLiveImport(blocks[0].Header())
	}()
	<-launched

	select {
	case err := <-recovered:
		t.Fatalf("recovery returned before live import completed: %v", err)
	case <-time.After(750 * time.Millisecond):
	}

	release()
	select {
	case err := <-imported:
		if err != nil {
			t.Fatalf("live import failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("live import did not complete")
	}

	select {
	case err := <-recovered:
		if err != nil {
			t.Fatalf("recovery recheck failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("recovery did not retire the imported target")
	}

	if chain.CurrentBlock().Hash() != blocks[0].Hash() {
		t.Fatal("imported block is not canonical")
	}
	t.Log("Real block imported; recovery waited and retired the canonical target without starting downloader.")
}
