package rabbitvrfstate

import (
	"encoding/json"
	"math/big"
	"os"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/crypto"
)

func dkgDealerPolynomialStoreFixtureV1(
	t *testing.T,
) (
	lqc.RabbitVRFDKGSessionContextV1,
	lqc.RabbitVRFCommitteeMemberV1,
) {
	t.Helper()

	context, err := lqc.NewRabbitVRFDKGSessionContextV1(
		big.NewInt(9280),
		7,
		crypto.Keccak256Hash(
			[]byte("rabbit-vrf-dealer-polynomial-store-test"),
		),
		4,
	)
	if err != nil {
		t.Fatal(err)
	}

	participantKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}

	member := lqc.RabbitVRFCommitteeMemberV1{
		ShareID: 2,
		TicketHash: crypto.Keccak256Hash(
			[]byte("rabbit-vrf-dealer-polynomial-member"),
		),
		Participant: crypto.PubkeyToAddress(
			participantKey.PublicKey,
		),
	}

	return context, member
}

func TestDKGDealerPolynomialStoreV1RestartRoundTrip(
	t *testing.T,
) {
	context, member :=
		dkgDealerPolynomialStoreFixtureV1(t)

	store, err := NewDKGDealerPolynomialStoreV1(
		t.TempDir(),
		keystore.LightScryptN,
		keystore.LightScryptP,
	)
	if err != nil {
		t.Fatal(err)
	}

	created, createdCommitment, createdRoot, err :=
		store.Create(
			context,
			member,
			"test-password",
		)
	if err != nil {
		t.Fatal(err)
	}
	defer created.Destroy()

	createdBytes, err := created.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	defer zeroBytesV1(createdBytes)

	createdEvaluation, err := created.Evaluate(3)
	if err != nil {
		t.Fatal(err)
	}

	restarted, err := NewDKGDealerPolynomialStoreV1(
		store.dir,
		keystore.LightScryptN,
		keystore.LightScryptP,
	)
	if err != nil {
		t.Fatal(err)
	}

	loaded, loadedCommitment, loadedRoot, err :=
		restarted.Load(
			context,
			member,
			"test-password",
		)
	if err != nil {
		t.Fatal(err)
	}
	defer loaded.Destroy()

	loadedBytes, err := loaded.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	defer zeroBytesV1(loadedBytes)

	if string(createdBytes) != string(loadedBytes) {
		t.Fatal("restart changed dealer polynomial")
	}

	if createdRoot != loadedRoot {
		t.Fatal("restart changed commitment root")
	}

	if createdCommitment.Version != loadedCommitment.Version ||
		createdCommitment.SessionID != loadedCommitment.SessionID ||
		createdCommitment.DealerShareID != loadedCommitment.DealerShareID ||
		len(createdCommitment.Coefficients) !=
			len(loadedCommitment.Coefficients) {
		t.Fatal("restart changed polynomial commitment")
	}

	for index := range createdCommitment.Coefficients {
		if createdCommitment.Coefficients[index] !=
			loadedCommitment.Coefficients[index] {
			t.Fatal("restart changed commitment coefficient")
		}
	}

	loadedEvaluation, err := loaded.Evaluate(3)
	if err != nil {
		t.Fatal(err)
	}

	if createdEvaluation != loadedEvaluation {
		t.Fatal("restart changed dealer evaluation")
	}
}

func TestDKGDealerPolynomialStoreV1WrongPassword(
	t *testing.T,
) {
	context, member :=
		dkgDealerPolynomialStoreFixtureV1(t)

	store, err := NewDKGDealerPolynomialStoreV1(
		t.TempDir(),
		keystore.LightScryptN,
		keystore.LightScryptP,
	)
	if err != nil {
		t.Fatal(err)
	}

	polynomial, _, _, err := store.Create(
		context,
		member,
		"correct-password",
	)
	if err != nil {
		t.Fatal(err)
	}
	polynomial.Destroy()

	if _, _, _, err := store.Load(
		context,
		member,
		"wrong-password",
	); err == nil {
		t.Fatal("wrong password accepted")
	}
}

func TestDKGDealerPolynomialStoreV1NoOverwrite(
	t *testing.T,
) {
	context, member :=
		dkgDealerPolynomialStoreFixtureV1(t)

	store, err := NewDKGDealerPolynomialStoreV1(
		t.TempDir(),
		keystore.LightScryptN,
		keystore.LightScryptP,
	)
	if err != nil {
		t.Fatal(err)
	}

	polynomial, _, root, err := store.Create(
		context,
		member,
		"test-password",
	)
	if err != nil {
		t.Fatal(err)
	}
	polynomial.Destroy()

	if second, _, _, err := store.Create(
		context,
		member,
		"test-password",
	); err == nil {
		if second != nil {
			second.Destroy()
		}
		t.Fatal("existing dealer polynomial overwritten")
	}

	loaded, _, loadedRoot, err := store.Load(
		context,
		member,
		"test-password",
	)
	if err != nil {
		t.Fatal(err)
	}
	defer loaded.Destroy()

	if loadedRoot != root {
		t.Fatal("failed create changed existing polynomial")
	}
}

func TestDKGDealerPolynomialStoreV1RejectsTamperedMetadata(
	t *testing.T,
) {
	context, member :=
		dkgDealerPolynomialStoreFixtureV1(t)

	store, err := NewDKGDealerPolynomialStoreV1(
		t.TempDir(),
		keystore.LightScryptN,
		keystore.LightScryptP,
	)
	if err != nil {
		t.Fatal(err)
	}

	polynomial, _, _, err := store.Create(
		context,
		member,
		"test-password",
	)
	if err != nil {
		t.Fatal(err)
	}
	polynomial.Destroy()

	path, err := store.Path(context, member)
	if err != nil {
		t.Fatal(err)
	}

	encoded, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var record dkgDealerPolynomialStoreFileV1
	if err := json.Unmarshal(encoded, &record); err != nil {
		t.Fatal(err)
	}

	record.CommitmentRoot = common.HexToHash("0x1234")

	tampered, err := json.MarshalIndent(
		record,
		"",
		"  ",
	)
	if err != nil {
		t.Fatal(err)
	}
	tampered = append(tampered, '\n')

	if err := os.WriteFile(
		path,
		tampered,
		0600,
	); err != nil {
		t.Fatal(err)
	}

	if _, _, _, err := store.Load(
		context,
		member,
		"test-password",
	); err == nil {
		t.Fatal("tampered metadata accepted")
	}
}
