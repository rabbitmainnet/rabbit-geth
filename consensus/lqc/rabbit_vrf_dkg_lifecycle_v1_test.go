package lqc

import (
	"errors"
	"sync"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
)

func rabbitVRFDKGLifecycleTestBridgeV1(
	t *testing.T,
) RabbitVRFDKGBridgeV1 {
	t.Helper()

	chainID := selectionBeaconContextChainIDV1()

	snapshot :=
		rabbitVRFCommitteeTestSnapshotV1(
			t,
			chainID,
			32,
		)

	datasetKey := common.HexToHash(
		"0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
	)

	bridge, err :=
		RabbitVRFDKGBridgeForSnapshotV1(
			chainID,
			snapshot,
			datasetKey,
			32,
			128,
			fakeSelectionBeaconHasherV1,
		)
	if err != nil {
		t.Fatal(err)
	}

	return bridge
}

func TestRabbitVRFDKGLifecycleV1CreateResumeRestart(
	t *testing.T,
) {
	bridge :=
		rabbitVRFDKGLifecycleTestBridgeV1(t)

	db := rawdb.NewMemoryDatabase()

	firstEngine := &LQC{db: db}

	first, created, err :=
		firstEngine.EnsureRabbitVRFDKGLifecycleV1(
			bridge,
		)
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("initial lifecycle was not created")
	}
	if first.SessionID != bridge.SessionID ||
		first.Phase !=
			RabbitVRFDKGLifecyclePhasePreparedV1 {
		t.Fatal("unexpected initial lifecycle")
	}

	second, created, err :=
		firstEngine.EnsureRabbitVRFDKGLifecycleV1(
			bridge,
		)
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Fatal("idempotent ensure recreated lifecycle")
	}
	if second.SessionID != first.SessionID {
		t.Fatal("session changed during resume")
	}

	restartedEngine := &LQC{db: db}

	restarted, created, err :=
		restartedEngine.EnsureRabbitVRFDKGLifecycleV1(
			bridge,
		)
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Fatal("restart recreated persisted lifecycle")
	}
	if restarted.SessionID != first.SessionID ||
		restarted.SourceWorkEpoch !=
			first.SourceWorkEpoch ||
		restarted.TargetVRFEpoch !=
			first.TargetVRFEpoch {
		t.Fatal("restart changed persisted lifecycle")
	}
}

func TestRabbitVRFDKGLifecycleV1RejectsConflict(
	t *testing.T,
) {
	bridge :=
		rabbitVRFDKGLifecycleTestBridgeV1(t)

	db := rawdb.NewMemoryDatabase()
	engine := &LQC{db: db}

	if _, _, err :=
		engine.EnsureRabbitVRFDKGLifecycleV1(
			bridge,
		); err != nil {
		t.Fatal(err)
	}

	conflicting := bridge
	conflicting.DatasetKey = common.HexToHash(
		"0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
	)

	if _, _, err :=
		engine.EnsureRabbitVRFDKGLifecycleV1(
			conflicting,
		); !errors.Is(
		err,
		ErrRabbitVRFDKGLifecycleConflictV1,
	) {
		t.Fatalf("conflict err=%v", err)
	}
}

func TestRabbitVRFDKGLifecycleV1RejectsCorruptStore(
	t *testing.T,
) {
	bridge :=
		rabbitVRFDKGLifecycleTestBridgeV1(t)

	db := rawdb.NewMemoryDatabase()
	engine := &LQC{db: db}

	key := rabbitVRFDKGLifecycleKeyV1(
		bridge.SessionID,
	)

	if err := db.Put(
		key,
		[]byte{0xff, 0x00, 0x01},
	); err != nil {
		t.Fatal(err)
	}

	if _, _, err :=
		engine.EnsureRabbitVRFDKGLifecycleV1(
			bridge,
		); !errors.Is(
		err,
		ErrInvalidRabbitVRFDKGLifecycleV1,
	) {
		t.Fatalf("corrupt lifecycle err=%v", err)
	}
}

func TestRabbitVRFDKGLifecycleV1RejectsMemberRootMismatch(
	t *testing.T,
) {
	bridge :=
		rabbitVRFDKGLifecycleTestBridgeV1(t)

	bridge.Members = append(
		[]RabbitVRFCommitteeMemberV1(nil),
		bridge.Members...,
	)

	bridge.Members[0].Participant =
		common.HexToAddress(
			"0x000000000000000000000000000000000000dEaD",
		)

	db := rawdb.NewMemoryDatabase()
	engine := &LQC{db: db}

	if _, _, err :=
		engine.EnsureRabbitVRFDKGLifecycleV1(
			bridge,
		); !errors.Is(
		err,
		ErrInvalidRabbitVRFDKGLifecycleV1,
	) {
		t.Fatalf(
			"member/root mismatch err=%v",
			err,
		)
	}
}

func TestRabbitVRFDKGLifecycleV1ConcurrentEnsure(
	t *testing.T,
) {
	bridge :=
		rabbitVRFDKGLifecycleTestBridgeV1(t)

	db := rawdb.NewMemoryDatabase()

	engines := []*LQC{
		{db: db},
		{db: db},
	}

	const workers = 24

	start := make(chan struct{})
	results := make(chan bool, workers)
	errs := make(chan error, workers)

	var wg sync.WaitGroup

	for index := 0; index < workers; index++ {
		wg.Add(1)

		go func(engine *LQC) {
			defer wg.Done()

			<-start

			state, created, err :=
				engine.EnsureRabbitVRFDKGLifecycleV1(
					bridge,
				)
			if err != nil {
				errs <- err
				return
			}

			if state == nil ||
				state.SessionID != bridge.SessionID {
				errs <- ErrInvalidRabbitVRFDKGLifecycleV1
				return
			}

			results <- created
		}(engines[index%len(engines)])
	}

	close(start)
	wg.Wait()

	close(results)
	close(errs)

	for err := range errs {
		t.Fatal(err)
	}

	createdCount := 0

	for created := range results {
		if created {
			createdCount++
		}
	}

	if createdCount != 1 {
		t.Fatalf(
			"created lifecycles=%d want=1",
			createdCount,
		)
	}

	loaded, err :=
		engines[0].LoadRabbitVRFDKGLifecycleV1(
			bridge.SessionID,
		)
	if err != nil {
		t.Fatal(err)
	}

	if loaded.SessionID != bridge.SessionID {
		t.Fatal(
			"persisted concurrent session changed",
		)
	}
}
