//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/eth/downloader"
	"github.com/ethereum/go-ethereum/internal/rabbitvrfstate"
	"github.com/ethereum/go-ethereum/log"
)

var errRabbitVRFDKGRuntimeV1 = errors.New(
	"invalid rabbit vrf dkg runtime v1",
)

type rabbitVRFDKGLocalContextV1 struct {
	HeadNumber                   uint64
	HeadHash                     common.Hash
	SessionID                    common.Hash
	SourceWorkEpoch              uint64
	PreparationEpoch             uint64
	TargetVRFEpoch               uint64
	CanonicalSession             lqc.RabbitVRFDKGSessionContextV1
	CanonicalMembers             []lqc.RabbitVRFCommitteeMemberV1
	Members                      []lqc.RabbitVRFCommitteeMemberV1
	TransportBindings            []lqc.RabbitVRFDKGTransportKeyBindingV1
	TransportEnvelopes           []lqc.RabbitVRFDKGEnvelopeV1
	PolynomialCommitments        []lqc.RabbitVRFDKGPolynomialCommitmentV1
	PolynomialEnvelopes          []lqc.RabbitVRFDKGEnvelopeV1
	CanonicalTransportKeySetRoot common.Hash
	CanonicalTransportBindings   []lqc.RabbitVRFDKGTransportKeyBindingV1
	CanonicalTransportEnvelopes  []lqc.RabbitVRFDKGEnvelopeV1
}

