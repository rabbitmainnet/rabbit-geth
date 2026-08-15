package eth

import (
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/crypto/rabbitx"
	"github.com/ethereum/go-ethereum/ethdb"
)

var errLQCWorkV1NoCommitWindow = errors.New(
	"lqc Work V1 relay has no commit window at current height",
)

// newLQCWorkV1BackendTransport wires the lqcw relay to the active Work V1
// consensus runtime. Production activation is guarded by the frozen Rabbit
// genesis and the rabbit_workv1 build tag in backend.go.
//
// Work mined in epoch N is relayed/committed during epoch N+1. Therefore this
// provider intentionally uses WorkCommitTargetEpochV1(nextBlock). During the
// first work epoch there is no commit target yet, so RPC/relay admission is
// intentionally unavailable until the first commit window opens.
func newLQCWorkV1BackendTransport(
	blockchain *core.BlockChain,
	engine *lqc.LQC,
	chainID *big.Int,
	networkID uint64,
	database ethdb.KeyValueStore,
) (*lqcWorkV1Transport, error) {
	if blockchain == nil || engine == nil ||
		chainID == nil || chainID.Sign() <= 0 ||
		database == nil {
		return nil, errLQCWorkV1Context
	}

	hasher, err := rabbitx.NewLightHasher()
	if err != nil {
		return nil, err
	}
	journal, err := newLQCWorkV1PoolJournal(
		database,
		blockchain.Genesis().Hash(),
	)
	if err != nil {
		hasher.Close()
		return nil, err
	}

	context := func() (lqcWorkV1Context, error) {
		head := blockchain.CurrentHeader()
		if head == nil || head.Number == nil || head.Number.Sign() < 0 {
			return lqcWorkV1Context{}, errLQCWorkV1Context
		}
		if head.Number.Uint64() == ^uint64(0) {
			return lqcWorkV1Context{}, errLQCWorkV1Context
		}

		nextBlock := head.Number.Uint64() + 1
		epoch,
			datasetAnchor,
			challengeAnchor,
			difficulty,
			eligibility,
			err := engine.WorkV1EngineLabRelayContext(
			blockchain,
			head.Number.Uint64(),
			head.Hash(),
			nextBlock,
		)
		if err != nil {
			return lqcWorkV1Context{}, err
		}
		if epoch == 0 {
			return lqcWorkV1Context{}, errLQCWorkV1NoCommitWindow
		}

		return lqcWorkV1Context{
			Epoch:           epoch,
			DatasetAnchor:   datasetAnchor,
			ChallengeAnchor: challengeAnchor,
			Difficulty:      difficulty,
			Eligibility:     eligibility,
		}, nil
	}

	transport, err := newLQCWorkV1Transport(
		lqcWorkV1TransportConfig{
			Enabled:         true,
			ChainID:         chainID,
			NetworkID:       networkID,
			Genesis:         blockchain.Genesis().Hash(),
			Context:         context,
			Hasher:          hasher.Hash,
			PoolPersistence: journal,
		},
	)
	if err != nil {
		hasher.Close()
		return nil, err
	}
	return transport, nil
}
