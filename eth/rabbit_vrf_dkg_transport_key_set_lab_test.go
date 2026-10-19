//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"crypto/ecdsa"
	"errors"
	"math/big"
	"sync"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/params"
)

type rabbitVRFDKGBlockingDatabaseV1 struct {
	ethdb.Database

	mu      sync.Mutex
	armed   bool
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func newRabbitVRFDKGBlockingDatabaseV1(
	db ethdb.Database,
) *rabbitVRFDKGBlockingDatabaseV1 {
	return &rabbitVRFDKGBlockingDatabaseV1{
		Database: db,
		entered:  make(chan struct{}),
		release:  make(chan struct{}),
	}
}

func (db *rabbitVRFDKGBlockingDatabaseV1) arm() {
	db.mu.Lock()
	db.armed = true
	db.mu.Unlock()
}

func (db *rabbitVRFDKGBlockingDatabaseV1) Put(
	key []byte,
	value []byte,
) error {
	db.mu.Lock()
	armed := db.armed
	entered := db.entered
	release := db.release
	db.mu.Unlock()

	if armed {
		db.once.Do(func() {
			close(entered)
		})
		<-release
	}

	return db.Database.Put(key, value)
}

func rabbitVRFDKGCanonicalTransportArtifactV1(
	t *testing.T,
	context lqc.RabbitVRFDKGSessionContextV1,
	member lqc.RabbitVRFCommitteeMemberV1,
	participantKey *ecdsa.PrivateKey,
) rabbitVRFDKGTransportArtifactPacketV1 {
	t.Helper()

	transportKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}

	publicKey, err :=
		lqc.RabbitVRFDKGTransportPublicKeyV1FromBytes(
			crypto.CompressPubkey(
				&transportKey.PublicKey,
			),
		)
	if err != nil {
		t.Fatal(err)
	}

	binding, bindingRoot, err :=
		lqc.NewRabbitVRFDKGTransportKeyBindingV1(
			context,
			member,
			publicKey,
		)
	if err != nil {
		t.Fatal(err)
	}

	envelope, err := lqc.NewRabbitVRFDKGEnvelopeV1(
		context,
		member,
		lqc.RabbitVRFDKGMessageTransportKeyBindingV1,
		bindingRoot,
	)
	if err != nil {
		t.Fatal(err)
	}

	signingHash, err :=
		lqc.RabbitVRFDKGEnvelopeSigningHashV1(
			context,
			envelope,
		)
	if err != nil {
		t.Fatal(err)
	}

	envelope.Signature, err = crypto.Sign(
		signingHash[:],
		participantKey,
	)
	if err != nil {
		t.Fatal(err)
	}

	return rabbitVRFDKGTransportArtifactPacketV1{
		Binding:  binding,
		Envelope: envelope,
	}
}

