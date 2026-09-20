package lqc

import (
	"crypto/ecdsa"
	"crypto/rand"
	"errors"
	"testing"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/ecies"
)

func rabbitVRFDKGTransportKeyFixtureV1(
	t *testing.T,
) (
	RabbitVRFDKGSessionContextV1,
	RabbitVRFCommitteeMemberV1,
	*ecdsa.PrivateKey,
	*ecdsa.PrivateKey,
	RabbitVRFDKGTransportKeyBindingV1,
	RabbitVRFDKGEnvelopeV1,
) {
	t.Helper()

	context :=
		rabbitVRFDKGTestSessionContextV1(
			t,
			9280,
			11,
		)

	participantKey, err :=
		crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}

	transportKey, err :=
		crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}

	member := RabbitVRFCommitteeMemberV1{
		ShareID: 7,
		TicketHash: crypto.Keccak256Hash(
			[]byte(
				"rabbit-vrf-dkg-transport-key-ticket",
			),
		),
		Participant: crypto.PubkeyToAddress(
			participantKey.PublicKey,
		),
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

	binding, root, err :=
		NewRabbitVRFDKGTransportKeyBindingV1(
			context,
			member,
			publicKey,
		)
	if err != nil {
		t.Fatal(err)
	}

	envelope, err :=
		NewRabbitVRFDKGEnvelopeV1(
			context,
			member,
			RabbitVRFDKGMessageTransportKeyBindingV1,
			root,
		)
	if err != nil {
		t.Fatal(err)
	}

	signingHash, err :=
		RabbitVRFDKGEnvelopeSigningHashV1(
			context,
			envelope,
		)
	if err != nil {
		t.Fatal(err)
	}

	envelope.Signature, err =
		crypto.Sign(
			signingHash[:],
			participantKey,
		)
	if err != nil {
		t.Fatal(err)
	}

	return context,
		member,
		participantKey,
		transportKey,
		binding,
		envelope
}

func TestRabbitVRFDKGTransportKeyV1CanonicalBinding(
	t *testing.T,
) {
	context,
		member,
		_,
		transportKey,
		binding,
		_ :=
		rabbitVRFDKGTransportKeyFixtureV1(t)

	root, err :=
		VerifyRabbitVRFDKGTransportKeyBindingV1(
			context,
			member,
			binding,
		)
	if err != nil {
		t.Fatal(err)
	}

	secondRoot, err :=
		RabbitVRFDKGTransportKeyRootV1(
			context,
			binding,
		)
	if err != nil {
		t.Fatal(err)
	}

	if root == (common.Hash{}) ||
		root != secondRoot {
		t.Fatal(
			"transport key root is not deterministic",
		)
	}

	expected, err :=
		RabbitVRFDKGTransportPublicKeyV1FromBytes(
			crypto.CompressPubkey(
				&transportKey.PublicKey,
			),
		)
	if err != nil {
		t.Fatal(err)
	}

	if binding.PublicKey != expected {
		t.Fatal(
			"transport public key encoding mismatch",
		)
	}

	if binding.ShareID != member.ShareID ||
		binding.Participant !=
			member.Participant {
		t.Fatal(
			"transport key binding lost canonical member identity",
		)
	}
}

func TestRabbitVRFDKGTransportKeyV1PublicKeyParsing(
	t *testing.T,
) {
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}

	encoded :=
		crypto.CompressPubkey(
			&key.PublicKey,
		)

	parsed, err :=
		RabbitVRFDKGTransportPublicKeyV1FromBytes(
			encoded,
		)
	if err != nil {
		t.Fatal(err)
	}

	if err :=
		ValidateRabbitVRFDKGTransportPublicKeyV1(
			parsed,
		); err != nil {
		t.Fatal(err)
	}

	if _, err :=
		RabbitVRFDKGTransportPublicKeyV1FromBytes(
			encoded[:32],
		); err == nil {
		t.Fatal(
			"short compressed key accepted",
		)
	}

	tooLong :=
		append(
			append([]byte(nil), encoded...),
			byte(0),
		)

	if _, err :=
		RabbitVRFDKGTransportPublicKeyV1FromBytes(
			tooLong,
		); err == nil {
		t.Fatal(
			"long compressed key accepted",
		)
	}

	malformed :=
		make(
			[]byte,
			RabbitVRFDKGTransportPublicKeySizeV1,
		)
	malformed[0] = 4

	if _, err :=
		RabbitVRFDKGTransportPublicKeyV1FromBytes(
			malformed,
		); err == nil {
		t.Fatal(
			"malformed compressed key accepted",
		)
	}
}

