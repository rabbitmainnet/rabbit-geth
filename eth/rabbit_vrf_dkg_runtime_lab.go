//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"errors"
	"fmt"
	"sync"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/log"
)

var errRabbitVRFDKGRuntimeV1 = errors.New(
	"invalid rabbit vrf dkg runtime v1",
)

type rabbitVRFDKGLocalContextV1 struct {
	HeadNumber       uint64
	HeadHash         common.Hash
	SessionID        common.Hash
	SourceWorkEpoch  uint64
	PreparationEpoch uint64
	TargetVRFEpoch   uint64
	Members          []lqc.RabbitVRFCommitteeMemberV1
}

// rabbitVRFDKGRuntime owns only local, non-consensus DKG observation state.
//
// At this stage it deliberately performs no:
//   - password reads;
//   - transport-key generation;
//   - wallet signing;
//   - private evaluation generation;
//   - P2P publication.
type rabbitVRFDKGRuntime struct {
	backend *Ethereum
	engine  *lqc.LQC

	mu      sync.RWMutex
	started bool
	stop    chan struct{}
	done    chan struct{}
	current rabbitVRFDKGLocalContextV1
}

func newRabbitVRFDKGRuntimeMaybeLab(
	backend *Ethereum,
	engine *lqc.LQC,
) (*rabbitVRFDKGRuntime, error) {
	if backend == nil ||
		backend.blockchain == nil ||
		backend.accountManager == nil ||
		engine == nil {
		return nil, errRabbitVRFDKGRuntimeV1
	}

	config := backend.blockchain.Config()
	if config == nil ||
		config.LQC == nil ||
		config.LQC.VRFProtocolBlock == 0 {
		return nil, nil
	}

	return &rabbitVRFDKGRuntime{
		backend: backend,
		engine:  engine,
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
	}, nil
}

func rabbitVRFDKGLocalWalletAddressesV1(
	manager *accounts.Manager,
) map[common.Address]struct{} {
	local := make(map[common.Address]struct{})

	if manager == nil {
		return local
	}

	for _, wallet := range manager.Wallets() {
		for _, account := range wallet.Accounts() {
			if account.Address != (common.Address{}) {
				local[account.Address] = struct{}{}
			}
		}
	}

	return local
}

func rabbitVRFDKGMatchLocalMembersV1(
	local map[common.Address]struct{},
	members []lqc.RabbitVRFCommitteeMemberV1,
) ([]lqc.RabbitVRFCommitteeMemberV1, error) {
	if len(members) == 0 {
		return nil, errRabbitVRFDKGRuntimeV1
	}

	seenParticipants := make(map[common.Address]struct{}, len(members))
	selected := make(
		[]lqc.RabbitVRFCommitteeMemberV1,
		0,
		len(members),
	)

	for index, member := range members {
		if member.ShareID != uint64(index)+1 ||
			member.Participant == (common.Address{}) ||
			member.TicketHash == (common.Hash{}) {
			return nil, errRabbitVRFDKGRuntimeV1
		}

		if _, exists := seenParticipants[member.Participant]; exists {
			return nil, errRabbitVRFDKGRuntimeV1
		}
		seenParticipants[member.Participant] = struct{}{}

		if _, exists := local[member.Participant]; !exists {
			continue
		}

		selected = append(
			selected,
			member,
		)
	}

	return selected, nil
}

func cloneRabbitVRFDKGMembersV1(
	members []lqc.RabbitVRFCommitteeMemberV1,
) []lqc.RabbitVRFCommitteeMemberV1 {
	if len(members) == 0 {
		return nil
	}

	out := make(
		[]lqc.RabbitVRFCommitteeMemberV1,
		len(members),
	)
	copy(out, members)
	return out
}

func (runtime *rabbitVRFDKGRuntime) setCurrent(
	context rabbitVRFDKGLocalContextV1,
) common.Hash {
	runtime.mu.Lock()
	previous := runtime.current.SessionID
	context.Members = cloneRabbitVRFDKGMembersV1(
		context.Members,
	)
	runtime.current = context
	runtime.mu.Unlock()

	return previous
}

func (runtime *rabbitVRFDKGRuntime) clearCurrent() {
	runtime.mu.Lock()
	runtime.current = rabbitVRFDKGLocalContextV1{}
	runtime.mu.Unlock()
}

func (runtime *rabbitVRFDKGRuntime) currentContext() rabbitVRFDKGLocalContextV1 {
	runtime.mu.RLock()
	context := runtime.current
	context.Members = cloneRabbitVRFDKGMembersV1(
		context.Members,
	)
	runtime.mu.RUnlock()

	return context
}