func newRabbitVRFDKGCanonicalTransportKeySetFixtureV1(
	t *testing.T,
) (
	*rabbitVRFDKGTransport,
	[]lqc.RabbitVRFCommitteeMemberV1,
	[]rabbitVRFDKGTransportArtifactPacketV1,
	[]*ecdsa.PrivateKey,
) {
	t.Helper()

	context, err := lqc.NewRabbitVRFDKGSessionContextV1(
		big.NewInt(9280),
		21,
		crypto.Keccak256Hash(
			[]byte("rabbit-vrf-canonical-transport-key-set-committee"),
		),
		2,
	)
	if err != nil {
		t.Fatal(err)
	}

	sessionID, err := lqc.RabbitVRFDKGSessionIDV1(context)
	if err != nil {
		t.Fatal(err)
	}

	participantKeys := make([]*ecdsa.PrivateKey, 2)
	members := make([]lqc.RabbitVRFCommitteeMemberV1, 2)
	packets := make(
		[]rabbitVRFDKGTransportArtifactPacketV1,
		2,
	)

	for index := range members {
		participantKey, err := crypto.GenerateKey()
		if err != nil {
			t.Fatal(err)
		}

		shareID := uint64(index + 1)
		member := lqc.RabbitVRFCommitteeMemberV1{
			ShareID: shareID,
			TicketHash: crypto.Keccak256Hash(
				[]byte{
					byte(shareID),
					byte(shareID >> 8),
					0x5a,
				},
			),
			Participant: crypto.PubkeyToAddress(
				participantKey.PublicKey,
			),
		}

		participantKeys[index] = participantKey
		members[index] = member
		packets[index] =
			rabbitVRFDKGCanonicalTransportArtifactV1(
				t,
				context,
				member,
				participantKey,
			)
	}

	runtime := &rabbitVRFDKGRuntime{
		current: rabbitVRFDKGLocalContextV1{
			SessionID:        sessionID,
			CanonicalSession: context,
			CanonicalMembers: append(
				[]lqc.RabbitVRFCommitteeMemberV1(nil),
				members...,
			),
			Members: []lqc.RabbitVRFCommitteeMemberV1{
				members[0],
			},
			TransportBindings: []lqc.RabbitVRFDKGTransportKeyBindingV1{
				packets[0].Binding,
			},
			TransportEnvelopes: []lqc.RabbitVRFDKGEnvelopeV1{
				packets[0].Envelope,
			},
		},
	}

	transport, err := newRabbitVRFDKGTransport(
		rabbitVRFDKGTransportConfig{
			ChainID:   big.NewInt(9280),
			NetworkID: 9280,
			Genesis: crypto.Keccak256Hash(
				[]byte("rabbit-vrf-canonical-transport-key-set-genesis"),
			),
			Runtime: runtime,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	return transport, members, packets, participantKeys
}

func TestRabbitVRFDKGTransportV1CompleteKeySetPersists(
	t *testing.T,
) {
	chainID := big.NewInt(9280)
	sourceEpoch := uint64(18)

	preparationEpoch, err :=
		lqc.RabbitVRFDKGPreparationEpochForSourceV1(
			sourceEpoch,
		)
	if err != nil {
		t.Fatal(err)
	}

	targetEpoch, err :=
		lqc.RabbitVRFTargetEpochForSourceWorkEpochV1(
			sourceEpoch,
		)
	if err != nil {
		t.Fatal(err)
	}

	selectionRoot := crypto.Keccak256Hash(
		[]byte("rabbit-vrf-persist-selection-root"),
	)
	committeeSeed := crypto.Keccak256Hash(
		[]byte("rabbit-vrf-persist-committee-seed"),
	)

	participantKeys := make([]*ecdsa.PrivateKey, 2)
	seats := make([]lqc.WorkSeatV1, 2)

	for index := range seats {
		key, err := crypto.GenerateKey()
		if err != nil {
			t.Fatal(err)
		}

		participantKeys[index] = key
		seats[index] = lqc.WorkSeatV1{
			TicketHash: crypto.Keccak256Hash(
				[]byte{byte(index + 1), 0x71},
			),
			Participant: crypto.PubkeyToAddress(
				key.PublicKey,
			),
		}
	}

	committeeRoot, members, err :=
		lqc.RabbitVRFCommitteeRootV1(
			chainID,
			sourceEpoch,
			selectionRoot,
			committeeSeed,
			seats,
		)
	if err != nil {
		t.Fatal(err)
	}

	session, err :=
		lqc.NewRabbitVRFDKGSessionContextV1(
			chainID,
			targetEpoch,
			committeeRoot,
			uint64(len(members)),
		)
	if err != nil {
		t.Fatal(err)
	}

	sessionID, err := lqc.RabbitVRFDKGSessionIDV1(session)
	if err != nil {
		t.Fatal(err)
	}

	bridge := lqc.RabbitVRFDKGBridgeV1{
		SourceWorkEpoch:  sourceEpoch,
		PreparationEpoch: preparationEpoch,
		TargetVRFEpoch:   targetEpoch,
		SelectionRoot:    selectionRoot,
		DatasetKey: crypto.Keccak256Hash(
			[]byte("rabbit-vrf-persist-dataset-key"),
		),
		Entropy: crypto.Keccak256Hash(
			[]byte("rabbit-vrf-persist-entropy"),
		),
		CommitteeSeed: committeeSeed,
		CommitteeRoot: committeeRoot,
		Members:       members,
		Session:       session,
		SessionID:     sessionID,
	}

	db := rawdb.NewMemoryDatabase()
	engine := lqc.New(&params.LQCConfig{}, db)

	if _, created, err :=
		engine.EnsureRabbitVRFDKGLifecycleV1(
			bridge,
		); err != nil {
		t.Fatal(err)
	} else if !created {
		t.Fatal("lifecycle was not created")
	}

	packets := make(
		[]rabbitVRFDKGTransportArtifactPacketV1,
		len(members),
	)
	for index := range members {
		packets[index] =
			rabbitVRFDKGCanonicalTransportArtifactV1(
				t,
				session,
				members[index],
				participantKeys[index],
			)
	}

	runtime := &rabbitVRFDKGRuntime{
		engine: engine,
		current: rabbitVRFDKGLocalContextV1{
			SessionID:        sessionID,
			CanonicalSession: session,
			CanonicalMembers: cloneRabbitVRFDKGMembersV1(members),
			Members: []lqc.RabbitVRFCommitteeMemberV1{
				members[0],
			},
			TransportBindings: []lqc.RabbitVRFDKGTransportKeyBindingV1{
				packets[0].Binding,
			},
			TransportEnvelopes: []lqc.RabbitVRFDKGEnvelopeV1{
				packets[0].Envelope,
			},
		},
	}

	transport, err := newRabbitVRFDKGTransport(
		rabbitVRFDKGTransportConfig{
			ChainID:   chainID,
			NetworkID: 9280,
			Genesis: crypto.Keccak256Hash(
				[]byte("rabbit-vrf-persist-genesis"),
			),
			Runtime: runtime,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	inserted, err :=
		transport.storeRemoteTransportArtifactV1(
			packets[1],
		)
	if err != nil {
		t.Fatal(err)
	}
	if !inserted {
		t.Fatal("remote canonical transport artifact was not inserted")
	}

	persistedNow, err :=
		transport.persistCanonicalTransportKeySetV1()
	if err != nil {
		t.Fatal(err)
	}
	if !persistedNow {
		t.Fatal("complete canonical transport key set was not persisted")
	}

	loaded, err :=
		engine.LoadRabbitVRFDKGTransportKeySetStateV1(
			sessionID,
		)
	if err != nil {
		t.Fatal(err)
	}
	if loaded == nil {
		t.Fatal("persisted canonical transport key set was not found")
	}

	got := runtime.currentContext()
	if got.CanonicalTransportKeySetRoot != loaded.Root {
		t.Fatal("runtime canonical transport root differs from persisted root")
	}
	if len(got.CanonicalTransportBindings) != len(members) ||
		len(got.CanonicalTransportEnvelopes) != len(members) {
		t.Fatal("runtime canonical transport key set is incomplete")
	}

	restartedEngine := lqc.New(&params.LQCConfig{}, db)

	restartedLoaded, err :=
		restartedEngine.LoadRabbitVRFDKGTransportKeySetStateV1(
			sessionID,
		)
	if err != nil {
		t.Fatal(err)
	}
	if restartedLoaded == nil {
		t.Fatal("restarted engine did not recover persisted canonical transport key set")
	}

	restartedRuntime := &rabbitVRFDKGRuntime{
		engine: restartedEngine,
		current: rabbitVRFDKGLocalContextV1{
			SessionID:        sessionID,
			CanonicalSession: session,
			CanonicalMembers: cloneRabbitVRFDKGMembersV1(members),
		},
	}

	if !restartedRuntime.setCanonicalTransportKeySetV1(
		restartedLoaded.SessionID,
		restartedLoaded.Root,
		restartedLoaded.Members,
		restartedLoaded.Bindings,
		restartedLoaded.Envelopes,
	) {
		t.Fatal("restarted runtime rejected persisted canonical transport key set")
	}

	restartedGot := restartedRuntime.currentContext()
	if restartedGot.CanonicalTransportKeySetRoot != loaded.Root {
		t.Fatal("restarted runtime canonical transport root mismatch")
	}
	if len(restartedGot.CanonicalTransportBindings) != len(members) ||
		len(restartedGot.CanonicalTransportEnvelopes) != len(members) {
		t.Fatal("restarted runtime did not recover complete canonical transport key set")
	}

}

func TestRabbitVRFDKGTransportV1SessionSwitchDuringPersistDoesNotPublishOldSet(
	t *testing.T,
) {
	chainID := big.NewInt(9280)
	sourceEpoch := uint64(18)

	preparationEpoch, err :=
		lqc.RabbitVRFDKGPreparationEpochForSourceV1(
			sourceEpoch,
		)
	if err != nil {
		t.Fatal(err)
	}

	targetEpoch, err :=
		lqc.RabbitVRFTargetEpochForSourceWorkEpochV1(
			sourceEpoch,
		)
	if err != nil {
		t.Fatal(err)
	}

	selectionRoot := crypto.Keccak256Hash(
		[]byte("rabbit-vrf-switch-selection-root"),
	)
	committeeSeed := crypto.Keccak256Hash(
		[]byte("rabbit-vrf-switch-committee-seed"),
	)

	participantKeys := make([]*ecdsa.PrivateKey, 2)
	seats := make([]lqc.WorkSeatV1, 2)

	for index := range seats {
		key, err := crypto.GenerateKey()
		if err != nil {
			t.Fatal(err)
		}

		participantKeys[index] = key
		seats[index] = lqc.WorkSeatV1{
			TicketHash: crypto.Keccak256Hash(
				[]byte{byte(index + 1), 0x72},
			),
			Participant: crypto.PubkeyToAddress(
				key.PublicKey,
			),
		}
	}

	committeeRoot, members, err :=
		lqc.RabbitVRFCommitteeRootV1(
			chainID,
			sourceEpoch,
			selectionRoot,
			committeeSeed,
			seats,
		)
	if err != nil {
		t.Fatal(err)
	}

	session, err :=
		lqc.NewRabbitVRFDKGSessionContextV1(
			chainID,
			targetEpoch,
			committeeRoot,
			uint64(len(members)),
		)
	if err != nil {
		t.Fatal(err)
	}

	sessionID, err := lqc.RabbitVRFDKGSessionIDV1(session)
	if err != nil {
		t.Fatal(err)
	}

	bridge := lqc.RabbitVRFDKGBridgeV1{
		SourceWorkEpoch:  sourceEpoch,
		PreparationEpoch: preparationEpoch,
		TargetVRFEpoch:   targetEpoch,
		SelectionRoot:    selectionRoot,
		DatasetKey: crypto.Keccak256Hash(
			[]byte("rabbit-vrf-switch-dataset-key"),
		),
		Entropy: crypto.Keccak256Hash(
			[]byte("rabbit-vrf-switch-entropy"),
		),
		CommitteeSeed: committeeSeed,
		CommitteeRoot: committeeRoot,
		Members:       members,
		Session:       session,
		SessionID:     sessionID,
	}

	baseDB := rawdb.NewMemoryDatabase()
	blockingDB := newRabbitVRFDKGBlockingDatabaseV1(baseDB)
	engine := lqc.New(&params.LQCConfig{}, blockingDB)

	if _, created, err :=
		engine.EnsureRabbitVRFDKGLifecycleV1(
			bridge,
		); err != nil {
		t.Fatal(err)
	} else if !created {
		t.Fatal("lifecycle was not created")
	}

	packets := make(
		[]rabbitVRFDKGTransportArtifactPacketV1,
		len(members),
	)
	for index := range members {
		packets[index] =
			rabbitVRFDKGCanonicalTransportArtifactV1(
				t,
				session,
				members[index],
				participantKeys[index],
			)
	}

	runtime := &rabbitVRFDKGRuntime{
		engine: engine,
		current: rabbitVRFDKGLocalContextV1{
			SessionID:        sessionID,
			CanonicalSession: session,
			CanonicalMembers: cloneRabbitVRFDKGMembersV1(members),
			Members: []lqc.RabbitVRFCommitteeMemberV1{
				members[0],
			},
			TransportBindings: []lqc.RabbitVRFDKGTransportKeyBindingV1{
				packets[0].Binding,
			},
			TransportEnvelopes: []lqc.RabbitVRFDKGEnvelopeV1{
				packets[0].Envelope,
			},
		},
	}

	transport, err := newRabbitVRFDKGTransport(
		rabbitVRFDKGTransportConfig{
			ChainID:   chainID,
			NetworkID: 9280,
			Genesis: crypto.Keccak256Hash(
				[]byte("rabbit-vrf-switch-genesis"),
			),
			Runtime: runtime,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	inserted, err :=
		transport.storeRemoteTransportArtifactV1(
			packets[1],
		)
	if err != nil {
		t.Fatal(err)
	}
	if !inserted {
		t.Fatal("remote canonical transport artifact was not inserted")
	}

	blockingDB.arm()

	persistDone := make(chan struct{})
	var persisted bool
	var persistErr error

	go func() {
		persisted, persistErr =
			transport.persistCanonicalTransportKeySetV1()
		close(persistDone)
	}()

	<-blockingDB.entered

	newSessionID := crypto.Keccak256Hash(
		[]byte("rabbit-vrf-switch-new-session"),
	)
	switchDone := make(chan struct{})

	go func() {
		runtime.setCurrent(
			rabbitVRFDKGLocalContextV1{
				SessionID: newSessionID,
			},
		)
		close(switchDone)
	}()

	select {
	case <-switchDone:
		t.Fatal("session switched while canonical key set persistence held runtime read lock")
	default:
	}

	close(blockingDB.release)

	<-switchDone
	<-persistDone

	if persistErr != nil {
		t.Fatal(persistErr)
	}
	_ = persisted

	got := runtime.currentContext()
	if got.SessionID != newSessionID {
		t.Fatal("runtime did not switch to new session")
	}
	if got.CanonicalTransportKeySetRoot != (common.Hash{}) ||
		len(got.CanonicalTransportBindings) != 0 ||
		len(got.CanonicalTransportEnvelopes) != 0 {
		t.Fatal("old canonical transport key set leaked into new runtime session")
	}

	storedOld, err :=
		engine.LoadRabbitVRFDKGTransportKeySetStateV1(
			sessionID,
		)
	if err != nil {
		t.Fatal(err)
	}
	if storedOld == nil {
		t.Fatal("old session canonical transport key set was not persisted under its own SessionID")
	}
}

func TestRabbitVRFDKGTransportV1CanonicalKeySetMissingMemberNotReady(
	t *testing.T,
) {
	transport, _, _, _ :=
		newRabbitVRFDKGCanonicalTransportKeySetFixtureV1(t)

	root, bindings, envelopes, ready, err :=
		transport.canonicalTransportKeySetV1()
	if err != nil {
		t.Fatal(err)
	}
	if ready {
		t.Fatal("incomplete canonical transport key set reported ready")
	}
	if root != (common.Hash{}) ||
		len(bindings) != 0 ||
		len(envelopes) != 0 {
		t.Fatal("incomplete canonical transport key set leaked partial result")
	}
}

func TestRabbitVRFDKGTransportV1CanonicalKeySetLocalAndRemoteComplete(
	t *testing.T,
) {
	transport, members, packets, _ :=
		newRabbitVRFDKGCanonicalTransportKeySetFixtureV1(t)

	inserted, err :=
		transport.storeRemoteTransportArtifactV1(
			packets[1],
		)
	if err != nil {
		t.Fatal(err)
	}
	if !inserted {
		t.Fatal("remote canonical transport artifact was not inserted")
	}

	root, bindings, envelopes, ready, err :=
		transport.canonicalTransportKeySetV1()
	if err != nil {
		t.Fatal(err)
	}
	if !ready {
		t.Fatal("complete canonical transport key set not ready")
	}

	if len(bindings) != 2 ||
		len(envelopes) != 2 {
		t.Fatalf(
			"wrong canonical transport key set size: bindings=%d envelopes=%d",
			len(bindings),
			len(envelopes),
		)
	}

	if bindings[0].ShareID != 1 ||
		bindings[1].ShareID != 2 {
		t.Fatalf(
			"canonical transport key order is wrong: %d %d",
			bindings[0].ShareID,
			bindings[1].ShareID,
		)
	}

	expectedBindings := []lqc.RabbitVRFDKGTransportKeyBindingV1{
		packets[0].Binding,
		packets[1].Binding,
	}
	expectedEnvelopes := []lqc.RabbitVRFDKGEnvelopeV1{
		packets[0].Envelope,
		packets[1].Envelope,
	}

	expectedRoot, err :=
		lqc.RabbitVRFDKGTransportKeySetRootV1(
			transport.runtime.current.CanonicalSession,
			members,
			expectedBindings,
			expectedEnvelopes,
		)
	if err != nil {
		t.Fatal(err)
	}

	if root == (common.Hash{}) ||
		root != expectedRoot {
		t.Fatal("canonical local+remote transport key set root mismatch")
	}
}

func TestRabbitVRFDKGTransportV1CanonicalKeySetRejectsLocalRemoteConflict(
	t *testing.T,
) {
	transport, members, _, participantKeys :=
		newRabbitVRFDKGCanonicalTransportKeySetFixtureV1(t)

	context := transport.runtime.currentContext()

	conflicting :=
		rabbitVRFDKGCanonicalTransportArtifactV1(
			t,
			context.CanonicalSession,
			members[0],
			participantKeys[0],
		)

	inserted, err :=
		transport.storeRemoteTransportArtifactV1(
			conflicting,
		)
	if err != nil {
		t.Fatal(err)
	}
	if !inserted {
		t.Fatal("conflicting valid remote artifact was not inserted")
	}

	_, _, _, ready, err :=
		transport.canonicalTransportKeySetV1()

	if err == nil {
		t.Fatal("local/remote transport key conflict was accepted")
	}
	if !errors.Is(err, errRabbitVRFDKGArtifactConflict) {
		t.Fatalf("wrong conflict error: %v", err)
	}
	if ready {
		t.Fatal("conflicting canonical transport key set reported ready")
	}
}