func TestRabbitVRFDKGTransportKeyV1RejectsParticipantWalletKeyReuse(
	t *testing.T,
) {
	context :=
		rabbitVRFDKGTestSessionContextV1(
			t,
			9280,
			11,
		)

	participantKey, err :=
		crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}

	member := RabbitVRFCommitteeMemberV1{
		ShareID: 7,
		TicketHash: crypto.Keccak256Hash(
			[]byte(
				"rabbit-vrf-dkg-key-reuse-ticket",
			),
		),
		Participant: crypto.PubkeyToAddress(
			participantKey.PublicKey,
		),
	}

	reused, err :=
		RabbitVRFDKGTransportPublicKeyV1FromBytes(
			crypto.CompressPubkey(
				&participantKey.PublicKey,
			),
		)
	if err != nil {
		t.Fatal(err)
	}

	_, _, err =
		NewRabbitVRFDKGTransportKeyBindingV1(
			context,
			member,
			reused,
		)

	if !errors.Is(
		err,
		ErrRabbitVRFDKGTransportKeyReusesParticipantKeyV1,
	) {
		t.Fatalf(
			"wallet key reuse error=%v",
			err,
		)
	}
}

func TestRabbitVRFDKGTransportKeyV1AuthenticatedEnvelope(
	t *testing.T,
) {
	context,
		member,
		_,
		_,
		binding,
		envelope :=
		rabbitVRFDKGTransportKeyFixtureV1(t)

	if err :=
		VerifyRabbitVRFDKGTransportKeyEnvelopeV1(
			context,
			member,
			binding,
			envelope,
		); err != nil {
		t.Fatal(err)
	}

	if err :=
		VerifyRabbitVRFDKGEnvelopeV1(
			context,
			member,
			envelope,
		); err != nil {
		t.Fatal(err)
	}
}

func TestRabbitVRFDKGTransportKeyV1RejectsTampering(
	t *testing.T,
) {
	context,
		member,
		_,
		_,
		binding,
		envelope :=
		rabbitVRFDKGTransportKeyFixtureV1(t)

	otherKey, err :=
		crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}

	otherPublicKey, err :=
		RabbitVRFDKGTransportPublicKeyV1FromBytes(
			crypto.CompressPubkey(
				&otherKey.PublicKey,
			),
		)
	if err != nil {
		t.Fatal(err)
	}

	changedBinding := binding
	changedBinding.PublicKey =
		otherPublicKey

	if err :=
		VerifyRabbitVRFDKGTransportKeyEnvelopeV1(
			context,
			member,
			changedBinding,
			envelope,
		); err == nil {
		t.Fatal(
			"changed transport key accepted with old envelope",
		)
	}

	changedEnvelope := envelope
	changedEnvelope.PayloadHash =
		crypto.Keccak256Hash(
			[]byte(
				"wrong-transport-key-root",
			),
		)

	if err :=
		VerifyRabbitVRFDKGTransportKeyEnvelopeV1(
			context,
			member,
			binding,
			changedEnvelope,
		); err == nil {
		t.Fatal(
			"changed envelope payload accepted",
		)
	}

	wrongMember := member
	wrongMember.ShareID++

	if _, err :=
		VerifyRabbitVRFDKGTransportKeyBindingV1(
			context,
			wrongMember,
			binding,
		); !errors.Is(
		err,
		ErrRabbitVRFDKGTransportKeySenderMismatchV1,
	) {
		t.Fatalf(
			"wrong member error=%v",
			err,
		)
	}
}