// rabbitVRFDKGRuntime owns local, non-consensus DKG runtime state.
//
// For canonical local committee members, the secret-ready path may read the
// explicit DKG password file and load or create an encrypted transport key.
// Only the public transport binding is retained in runtime state.
//
// It still deliberately performs no:
//   - wallet signing;
//   - private evaluation generation;
//   - P2P publication.
type rabbitVRFDKGRuntime struct {
	backend *Ethereum
	engine  *lqc.LQC

	secretGate sync.RWMutex

	mu          sync.RWMutex
	started     bool
	syncing     bool
	secretReady bool
	stop        chan struct{}
	done        chan struct{}
	current     rabbitVRFDKGLocalContextV1

	evaluationDispatch rabbitVRFDKGEvaluationDispatchV1
	localSharesReady   rabbitVRFDKGLocalSharesReadyV1

	canonicalRequestLookup func(common.Hash) (rabbitVRFCanonicalRequestV1, error)
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

func cloneRabbitVRFDKGTransportBindingsV1(
	bindings []lqc.RabbitVRFDKGTransportKeyBindingV1,
) []lqc.RabbitVRFDKGTransportKeyBindingV1 {
	if len(bindings) == 0 {
		return nil
	}

	out := make(
		[]lqc.RabbitVRFDKGTransportKeyBindingV1,
		len(bindings),
	)
	copy(out, bindings)
	return out
}

func cloneRabbitVRFDKGTransportEnvelopesV1(
	envelopes []lqc.RabbitVRFDKGEnvelopeV1,
) []lqc.RabbitVRFDKGEnvelopeV1 {
	if len(envelopes) == 0 {
		return nil
	}

	out := make(
		[]lqc.RabbitVRFDKGEnvelopeV1,
		len(envelopes),
	)
	for index := range envelopes {
		out[index] = envelopes[index]
		out[index].Signature = append(
			[]byte(nil),
			envelopes[index].Signature...,
		)
	}
	return out
}

func (runtime *rabbitVRFDKGRuntime) setCanonicalTransportKeySetV1(
	sessionID common.Hash,
	root common.Hash,
	members []lqc.RabbitVRFCommitteeMemberV1,
	bindings []lqc.RabbitVRFDKGTransportKeyBindingV1,
	envelopes []lqc.RabbitVRFDKGEnvelopeV1,
) bool {
	if sessionID == (common.Hash{}) ||
		root == (common.Hash{}) ||
		len(members) == 0 ||
		len(bindings) != len(members) ||
		len(envelopes) != len(members) {
		return false
	}

	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	if runtime.current.SessionID != sessionID ||
		len(runtime.current.CanonicalMembers) != len(members) {
		return false
	}

	for index := range members {
		if runtime.current.CanonicalMembers[index] != members[index] {
			return false
		}
	}

	runtime.current.CanonicalTransportKeySetRoot = root
	runtime.current.CanonicalTransportBindings =
		cloneRabbitVRFDKGTransportBindingsV1(bindings)
	runtime.current.CanonicalTransportEnvelopes =
		cloneRabbitVRFDKGTransportEnvelopesV1(envelopes)

	return true
}

func (runtime *rabbitVRFDKGRuntime) setTransportArtifactsV1(
	sessionID common.Hash,
	bindings []lqc.RabbitVRFDKGTransportKeyBindingV1,
	envelopes []lqc.RabbitVRFDKGEnvelopeV1,
) bool {
	if len(bindings) == 0 ||
		len(bindings) != len(envelopes) {
		return false
	}

	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	if runtime.current.SessionID != sessionID {
		return false
	}

	runtime.current.TransportBindings =
		cloneRabbitVRFDKGTransportBindingsV1(bindings)
	runtime.current.TransportEnvelopes =
		cloneRabbitVRFDKGTransportEnvelopesV1(envelopes)

	return true
}

func (runtime *rabbitVRFDKGRuntime) setTransportBindingsV1(
	sessionID common.Hash,
	bindings []lqc.RabbitVRFDKGTransportKeyBindingV1,
) bool {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	if runtime.current.SessionID != sessionID {
		return false
	}

	runtime.current.TransportBindings =
		cloneRabbitVRFDKGTransportBindingsV1(bindings)

	return true
}

func (runtime *rabbitVRFDKGRuntime) transportArtifactsReadyV1(
	context lqc.RabbitVRFDKGSessionContextV1,
	sessionID common.Hash,
	members []lqc.RabbitVRFCommitteeMemberV1,
) bool {
	runtime.mu.RLock()
	defer runtime.mu.RUnlock()

	bindings := runtime.current.TransportBindings
	envelopes := runtime.current.TransportEnvelopes

	if runtime.current.SessionID != sessionID ||
		len(members) == 0 ||
		len(bindings) != len(members) ||
		len(envelopes) != len(members) {
		return false
	}

	for index := range members {
		if err := lqc.VerifyRabbitVRFDKGTransportKeyEnvelopeV1(
			context,
			members[index],
			bindings[index],
			envelopes[index],
		); err != nil {
			return false
		}
	}

	return true
}

func (runtime *rabbitVRFDKGRuntime) transportBindingsReadyV1(
	sessionID common.Hash,
	members []lqc.RabbitVRFCommitteeMemberV1,
) bool {
	runtime.mu.RLock()
	defer runtime.mu.RUnlock()

	bindings := runtime.current.TransportBindings
	if runtime.current.SessionID != sessionID ||
		len(bindings) != len(members) ||
		len(bindings) == 0 {
		return false
	}

	for index := range members {
		if bindings[index].SessionID != sessionID ||
			bindings[index].ShareID != members[index].ShareID ||
			bindings[index].Participant != members[index].Participant {
			return false
		}
	}

	return true
}

func (runtime *rabbitVRFDKGRuntime) setCurrent(
	context rabbitVRFDKGLocalContextV1,
) common.Hash {
	runtime.mu.Lock()
	previous := runtime.current.SessionID
	if previous == context.SessionID {
		context.TransportBindings =
			cloneRabbitVRFDKGTransportBindingsV1(
				runtime.current.TransportBindings,
			)
		context.TransportEnvelopes =
			cloneRabbitVRFDKGTransportEnvelopesV1(
				runtime.current.TransportEnvelopes,
			)
		context.PolynomialCommitments = cloneRabbitVRFDKGPolynomialCommitmentsV1(runtime.current.PolynomialCommitments)
		context.PolynomialEnvelopes = cloneRabbitVRFDKGTransportEnvelopesV1(runtime.current.PolynomialEnvelopes)
		context.CanonicalTransportKeySetRoot = runtime.current.CanonicalTransportKeySetRoot
		context.CanonicalTransportBindings = cloneRabbitVRFDKGTransportBindingsV1(runtime.current.CanonicalTransportBindings)
		context.CanonicalTransportEnvelopes = cloneRabbitVRFDKGTransportEnvelopesV1(runtime.current.CanonicalTransportEnvelopes)
	}
	context.Members = cloneRabbitVRFDKGMembersV1(
		context.Members,
	)
	context.CanonicalMembers = cloneRabbitVRFDKGMembersV1(
		context.CanonicalMembers,
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
	context.CanonicalMembers = cloneRabbitVRFDKGMembersV1(
		context.CanonicalMembers,
	)
	context.TransportBindings =
		cloneRabbitVRFDKGTransportBindingsV1(
			context.TransportBindings,
		)
	context.TransportEnvelopes =
		cloneRabbitVRFDKGTransportEnvelopesV1(
			context.TransportEnvelopes,
		)
	context.PolynomialCommitments = cloneRabbitVRFDKGPolynomialCommitmentsV1(context.PolynomialCommitments)
	context.PolynomialEnvelopes = cloneRabbitVRFDKGTransportEnvelopesV1(context.PolynomialEnvelopes)
	context.CanonicalTransportBindings = cloneRabbitVRFDKGTransportBindingsV1(context.CanonicalTransportBindings)
	context.CanonicalTransportEnvelopes = cloneRabbitVRFDKGTransportEnvelopesV1(context.CanonicalTransportEnvelopes)
	runtime.mu.RUnlock()

	return context
}

func (runtime *rabbitVRFDKGRuntime) setSyncingV1(syncing bool) {
	if runtime == nil {
		return
	}

	runtime.secretGate.Lock()
	defer runtime.secretGate.Unlock()

	runtime.mu.Lock()
	runtime.syncing = syncing
	if syncing {
		runtime.secretReady = false
	}
	runtime.mu.Unlock()
}

func (runtime *rabbitVRFDKGRuntime) secretReadyV1() bool {
	if runtime == nil {
		return false
	}

	runtime.mu.RLock()
	ready := runtime.secretReady
	runtime.mu.RUnlock()

	return ready
}

func (runtime *rabbitVRFDKGRuntime) beginSecretOperationV1() bool {
	if runtime == nil {
		return false
	}

	var downloaderGuard bool

	if runtime.backend != nil {
		if runtime.backend.handler == nil ||
			runtime.backend.handler.downloader == nil {
			return false
		}
		if !runtime.backend.handler.downloader.TryAcquireSyncIdleGuard() {
			return false
		}
		downloaderGuard = true
	}

	runtime.secretGate.RLock()

	runtime.mu.RLock()
	ready := runtime.secretReady && !runtime.syncing
	runtime.mu.RUnlock()

	if !ready {
		runtime.secretGate.RUnlock()
		if downloaderGuard {
			runtime.backend.handler.downloader.ReleaseSyncIdleGuard()
		}
		return false
	}

	return true
}

func (runtime *rabbitVRFDKGRuntime) endSecretOperationV1() {
	if runtime == nil {
		return
	}

	runtime.secretGate.RUnlock()

	if runtime.backend != nil &&
		runtime.backend.handler != nil &&
		runtime.backend.handler.downloader != nil {
		runtime.backend.handler.downloader.ReleaseSyncIdleGuard()
	}
}

// refreshSecretReadyV1 returns true only when the runtime transitions from
// not-ready to ready. Synced() is intentionally not sufficient by itself
// because it is sticky after the first successful synchronization.
func (runtime *rabbitVRFDKGRuntime) refreshSecretReadyV1() bool {
	if runtime == nil ||
		runtime.backend == nil ||
		runtime.backend.blockchain == nil {
		return false
	}

	runtime.mu.RLock()
	syncing := runtime.syncing
	previous := runtime.secretReady
	runtime.mu.RUnlock()

	ready := false

	if !syncing && runtime.backend.Synced() {
		header := runtime.backend.blockchain.CurrentHeader()

		if header != nil &&
			header.Number != nil &&
			header.Number.IsUint64() &&
			runtime.backend.lqcHeadStateAvailable(header) {
			ready = true
		}
	}

	runtime.mu.Lock()

	// A SyncStarted event may have arrived while canonical state was checked.
	if runtime.syncing {
		ready = false
	}

	previous = runtime.secretReady
	runtime.secretReady = ready

	runtime.mu.Unlock()

	return ready && !previous
}

func (runtime *rabbitVRFDKGRuntime) ensureTransportBindingsV1(
	bridge lqc.RabbitVRFDKGBridgeV1,
	members []lqc.RabbitVRFCommitteeMemberV1,
) error {
	if runtime == nil || runtime.backend == nil {
		return errRabbitVRFDKGRuntimeV1
	}
	if len(members) == 0 {
		return nil
	}
	if runtime.transportArtifactsReadyV1(
		bridge.Session,
		bridge.SessionID,
		members,
	) {
		return nil
	}

	if !runtime.beginSecretOperationV1() {
		return nil
	}
	defer runtime.endSecretOperationV1()

	// Recheck after entering the full anti-TOCTOU gate.
	if runtime.transportArtifactsReadyV1(
		bridge.Session,
		bridge.SessionID,
		members,
	) {
		return nil
	}

	if runtime.backend.config == nil ||
		runtime.backend.accountManager == nil ||
		runtime.backend.vrfDKGInstanceDir == "" ||
		runtime.backend.p2pServer == nil ||
		runtime.backend.p2pServer.PrivateKey == nil {
		return errRabbitVRFDKGRuntimeV1
	}

	password, err :=
		readRabbitVRFDKGPasswordFileV1(
			runtime.backend.config.RabbitVRFDKGPasswordFile,
		)
	if err != nil {
		return fmt.Errorf(
			"read rabbit vrf dkg password file: %w",
			err,
		)
	}

	store, err :=
		rabbitvrfstate.NewStandardDKGTransportKeyStoreV1(
			filepath.Join(
				runtime.backend.vrfDKGInstanceDir,
				"rabbit-vrf",
				"dkg-transport",
			),
		)
	if err != nil {
		return fmt.Errorf(
			"open rabbit vrf dkg transport store: %w",
			err,
		)
	}

	bindings := make(
		[]lqc.RabbitVRFDKGTransportKeyBindingV1,
		0,
		len(members),
	)
	envelopes := make(
		[]lqc.RabbitVRFDKGEnvelopeV1,
		0,
		len(members),
	)

	wallets := runtime.backend.accountManager.Wallets()

	for _, member := range members {
		binding, err :=
			rabbitVRFDKGLoadOrCreateTransportBindingV1(
				store,
				bridge.Session,
				member,
				password,
				runtime.backend.p2pServer.PrivateKey,
			)
		if err != nil {
			return fmt.Errorf(
				"prepare rabbit vrf dkg transport key share %d: %w",
				member.ShareID,
				err,
			)
		}

		envelope, err :=
			rabbitVRFDKGSignTransportBindingEnvelopeV1(
				wallets,
				bridge.Session,
				member,
				binding,
			)
		if err != nil {
			return fmt.Errorf(
				"authenticate rabbit vrf dkg transport key share %d: %w",
				member.ShareID,
				err,
			)
		}

		bindings = append(bindings, binding)
		envelopes = append(envelopes, envelope)
	}

	if !runtime.setTransportArtifactsV1(
		bridge.SessionID,
		bindings,
		envelopes,
	) {
		return errRabbitVRFDKGRuntimeV1
	}

	if runtime.backend.vrfDKGTransport != nil {

		if _, err := runtime.backend.vrfDKGTransport.persistCanonicalTransportKeySetV1(); err != nil {

			return fmt.Errorf("persist rabbit vrf dkg canonical transport key set: %w", err)

		}

	}

	log.Info(
		"Rabbit VRF DKG local authenticated transport artifacts ready",
		"session", bridge.SessionID,
		"bindings", len(bindings),
		"envelopes", len(envelopes),
	)
	return nil
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

	if !lqc.RabbitVRFDKGLifecycleMatchesBridgeV1(
		lifecycle,
		bridge,
	) {
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
				CanonicalSession: bridge.Session,
				CanonicalMembers: bridge.Members,
				Members:          members,
			},
		)

	persistedKeySet, err := runtime.engine.LoadRabbitVRFDKGTransportKeySetStateV1(
		bridge.SessionID,
	)
	if err != nil {
		runtime.clearCurrent()
		return fmt.Errorf(
			"load persisted rabbit vrf dkg transport key set: %w",
			err,
		)
	}

	if persistedKeySet != nil {
		if !runtime.setCanonicalTransportKeySetV1(
			persistedKeySet.SessionID,
			persistedKeySet.Root,
			persistedKeySet.Members,
			persistedKeySet.Bindings,
			persistedKeySet.Envelopes,
		) {
			runtime.clearCurrent()
			return errRabbitVRFDKGRuntimeV1
		}
	}

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

	if err := runtime.ensureTransportBindingsV1(
		bridge,
		members,
	); err != nil {
		return fmt.Errorf(
			"prepare local rabbit vrf dkg transport bindings: %w",
			err,
		)
	}

	if err := runtime.ensurePolynomialCommitmentsV1(
		bridge,
		members,
	); err != nil {
		return fmt.Errorf(
			"prepare local rabbit vrf dkg polynomial commitments: %w",
			err,
		)
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
	headSub :=
		runtime.backend.blockchain.SubscribeChainHeadEvent(
			headCh,
		)

	if runtime.backend.handler == nil ||
		runtime.backend.handler.downloader == nil {
		headSub.Unsubscribe()
		return errRabbitVRFDKGRuntimeV1
	}

	syncCh := make(chan downloader.SyncEvent, 16)
	syncSub :=
		runtime.backend.handler.downloader.SubscribeSyncEvents(
			syncCh,
		)

	go func() {
		defer close(runtime.done)
		defer headSub.Unsubscribe()
		defer syncSub.Unsubscribe()

		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		requestScan := rabbitVRFRequestScanCursorV1{
			Next: runtime.rabbitVRFRequestScanStartV1(),
		}

		process := func() {
			if err := runtime.processCurrentHead(); err != nil {
				log.Debug(
					"Rabbit VRF DKG local runtime waiting",
					"err", err,
				)
			}
		}

		processRequests := func() {
			if err := runtime.dispatchLocalEvaluationsV1(); err != nil {
				log.Debug("Rabbit VRF DKG evaluation dispatch waiting", "err", err)
			}
			ready, err := runtime.ensureFinalKeysetV1()
			if err != nil {
				log.Debug("Rabbit VRF DKG final keyset waiting", "err", err)
				return
			}
			if !ready {
				return
			}

			sharesReady, shareErr := runtime.ensureLocalSecretSharesV1()
			if shareErr != nil {
				log.Debug("Rabbit VRF DKG local secret shares waiting", "err", shareErr)
				return
			}
			if !sharesReady {
				return
			}
			if err := runtime.processCanonicalPendingRequestsV1(&requestScan); err != nil {
				log.Debug(
					"Rabbit VRF canonical request worker waiting",
					"next", requestScan.Next,
					"err", err,
				)
			}
		}

		refresh := func() {
			if runtime.refreshSecretReadyV1() {
				log.Info(
					"Rabbit VRF DKG secret runtime ready",
				)
				process()
			}
		}

		process()
		refresh()
		processRequests()

		for {
			select {
			case <-runtime.stop:
				return

			case <-headSub.Err():
				return

			case <-syncSub.Err():
				return

			case <-headCh:
				process()
				refresh()
				processRequests()

			case ev, ok := <-syncCh:
				if !ok {
					return
				}

				switch ev.Type {
				case downloader.SyncStarted:
					runtime.setSyncingV1(true)

				case downloader.SyncFailed:
					// Failed synchronization remains fail-closed until a
					// later successful downloader cycle.
					runtime.setSyncingV1(true)

				case downloader.SyncCompleted:
					runtime.setSyncingV1(false)
					refresh()
					processRequests()
				}

			case <-ticker.C:
				refresh()
				processRequests()
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
