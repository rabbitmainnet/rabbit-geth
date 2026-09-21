package lqc

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestRabbitVRFDKGBridgeForSnapshotV1Deterministic(
	t *testing.T,
) {
	chainID := selectionBeaconContextChainIDV1()
	snapshot :=
		rabbitVRFCommitteeTestSnapshotV1(
			t,
			chainID,
			32,
		)

	datasetKey :=
		common.HexToHash(
			"0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
		)

	calls := 0
	hasher := func(
		key common.Hash,
		input []byte,
	) (common.Hash, error) {
		calls++
		return fakeSelectionBeaconHasherV1(
			key,
			input,
		)
	}

	first, err :=
		RabbitVRFDKGBridgeForSnapshotV1(
			chainID,
			snapshot,
			datasetKey,
			32,
			128,
			hasher,
		)
	if err != nil {
		t.Fatal(err)
	}

	second, err :=
		RabbitVRFDKGBridgeForSnapshotV1(
			chainID,
			snapshot,
			datasetKey,
			32,
			128,
			hasher,
		)
	if err != nil {
		t.Fatal(err)
	}

	if calls != 2 {
		t.Fatalf(
			"selection beacon hashes=%d want=2",
			calls,
		)
	}

	if first.SourceWorkEpoch != 7 ||
		first.PreparationEpoch != 9 ||
		first.TargetVRFEpoch != 10 {
		t.Fatalf(
			"epoch mapping source=%d preparation=%d target=%d",
			first.SourceWorkEpoch,
			first.PreparationEpoch,
			first.TargetVRFEpoch,
		)
	}

	if first.SelectionRoot != snapshot.Root ||
		first.DatasetKey != datasetKey {
		t.Fatal("bridge did not preserve canonical Work context")
	}

	if first.Entropy == (common.Hash{}) ||
		first.CommitteeSeed == (common.Hash{}) ||
		first.CommitteeRoot == (common.Hash{}) ||
		first.SessionID == (common.Hash{}) {
		t.Fatal("bridge produced zero commitment material")
	}

	if first.Entropy != second.Entropy ||
		first.CommitteeSeed != second.CommitteeSeed ||
		first.CommitteeRoot != second.CommitteeRoot ||
		first.SessionID != second.SessionID {
		t.Fatal("identical bridge input produced different result")
	}

	if len(first.Members) != len(second.Members) ||
		len(first.Members) == 0 {
		t.Fatal("invalid deterministic committee size")
	}

	for index := range first.Members {
		if first.Members[index] != second.Members[index] {
			t.Fatalf(
				"member %d changed across identical bridge input",
				index,
			)
		}

		if first.Members[index].ShareID != uint64(index+1) {
			t.Fatalf(
				"member %d shareID=%d",
				index,
				first.Members[index].ShareID,
			)
		}
	}

	if first.Session.TargetVRFEpoch != first.TargetVRFEpoch ||
		first.Session.CommitteeRoot != first.CommitteeRoot ||
		first.Session.CommitteeSize != uint64(len(first.Members)) {
		t.Fatal("DKG session does not bind bridge committee")
	}
}

func selectionBeaconContextChainIDV1() *big.Int {
	chainID, _, _, _ := selectionBeaconContextV1()
	return chainID
}

func TestRabbitVRFDKGBridgeForSnapshotV1RejectsInvalidInputs(
	t *testing.T,
) {
	chainID := big.NewInt(9280)
	snapshot :=
		rabbitVRFCommitteeTestSnapshotV1(
			t,
			chainID,
			32,
		)
	datasetKey :=
		common.HexToHash(
			"0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
		)

	if _, err :=
		RabbitVRFDKGBridgeForSnapshotV1(
			chainID,
			nil,
			datasetKey,
			32,
			128,
			fakeSelectionBeaconHasherV1,
		); err == nil {
		t.Fatal("nil snapshot accepted")
	}

	if _, err :=
		RabbitVRFDKGBridgeForSnapshotV1(
			chainID,
			snapshot,
			common.Hash{},
			32,
			128,
			fakeSelectionBeaconHasherV1,
		); err == nil {
		t.Fatal("zero dataset key accepted")
	}

	if _, err :=
		RabbitVRFDKGBridgeForSnapshotV1(
			chainID,
			snapshot,
			datasetKey,
			32,
			128,
			nil,
		); err == nil {
		t.Fatal("nil selection beacon hasher accepted")
	}
}

func TestRabbitVRFDKGBridgeForSnapshotV1BindsDatasetKey(
	t *testing.T,
) {
	chainID := big.NewInt(9280)
	snapshot :=
		rabbitVRFCommitteeTestSnapshotV1(
			t,
			chainID,
			64,
		)

	first, err :=
		RabbitVRFDKGBridgeForSnapshotV1(
			chainID,
			snapshot,
			common.HexToHash("0x1111"),
			32,
			128,
			fakeSelectionBeaconHasherV1,
		)
	if err != nil {
		t.Fatal(err)
	}

	second, err :=
		RabbitVRFDKGBridgeForSnapshotV1(
			chainID,
			snapshot,
			common.HexToHash("0x2222"),
			32,
			128,
			fakeSelectionBeaconHasherV1,
		)
	if err != nil {
		t.Fatal(err)
	}

	if first.Entropy == second.Entropy {
		t.Fatal("dataset key did not bind selection entropy")
	}

	if first.CommitteeSeed == second.CommitteeSeed {
		t.Fatal("dataset key did not bind committee seed")
	}

	if first.CommitteeRoot == second.CommitteeRoot {
		t.Fatal("dataset key did not bind committee root")
	}

	if first.SessionID == second.SessionID {
		t.Fatal("dataset key did not bind DKG session")
	}
}