func TestRabbitVRFDKGTransportKeyV1BindsSessionAndChain(
	t *testing.T,
) {
	_,
		member,
		_,
		_,
		binding,
		envelope :=
		rabbitVRFDKGTransportKeyFixtureV1(t)

	otherEpoch :=
		rabbitVRFDKGTestSessionContextV1(
			t,
			9280,
			12,
		)

	if err :=
		VerifyRabbitVRFDKGTransportKeyEnvelopeV1(
			otherEpoch,
			member,
			binding,
			envelope,
		); err == nil {
		t.Fatal(
			"cross-session transport key accepted",
		)
	}

	otherChain :=
		rabbitVRFDKGTestSessionContextV1(
			t,
			9281,
			11,
		)

	if err :=
		VerifyRabbitVRFDKGTransportKeyEnvelopeV1(
			otherChain,
			member,
			binding,
			envelope,
		); err == nil {
		t.Fatal(
			"cross-chain transport key accepted",
		)
	}
}

func TestRabbitVRFDKGTransportKeyV1SignedEquivocation(
	t *testing.T,
) {
	context,
		member,
		participantKey,
		_,
		_,
		_ :=
		rabbitVRFDKGTransportKeyFixtureV1(t)

	makeEnvelope := func(
		transportKey *ecdsa.PrivateKey,
	) (
		RabbitVRFDKGTransportKeyBindingV1,
		RabbitVRFDKGEnvelopeV1,
	) {
		t.Helper()

		publicKey, err :=
			RabbitVRFDKGTransportPublicKeyV1FromBytes(
				crypto.CompressPubkey(
					&transportKey.PublicKey,
				),
			)
		if err != nil {
			t.Fatal(err)
		}

		binding, root, err :=
			NewRabbitVRFDKGTransportKeyBindingV1(
				context,
				member,
				publicKey,
			)
		if err != nil {
			t.Fatal(err)
		}

		envelope, err :=
			NewRabbitVRFDKGEnvelopeV1(
				context,
				member,
				RabbitVRFDKGMessageTransportKeyBindingV1,
				root,
			)
		if err != nil {
			t.Fatal(err)
		}

		signingHash, err :=
			RabbitVRFDKGEnvelopeSigningHashV1(
				context,
				envelope,
			)
		if err != nil {
			t.Fatal(err)
		}

		envelope.Signature, err =
			crypto.Sign(
				signingHash[:],
				participantKey,
			)
		if err != nil {
			t.Fatal(err)
		}

		return binding, envelope
	}

	firstKey, err :=
		crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}

	secondKey, err :=
		crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}

	firstBinding, first :=
		makeEnvelope(firstKey)

	secondBinding, second :=
		makeEnvelope(secondKey)

	if err :=
		VerifyRabbitVRFDKGTransportKeyEnvelopeV1(
			context,
			member,
			firstBinding,
			first,
		); err != nil {
		t.Fatal(err)
	}

	if err :=
		VerifyRabbitVRFDKGTransportKeyEnvelopeV1(
			context,
			member,
			secondBinding,
			second,
		); err != nil {
		t.Fatal(err)
	}

	firstSlot, err :=
		RabbitVRFDKGEnvelopeSlotIDV1(
			context,
			first,
		)
	if err != nil {
		t.Fatal(err)
	}

	secondSlot, err :=
		RabbitVRFDKGEnvelopeSlotIDV1(
			context,
			second,
		)
	if err != nil {
		t.Fatal(err)
	}

	firstID, err :=
		RabbitVRFDKGEnvelopeIDV1(
			context,
			first,
		)
	if err != nil {
		t.Fatal(err)
	}

	secondID, err :=
		RabbitVRFDKGEnvelopeIDV1(
			context,
			second,
		)
	if err != nil {
		t.Fatal(err)
	}

	if firstSlot != secondSlot {
		t.Fatal(
			"transport key equivocation changed singleton slot",
		)
	}

	if firstID == secondID {
		t.Fatal(
			"conflicting transport keys produced same envelope identity",
		)
	}
}