func (runtime *rabbitVRFDKGRuntime) processCurrentHead() error {
	if runtime == nil ||
		runtime.backend == nil ||
		runtime.backend.blockchain == nil ||
		runtime.backend.accountManager == nil ||
		runtime.engine == nil {
		return errRabbitVRFDKGRuntimeV1
	}

	header := runtime.backend.blockchain.CurrentHeader()
	if header == nil ||
		header.Number == nil ||
		!header.Number.IsUint64() ||
		header.Number.Sign() <= 0 {
		runtime.clearCurrent()
		return nil
	}

	blockNumber := header.Number.Uint64()

	bridge, ok, err :=
		runtime.engine.RabbitVRFDKGBridgeContextV1(
			runtime.backend.blockchain,
			blockNumber-1,
			header.ParentHash,
			blockNumber,
		)
	if err != nil {
		runtime.clearCurrent()
		return err
	}
	if !ok {
		runtime.clearCurrent()
		return nil
	}

	lifecycle, err :=
		runtime.engine.LoadRabbitVRFDKGLifecycleV1(
			bridge.SessionID,
		)
	if err != nil {
		runtime.clearCurrent()
		return fmt.Errorf(
			"load persisted rabbit vrf dkg lifecycle: %w",
			err,
		)
	}

	if lifecycle == nil ||
		lifecycle.Phase !=
			lqc.RabbitVRFDKGLifecyclePhasePreparedV1 ||
		lifecycle.SessionID != bridge.SessionID ||
		lifecycle.SourceWorkEpoch != bridge.SourceWorkEpoch ||
		lifecycle.PreparationEpoch != bridge.PreparationEpoch ||
		lifecycle.TargetVRFEpoch != bridge.TargetVRFEpoch ||
		lifecycle.CommitteeRoot != bridge.CommitteeRoot {
		runtime.clearCurrent()
		return errRabbitVRFDKGRuntimeV1
	}

	local :=
		rabbitVRFDKGLocalWalletAddressesV1(
			runtime.backend.accountManager,
		)

	members, err :=
		rabbitVRFDKGMatchLocalMembersV1(
			local,
			bridge.Members,
		)
	if err != nil {
		runtime.clearCurrent()
		return err
	}

	previous :=
		runtime.setCurrent(
			rabbitVRFDKGLocalContextV1{
				HeadNumber:       blockNumber,
				HeadHash:         header.Hash(),
				SessionID:        bridge.SessionID,
				SourceWorkEpoch:  bridge.SourceWorkEpoch,
				PreparationEpoch: bridge.PreparationEpoch,
				TargetVRFEpoch:   bridge.TargetVRFEpoch,
				Members:          members,
			},
		)

	if previous != bridge.SessionID {
		log.Info(
			"Rabbit VRF DKG canonical session observed",
			"session", bridge.SessionID,
			"sourceEpoch", bridge.SourceWorkEpoch,
			"preparationEpoch", bridge.PreparationEpoch,
			"targetEpoch", bridge.TargetVRFEpoch,
			"localMembers", len(members),
		)

		for _, member := range members {
			log.Info(
				"Rabbit VRF DKG local committee member resolved",
				"session", bridge.SessionID,
				"participant", member.Participant,
				"shareID", member.ShareID,
			)
		}
	}

	return nil
}

func (runtime *rabbitVRFDKGRuntime) Start() error {
	if runtime == nil {
		return nil
	}

	runtime.mu.Lock()
	if runtime.started {
		runtime.mu.Unlock()
		return nil
	}
	runtime.started = true
	runtime.mu.Unlock()

	headCh := make(chan core.ChainHeadEvent, 16)
	sub :=
		runtime.backend.blockchain.SubscribeChainHeadEvent(
			headCh,
		)

	go func() {
		defer close(runtime.done)
		defer sub.Unsubscribe()

		process := func() {
			if err := runtime.processCurrentHead(); err != nil {
				log.Debug(
					"Rabbit VRF DKG local runtime waiting",
					"err", err,
				)
			}
		}

		process()

		for {
			select {
			case <-runtime.stop:
				return
			case <-sub.Err():
				return
			case <-headCh:
				process()
			}
		}
	}()

	return nil
}

func (runtime *rabbitVRFDKGRuntime) Close() {
	if runtime == nil {
		return
	}

	runtime.mu.Lock()
	if !runtime.started {
		runtime.mu.Unlock()
		return
	}
	runtime.started = false
	close(runtime.stop)
	done := runtime.done
	runtime.mu.Unlock()

	<-done
}
