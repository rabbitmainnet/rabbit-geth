//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"crypto/ecdsa"
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/internal/rabbitvrfstate"
)

func rabbitVRFDKGTransportSecretFixtureV1(
	t *testing.T,
) (
	lqc.RabbitVRFDKGSessionContextV1,
	lqc.RabbitVRFCommitteeMemberV1,
) {
	t.Helper()

	context, err :=
		lqc.NewRabbitVRFDKGSessionContextV1(
			big.NewInt(9280),
			11,
			crypto.Keccak256Hash(
				[]byte("rabbit-vrf-dkg-transport-secret-test-committee-v1"),
			),
			32,
		)
	if err != nil {
		t.Fatal(err)
	}

	participantKey, err :=
		crypto.HexToECDSA(
			"0000000000000000000000000000000000000000000000000000000000000001",
		)
	if err != nil {
		t.Fatal(err)
	}

	member := lqc.RabbitVRFCommitteeMemberV1{
		ShareID: 7,
		TicketHash: crypto.Keccak256Hash(
			[]byte("rabbit-vrf-dkg-transport-secret-test-ticket-v1"),
		),
		Participant: crypto.PubkeyToAddress(
			participantKey.PublicKey,
		),
	}

	return context, member
}

func TestRabbitVRFDKGTransportSecretV1RejectsP2PKeyBeforePersist(
	t *testing.T,
) {
	context, member :=
		rabbitVRFDKGTransportSecretFixtureV1(t)

	store, err :=
		rabbitvrfstate.NewDKGTransportKeyStoreV1(
			t.TempDir(),
			2,
			1,
		)
	if err != nil {
		t.Fatal(err)
	}

	p2pKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}

	generatedKey, err :=
		crypto.ToECDSA(
			crypto.FromECDSA(p2pKey),
		)
	if err != nil {
		t.Fatal(err)
	}

	var injected *ecdsa.PrivateKey

	generate := func() (*ecdsa.PrivateKey, error) {
		injected = generatedKey
		return generatedKey, nil
	}

	_, err =
		rabbitVRFDKGLoadOrCreateTransportBindingWithGeneratorV1(
			store,
			context,
			member,
			"rabbit-vrf-transport-secret-test-password",
			p2pKey,
			generate,
		)
	if !errors.Is(
		err,
		errRabbitVRFDKGTransportReusesP2PKeyV1,
	) {
		t.Fatalf("unexpected p2p-reuse error: %v", err)
	}

	if injected == nil ||
		injected.D == nil ||
		injected.D.Sign() != 0 {
		t.Fatal("generated transport private scalar was not zeroized")
	}

	if p2pKey.D == nil ||
		p2pKey.D.Sign() == 0 {
		t.Fatal("p2p node private key was modified")
	}

	loaded, _, err :=
		store.Load(
			context,
			member,
			"rabbit-vrf-transport-secret-test-password",
		)
	if loaded != nil {
		zeroRabbitVRFDKGPrivateKeyV1(loaded)
		t.Fatal("p2p-reused transport key was persisted")
	}
	if !errors.Is(
		err,
		rabbitvrfstate.ErrDKGTransportKeyStoreMissingV1,
	) {
		t.Fatalf("transport store error after p2p rejection: %v", err)
	}
}

