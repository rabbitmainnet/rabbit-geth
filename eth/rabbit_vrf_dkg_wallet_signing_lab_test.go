//go:build (rabbit_workv1_engine_lab || rabbit_workv1) && rabbit_randomx

package eth

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/crypto"
)

func TestRabbitVRFDKGWalletSigningV1TransportBinding(
	t *testing.T,
) {
	store := keystore.NewKeyStore(
		t.TempDir(),
		keystore.LightScryptN,
		keystore.LightScryptP,
	)

	password := "rabbit-vrf-runtime-wallet-signing-test"

	account, err := store.NewAccount(password)
	if err != nil {
		t.Fatal(err)
	}

	if err := store.Unlock(account, password); err != nil {
		t.Fatal(err)
	}

	context, err := lqc.NewRabbitVRFDKGSessionContextV1(
		big.NewInt(9280),
		11,
		crypto.Keccak256Hash(
			[]byte("rabbit-vrf-runtime-wallet-signing-committee"),
		),
		32,
	)
	if err != nil {
		t.Fatal(err)
	}

	member := lqc.RabbitVRFCommitteeMemberV1{
		ShareID: 7,
		TicketHash: crypto.Keccak256Hash(
			[]byte("rabbit-vrf-runtime-wallet-signing-ticket"),
		),
		Participant: account.Address,
	}

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

	binding, _, err :=
		lqc.NewRabbitVRFDKGTransportKeyBindingV1(
			context,
			member,
			publicKey,
		)
	if err != nil {
		t.Fatal(err)
	}

	envelope, err :=
		rabbitVRFDKGSignTransportBindingEnvelopeV1(
			store.Wallets(),
			context,
			member,
			binding,
		)
	if err != nil {
		t.Fatal(err)
	}

	if len(envelope.Signature) == 0 {
		t.Fatal("transport binding envelope has no signature")
	}

	if err :=
		lqc.VerifyRabbitVRFDKGTransportKeyEnvelopeV1(
			context,
			member,
			binding,
			envelope,
		); err != nil {
		t.Fatalf(
			"signed transport binding envelope rejected: %v",
			err,
		)
	}
}

func TestRabbitVRFDKGWalletSigningV1RejectsWrongWallet(
	t *testing.T,
) {
	store := keystore.NewKeyStore(
		t.TempDir(),
		keystore.LightScryptN,
		keystore.LightScryptP,
	)

	wrongAccount, err := store.NewAccount("wrong-wallet-password")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Unlock(wrongAccount, "wrong-wallet-password"); err != nil {
		t.Fatal(err)
	}

	participantKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}

	context, err := lqc.NewRabbitVRFDKGSessionContextV1(
		big.NewInt(9280),
		11,
		crypto.Keccak256Hash(
			[]byte("rabbit-vrf-runtime-wrong-wallet-committee"),
		),
		32,
	)
	if err != nil {
		t.Fatal(err)
	}

	member := lqc.RabbitVRFCommitteeMemberV1{
		ShareID: 7,
		TicketHash: crypto.Keccak256Hash(
			[]byte("rabbit-vrf-runtime-wrong-wallet-ticket"),
		),
		Participant: crypto.PubkeyToAddress(
			participantKey.PublicKey,
		),
	}

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

	binding, _, err :=
		lqc.NewRabbitVRFDKGTransportKeyBindingV1(
			context,
			member,
			publicKey,
		)
	if err != nil {
		t.Fatal(err)
	}

	envelope, err :=
		rabbitVRFDKGSignTransportBindingEnvelopeV1(
			store.Wallets(),
			context,
			member,
			binding,
		)
	if err == nil {
		t.Fatal("wrong wallet signed participant transport binding")
	}
	if len(envelope.Signature) != 0 {
		t.Fatal("failed signing returned a signature")
	}
}
