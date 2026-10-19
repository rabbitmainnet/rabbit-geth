package lqc

import (
	"bytes"
	"crypto/ecdsa"
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/crypto"
)

func rabbitVRFDKGTransportKeySetStateArtifactsV1(
	t *testing.T,
	bridge RabbitVRFDKGBridgeV1,
	participantKeys map[common.Address]*ecdsa.PrivateKey,
) (
	[]RabbitVRFDKGTransportKeyBindingV1,
	[]RabbitVRFDKGEnvelopeV1,
) {
	t.Helper()

	bindings := make(
		[]RabbitVRFDKGTransportKeyBindingV1,
		0,
		len(bridge.Members),
	)
	envelopes := make(
		[]RabbitVRFDKGEnvelopeV1,
		0,
		len(bridge.Members),
	)

	for _, member := range bridge.Members {
		participantKey := participantKeys[member.Participant]
		if participantKey == nil {
			t.Fatalf(
				"missing participant key for share %d",
				member.ShareID,
			)
		}

		transportKey, err := crypto.GenerateKey()
		if err != nil {
			t.Fatal(err)
		}

		publicKey, err :=
			RabbitVRFDKGTransportPublicKeyV1FromBytes(
				crypto.CompressPubkey(
					&transportKey.PublicKey,
				),
			)
		if err != nil {
			t.Fatal(err)
		}

		binding, bindingRoot, err :=
			NewRabbitVRFDKGTransportKeyBindingV1(
				bridge.Session,
				member,
				publicKey,
			)
		if err != nil {
			t.Fatal(err)
		}

		envelope, err := NewRabbitVRFDKGEnvelopeV1(
			bridge.Session,
			member,
			RabbitVRFDKGMessageTransportKeyBindingV1,
			bindingRoot,
		)
		if err != nil {
			t.Fatal(err)
		}

		signingHash, err :=
			RabbitVRFDKGEnvelopeSigningHashV1(
				bridge.Session,
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

		bindings = append(bindings, binding)
		envelopes = append(envelopes, envelope)
	}

	return bindings, envelopes
}

func rabbitVRFDKGTransportKeySetStateFixtureV1(
	t *testing.T,
) (
	*LQC,
	RabbitVRFDKGBridgeV1,
	[]RabbitVRFDKGTransportKeyBindingV1,
	[]RabbitVRFDKGEnvelopeV1,
	map[common.Address]*ecdsa.PrivateKey,
) {
	t.Helper()

	chainID := selectionBeaconContextChainIDV1()
	anchor := common.HexToHash("0x7a01")
	difficulty := big.NewInt(4096)

	participantKeys :=
		make(map[common.Address]*ecdsa.PrivateKey)
	seats := make([]WorkSeatV1, 0, 2)

	for index := 0; index < 2; index++ {
		participantKey, err := crypto.GenerateKey()
		if err != nil {
			t.Fatal(err)
		}

		participant :=
			crypto.PubkeyToAddress(
				participantKey.PublicKey,
			)

		participantKeys[participant] = participantKey

		seats = append(
			seats,
			WorkSeatV1{
				TicketHash: crypto.Keccak256Hash(
					[]byte{
						byte(index + 1),
						0x7a,
						0x01,
					},
				),
				Participant: participant,
			},
		)
	}

	root, canonicalSeats, err := WorkEpochRootV1(
		chainID,
		7,
		anchor,
		difficulty,
		seats,
	)
	if err != nil {
		t.Fatal(err)
	}

	snapshot := &WorkEpochSnapshotV1{
		Epoch:      7,
		Anchor:     anchor,
		Difficulty: new(big.Int).Set(difficulty),
		Root:       root,
		Seats:      canonicalSeats,
	}
	if err := snapshot.Validate(chainID); err != nil {
		t.Fatal(err)
	}

	bridge, err := RabbitVRFDKGBridgeForSnapshotV1(
		chainID,
		snapshot,
		common.HexToHash(
			"0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
		),
		2,
		2,
		fakeSelectionBeaconHasherV1,
	)
	if err != nil {
		t.Fatal(err)
	}

	db := rawdb.NewMemoryDatabase()
	engine := &LQC{db: db}

	if _, created, err :=
		engine.EnsureRabbitVRFDKGLifecycleV1(
			bridge,
		); err != nil {
		t.Fatal(err)
	} else if !created {
		t.Fatal("fixture lifecycle was not created")
	}

	bindings, envelopes :=
		rabbitVRFDKGTransportKeySetStateArtifactsV1(
			t,
			bridge,
			participantKeys,
		)

	return engine,
		bridge,
		bindings,
		envelopes,
		participantKeys
}

func TestRabbitVRFDKGTransportKeySetStateV1MissingStateIsNotError(
	t *testing.T,
) {
	engine, bridge, _, _, _ :=
		rabbitVRFDKGTransportKeySetStateFixtureV1(t)

	state, err :=
		engine.LoadRabbitVRFDKGTransportKeySetStateV1(
			bridge.SessionID,
		)
	if err != nil {
		t.Fatal(err)
	}
	if state != nil {
		t.Fatal("missing transport key set state unexpectedly loaded")
	}
}

func TestRabbitVRFDKGTransportKeySetStateV1CreateResumeRestart(
	t *testing.T,
) {
	engine,
		bridge,
		bindings,
		envelopes,
		_ :=
		rabbitVRFDKGTransportKeySetStateFixtureV1(t)

	first, created, err :=
		engine.EnsureRabbitVRFDKGTransportKeySetStateV1(
			bridge.SessionID,
			bridge.Members,
			bindings,
			envelopes,
		)
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("initial transport key set state was not created")
	}
	if first.Root == (common.Hash{}) {
		t.Fatal("persisted transport key set root is zero")
	}

	second, created, err :=
		engine.EnsureRabbitVRFDKGTransportKeySetStateV1(
			bridge.SessionID,
			bridge.Members,
			bindings,
			envelopes,
		)
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Fatal("idempotent ensure recreated transport key set state")
	}
	if second.Root != first.Root {
		t.Fatal("idempotent ensure changed transport key set root")
	}

	restartedEngine := &LQC{db: engine.db}

	loaded, err :=
		restartedEngine.LoadRabbitVRFDKGTransportKeySetStateV1(
			bridge.SessionID,
		)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.SessionID != first.SessionID ||
		loaded.Root != first.Root ||
		len(loaded.Members) != len(first.Members) ||
		len(loaded.Bindings) != len(first.Bindings) ||
		len(loaded.Envelopes) != len(first.Envelopes) {
		t.Fatal("restart changed persisted transport key set state")
	}

	for index := range first.Bindings {
		if loaded.Members[index] != first.Members[index] ||
			loaded.Bindings[index] != first.Bindings[index] ||
			!bytes.Equal(
				loaded.Envelopes[index].Signature,
				first.Envelopes[index].Signature,
			) {
			t.Fatalf(
				"restart changed transport key set entry %d",
				index,
			)
		}
	}

	restarted, created, err :=
		restartedEngine.EnsureRabbitVRFDKGTransportKeySetStateV1(
			bridge.SessionID,
			bridge.Members,
			bindings,
			envelopes,
		)
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Fatal("restart recreated persisted transport key set state")
	}
	if restarted.Root != first.Root {
		t.Fatal("restart changed transport key set root")
	}
}