func TestRabbitVRFDKGTransportKeyV1WalletSignData(
	t *testing.T,
) {
	context :=
		rabbitVRFDKGTestSessionContextV1(
			t,
			9280,
			11,
		)

	store :=
		keystore.NewKeyStore(
			t.TempDir(),
			keystore.LightScryptN,
			keystore.LightScryptP,
		)

	account, err :=
		store.NewAccount(
			"rabbit-vrf-dkg-transport-test",
		)
	if err != nil {
		t.Fatal(err)
	}

	if err :=
		store.Unlock(
			account,
			"rabbit-vrf-dkg-transport-test",
		); err != nil {
		t.Fatal(err)
	}

	member := RabbitVRFCommitteeMemberV1{
		ShareID: 7,
		TicketHash: crypto.Keccak256Hash(
			[]byte(
				"rabbit-vrf-dkg-transport-wallet-ticket",
			),
		),
		Participant: account.Address,
	}

	transportKey, err :=
		crypto.GenerateKey()
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

	binding, root, err :=
		NewRabbitVRFDKGTransportKeyBindingV1(
			context,
			member,
			publicKey,
		)
	if err != nil {
		t.Fatal(err)
	}

	envelope, err :=
		NewRabbitVRFDKGEnvelopeV1(
			context,
			member,
			RabbitVRFDKGMessageTransportKeyBindingV1,
			root,
		)
	if err != nil {
		t.Fatal(err)
	}

	signingData, err :=
		RabbitVRFDKGEnvelopeSigningDataV1(
			context,
			envelope,
		)
	if err != nil {
		t.Fatal(err)
	}

	wallets := store.Wallets()

	if len(wallets) != 1 {
		t.Fatalf(
			"wallets=%d want=1",
			len(wallets),
		)
	}

	envelope.Signature, err =
		wallets[0].SignData(
			account,
			accounts.MimetypeClique,
			signingData,
		)
	if err != nil {
		t.Fatal(err)
	}

	if err :=
		VerifyRabbitVRFDKGTransportKeyEnvelopeV1(
			context,
			member,
			binding,
			envelope,
		); err != nil {
		t.Fatalf(
			"Wallet.SignData transport-key envelope rejected: %v",
			err,
		)
	}
}

func TestRabbitVRFDKGTransportKeyV1ECIESRoundTrip(
	t *testing.T,
) {
	context,
		member,
		_,
		transportPrivateKey,
		binding,
		envelope :=
		rabbitVRFDKGTransportKeyFixtureV1(t)

	if err :=
		VerifyRabbitVRFDKGTransportKeyEnvelopeV1(
			context,
			member,
			binding,
			envelope,
		); err != nil {
		t.Fatal(err)
	}

	if binding.Scheme !=
		RabbitVRFDKGTransportSchemeECIESSecp256k1AES128SHA256V1 {
		t.Fatalf(
			"unexpected transport scheme=%d",
			binding.Scheme,
		)
	}

	publicKey, err :=
		parseRabbitVRFDKGTransportPublicKeyV1(
			binding.PublicKey,
		)
	if err != nil {
		t.Fatal(err)
	}

	plaintext := []byte(
		"rabbit-vrf-dkg-transport-key-round-trip-v1",
	)

	ciphertext, err :=
		ecies.Encrypt(
			rand.Reader,
			ecies.ImportECDSAPublic(publicKey),
			plaintext,
			nil,
			nil,
		)
	if err != nil {
		t.Fatal(err)
	}

	if len(ciphertext) == 0 {
		t.Fatal("empty ECIES ciphertext")
	}

	recovered, err :=
		ecies.ImportECDSA(
			transportPrivateKey,
		).Decrypt(
			ciphertext,
			nil,
			nil,
		)
	if err != nil {
		t.Fatal(err)
	}

	if string(recovered) != string(plaintext) {
		t.Fatal(
			"ECIES plaintext round-trip mismatch",
		)
	}

	wrongKey, err :=
		crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}

	if _, err :=
		ecies.ImportECDSA(
			wrongKey,
		).Decrypt(
			ciphertext,
			nil,
			nil,
		); err == nil {
		t.Fatal(
			"ECIES ciphertext decrypted with wrong transport key",
		)
	}
}