func TestRabbitVRFDKGTransportSecretV1CreateAndRestartLoad(
	t *testing.T,
) {
	context, member :=
		rabbitVRFDKGTransportSecretFixtureV1(t)

	dir := t.TempDir()

	store, err :=
		rabbitvrfstate.NewDKGTransportKeyStoreV1(
			dir,
			2,
			1,
		)
	if err != nil {
		t.Fatal(err)
	}

	p2pKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}

	var generated *ecdsa.PrivateKey
	generateCalls := 0

	generate := func() (*ecdsa.PrivateKey, error) {
		generateCalls++

		key, err := crypto.GenerateKey()
		if err != nil {
			return nil, err
		}

		generated = key
		return key, nil
	}

	first, err :=
		rabbitVRFDKGLoadOrCreateTransportBindingWithGeneratorV1(
			store,
			context,
			member,
			"rabbit-vrf-transport-secret-test-password",
			p2pKey,
			generate,
		)
	if err != nil {
		t.Fatal(err)
	}

	if generateCalls != 1 {
		t.Fatalf(
			"first call generated %d keys; want 1",
			generateCalls,
		)
	}

	if generated == nil ||
		generated.D == nil ||
		generated.D.Sign() != 0 {
		t.Fatal(
			"generated transport private scalar was not zeroized",
		)
	}

	restarted, err :=
		rabbitvrfstate.NewDKGTransportKeyStoreV1(
			dir,
			2,
			1,
		)
	if err != nil {
		t.Fatal(err)
	}

	restartGenerateCalls := 0
	restartGenerate := func() (*ecdsa.PrivateKey, error) {
		restartGenerateCalls++
		return crypto.GenerateKey()
	}

	second, err :=
		rabbitVRFDKGLoadOrCreateTransportBindingWithGeneratorV1(
			restarted,
			context,
			member,
			"rabbit-vrf-transport-secret-test-password",
			p2pKey,
			restartGenerate,
		)
	if err != nil {
		t.Fatal(err)
	}

	if restartGenerateCalls != 0 {
		t.Fatalf(
			"restart generated %d keys; want 0",
			restartGenerateCalls,
		)
	}

	if second != first {
		t.Fatalf(
			"restart binding mismatch:\nfirst=%+v\nsecond=%+v",
			first,
			second,
		)
	}

	if p2pKey.D == nil ||
		p2pKey.D.Sign() == 0 {
		t.Fatal("p2p node private key was modified")
	}
}

func TestRabbitVRFDKGTransportSecretV1AlreadyExistsLoadsWinner(
	t *testing.T,
) {
	context, member :=
		rabbitVRFDKGTransportSecretFixtureV1(t)

	dir := t.TempDir()

	store, err :=
		rabbitvrfstate.NewDKGTransportKeyStoreV1(
			dir,
			2,
			1,
		)
	if err != nil {
		t.Fatal(err)
	}

	racingStore, err :=
		rabbitvrfstate.NewDKGTransportKeyStoreV1(
			dir,
			2,
			1,
		)
	if err != nil {
		t.Fatal(err)
	}

	p2pKey, err :=
		crypto.HexToECDSA(
			"0000000000000000000000000000000000000000000000000000000000000002",
		)
	if err != nil {
		t.Fatal(err)
	}
	defer zeroRabbitVRFDKGPrivateKeyV1(p2pKey)

	winnerKey, err :=
		crypto.HexToECDSA(
			"0000000000000000000000000000000000000000000000000000000000000003",
		)
	if err != nil {
		t.Fatal(err)
	}
	defer zeroRabbitVRFDKGPrivateKeyV1(winnerKey)

	loserKey, err :=
		crypto.HexToECDSA(
			"0000000000000000000000000000000000000000000000000000000000000004",
		)
	if err != nil {
		t.Fatal(err)
	}

	const password = "rabbit-vrf-transport-secret-race-password"

	var (
		winnerBinding lqc.RabbitVRFDKGTransportKeyBindingV1
		generateCalls int
	)

	generate := func() (*ecdsa.PrivateKey, error) {
		generateCalls++

		var saveErr error
		winnerBinding, saveErr =
			racingStore.Save(
				context,
				member,
				winnerKey,
				password,
			)
		if saveErr != nil {
			return nil, saveErr
		}

		return loserKey, nil
	}

	binding, err :=
		rabbitVRFDKGLoadOrCreateTransportBindingWithGeneratorV1(
			store,
			context,
			member,
			password,
			p2pKey,
			generate,
		)
	if err != nil {
		t.Fatal(err)
	}

	if generateCalls != 1 {
		t.Fatalf(
			"generate calls=%d want=1",
			generateCalls,
		)
	}

	if binding != winnerBinding {
		t.Fatal("race loser did not load winner binding")
	}

	if loserKey.D == nil ||
		loserKey.D.Sign() != 0 {
		t.Fatal("race loser private scalar was not zeroized")
	}

	loadedKey, loadedBinding, err :=
		store.Load(
			context,
			member,
			password,
		)
	if err != nil {
		t.Fatal(err)
	}
	defer zeroRabbitVRFDKGPrivateKeyV1(loadedKey)

	if loadedBinding != winnerBinding {
		t.Fatal("persisted binding differs from race winner")
	}

	if loadedKey == nil ||
		loadedKey.D == nil ||
		loadedKey.D.Cmp(winnerKey.D) != 0 {
		t.Fatal("persisted private key differs from race winner")
	}
}