func TestRabbitVRFDKGTransportKeySetStateV1SessionsCoexist(
	t *testing.T,
) {
	engine,
		firstBridge,
		firstBindings,
		firstEnvelopes,
		participantKeys :=
		rabbitVRFDKGTransportKeySetStateFixtureV1(t)

	first, created, err :=
		engine.EnsureRabbitVRFDKGTransportKeySetStateV1(
			firstBridge.SessionID,
			firstBridge.Members,
			firstBindings,
			firstEnvelopes,
		)
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("first transport key set state was not created")
	}

	seats := make(
		[]WorkSeatV1,
		len(firstBridge.Members),
	)
	for index, member := range firstBridge.Members {
		seats[index] = WorkSeatV1{
			TicketHash:  member.TicketHash,
			Participant: member.Participant,
		}
	}

	chainID := new(big.Int).Set(
		firstBridge.Session.ChainID,
	)
	anchor := common.HexToHash("0x7a02")
	difficulty := big.NewInt(4096)

	root, canonicalSeats, err := WorkEpochRootV1(
		chainID,
		8,
		anchor,
		difficulty,
		seats,
	)
	if err != nil {
		t.Fatal(err)
	}

	snapshot := &WorkEpochSnapshotV1{
		Epoch:      8,
		Anchor:     anchor,
		Difficulty: new(big.Int).Set(difficulty),
		Root:       root,
		Seats:      canonicalSeats,
	}
	if err := snapshot.Validate(chainID); err != nil {
		t.Fatal(err)
	}

	secondBridge, err := RabbitVRFDKGBridgeForSnapshotV1(
		chainID,
		snapshot,
		common.HexToHash(
			"0x2234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
		),
		2,
		2,
		fakeSelectionBeaconHasherV1,
	)
	if err != nil {
		t.Fatal(err)
	}
	if secondBridge.SessionID == firstBridge.SessionID {
		t.Fatal("distinct snapshots produced identical DKG SessionID")
	}

	if _, created, err :=
		engine.EnsureRabbitVRFDKGLifecycleV1(
			secondBridge,
		); err != nil {
		t.Fatal(err)
	} else if !created {
		t.Fatal("second lifecycle was not created")
	}

	secondBindings, secondEnvelopes :=
		rabbitVRFDKGTransportKeySetStateArtifactsV1(
			t,
			secondBridge,
			participantKeys,
		)

	second, created, err :=
		engine.EnsureRabbitVRFDKGTransportKeySetStateV1(
			secondBridge.SessionID,
			secondBridge.Members,
			secondBindings,
			secondEnvelopes,
		)
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("second transport key set state was not created")
	}

	loadedFirst, err :=
		engine.LoadRabbitVRFDKGTransportKeySetStateV1(
			firstBridge.SessionID,
		)
	if err != nil {
		t.Fatal(err)
	}

	loadedSecond, err :=
		engine.LoadRabbitVRFDKGTransportKeySetStateV1(
			secondBridge.SessionID,
		)
	if err != nil {
		t.Fatal(err)
	}

	if loadedFirst == nil || loadedSecond == nil {
		t.Fatal("coexisting transport key set state is missing")
	}
	if loadedFirst.SessionID != firstBridge.SessionID ||
		loadedFirst.Root != first.Root {
		t.Fatal("first session state changed after second session persisted")
	}
	if loadedSecond.SessionID != secondBridge.SessionID ||
		loadedSecond.Root != second.Root {
		t.Fatal("second session state did not persist independently")
	}
}

func TestRabbitVRFDKGTransportKeySetStateV1RejectsCorruptStore(
	t *testing.T,
) {
	engine,
		bridge,
		_,
		_,
		_ :=
		rabbitVRFDKGTransportKeySetStateFixtureV1(t)

	key :=
		rabbitVRFDKGTransportKeySetStateKeyV1(
			bridge.SessionID,
		)

	if err := engine.db.Put(
		key,
		[]byte{0xff, 0x00, 0x01},
	); err != nil {
		t.Fatal(err)
	}

	if _, err :=
		engine.LoadRabbitVRFDKGTransportKeySetStateV1(
			bridge.SessionID,
		); !errors.Is(
		err,
		ErrInvalidRabbitVRFDKGTransportKeySetStateV1,
	) {
		t.Fatalf("corrupt transport key set state err=%v", err)
	}
}

func TestRabbitVRFDKGTransportKeySetStateV1RejectsConflict(
	t *testing.T,
) {
	engine,
		bridge,
		bindings,
		envelopes,
		participantKeys :=
		rabbitVRFDKGTransportKeySetStateFixtureV1(t)

	first, created, err :=
		engine.EnsureRabbitVRFDKGTransportKeySetStateV1(
			bridge.SessionID,
			bridge.Members,
			bindings,
			envelopes,
		)
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("initial transport key set state was not created")
	}

	conflictingBindings, conflictingEnvelopes :=
		rabbitVRFDKGTransportKeySetStateArtifactsV1(
			t,
			bridge,
			participantKeys,
		)

	conflictingRoot, err :=
		RabbitVRFDKGTransportKeySetRootV1(
			bridge.Session,
			bridge.Members,
			conflictingBindings,
			conflictingEnvelopes,
		)
	if err != nil {
		t.Fatal(err)
	}
	if conflictingRoot == first.Root {
		t.Fatal("fixture failed to create conflicting transport key set")
	}

	if _, _, err :=
		engine.EnsureRabbitVRFDKGTransportKeySetStateV1(
			bridge.SessionID,
			bridge.Members,
			conflictingBindings,
			conflictingEnvelopes,
		); !errors.Is(
		err,
		ErrRabbitVRFDKGTransportKeySetStateConflictV1,
	) {
		t.Fatalf("transport key set conflict err=%v", err)
	}
}

func TestRabbitVRFDKGTransportKeySetStateV1RejectsCommitteeRootMismatch(
	t *testing.T,
) {
	engine,
		bridge,
		bindings,
		envelopes,
		_ :=
		rabbitVRFDKGTransportKeySetStateFixtureV1(t)

	wrongMembers := append(
		[]RabbitVRFCommitteeMemberV1(nil),
		bridge.Members...,
	)
	wrongMembers[0].TicketHash =
		crypto.Keccak256Hash(
			[]byte("wrong-rabbit-vrf-dkg-ticket"),
		)

	if _, _, err :=
		engine.EnsureRabbitVRFDKGTransportKeySetStateV1(
			bridge.SessionID,
			wrongMembers,
			bindings,
			envelopes,
		); !errors.Is(
		err,
		ErrInvalidRabbitVRFDKGTransportKeySetStateV1,
	) {
		t.Fatalf("committee root mismatch err=%v", err)
	}

	has, err := engine.db.Has(
		rabbitVRFDKGTransportKeySetStateKeyV1(
			bridge.SessionID,
		),
	)
	if err != nil {
		t.Fatal(err)
	}
	if has {
		t.Fatal("invalid committee transport key set state was persisted")
	}
}
