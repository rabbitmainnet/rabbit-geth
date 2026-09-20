package lqc

import (
	"errors"
	"testing"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

func rabbitVRFDKGEnvelopeTestObjectsV1(
	t *testing.T,
) (
	RabbitVRFDKGSessionContextV1,
	RabbitVRFCommitteeMemberV1,
	RabbitVRFDKGPolynomialCommitmentV1,
	RabbitVRFDKGEnvelopeV1,
) {
	t.Helper()

	context :=
		rabbitVRFDKGTestSessionContextV1(
			t,
			9280,
			11,
		)

	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}

	participant :=
		crypto.PubkeyToAddress(
			key.PublicKey,
		)

	member := RabbitVRFCommitteeMemberV1{
		ShareID: 7,
		TicketHash: crypto.Keccak256Hash(
			[]byte("rabbit-vrf-dkg-envelope-ticket"),
		),
		Participant: participant,
	}

	coefficients :=
		rabbitVRFDKGTestCommitmentCoefficientsV1(
			context.Threshold,
		)

	commitment, root, err :=
		NewRabbitVRFDKGPolynomialCommitmentV1(
			context,
			member.ShareID,
			coefficients,
		)
	if err != nil {
		t.Fatal(err)
	}

	envelope, err :=
		NewRabbitVRFDKGEnvelopeV1(
			context,
			member,
			RabbitVRFDKGMessagePolynomialCommitmentV1,
			root,
		)
	if err != nil {
		t.Fatal(err)
	}

	hash, err :=
		RabbitVRFDKGEnvelopeSigningHashV1(
			context,
			envelope,
		)
	if err != nil {
		t.Fatal(err)
	}

	envelope.Signature, err =
		crypto.Sign(hash[:], key)
	if err != nil {
		t.Fatal(err)
	}

	return context,
		member,
		commitment,
		envelope
}

func TestRabbitVRFDKGEnvelopeV1ValidSignature(
	t *testing.T,
) {
	context,
		member,
		commitment,
		envelope :=
		rabbitVRFDKGEnvelopeTestObjectsV1(t)

	payloadHash, err :=
		RabbitVRFDKGPolynomialCommitmentPayloadHashV1(
			context,
			commitment,
		)
	if err != nil {
		t.Fatal(err)
	}

	if payloadHash != envelope.PayloadHash {
		t.Fatal("polynomial commitment payload hash mismatch")
	}

	if err := VerifyRabbitVRFDKGEnvelopeV1(
		context,
		member,
		envelope,
	); err != nil {
		t.Fatal(err)
	}

	slotID, err :=
		RabbitVRFDKGEnvelopeSlotIDV1(
			context,
			envelope,
		)
	if err != nil {
		t.Fatal(err)
	}

	envelopeID, err :=
		RabbitVRFDKGEnvelopeIDV1(
			context,
			envelope,
		)
	if err != nil {
		t.Fatal(err)
	}

	if slotID == (common.Hash{}) ||
		envelopeID == (common.Hash{}) {
		t.Fatal("zero canonical envelope identity")
	}
}

func TestRabbitVRFDKGEnvelopeV1DeterministicIdentity(
	t *testing.T,
) {
	context,
		_,
		_,
		envelope :=
		rabbitVRFDKGEnvelopeTestObjectsV1(t)

	firstSlot, err :=
		RabbitVRFDKGEnvelopeSlotIDV1(
			context,
			envelope,
		)
	if err != nil {
		t.Fatal(err)
	}

	secondSlot, err :=
		RabbitVRFDKGEnvelopeSlotIDV1(
			context,
			envelope,
		)
	if err != nil {
		t.Fatal(err)
	}

	firstID, err :=
		RabbitVRFDKGEnvelopeIDV1(
			context,
			envelope,
		)
	if err != nil {
		t.Fatal(err)
	}

	secondID, err :=
		RabbitVRFDKGEnvelopeIDV1(
			context,
			envelope,
		)
	if err != nil {
		t.Fatal(err)
	}

	if firstSlot != secondSlot ||
		firstID != secondID {
		t.Fatal("envelope identity is not deterministic")
	}

	changedSignature := envelope
	changedSignature.Signature =
		append(
			[]byte(nil),
			envelope.Signature...,
		)
	changedSignature.Signature[10] ^= 0x01

	signatureIndependentID, err :=
		RabbitVRFDKGEnvelopeIDV1(
			context,
			changedSignature,
		)
	if err != nil {
		t.Fatal(err)
	}

	if signatureIndependentID != firstID {
		t.Fatal("signature bytes changed canonical envelope identity")
	}
}

func TestRabbitVRFDKGEnvelopeV1EquivocationSlot(
	t *testing.T,
) {
	context,
		member,
		_,
		first :=
		rabbitVRFDKGEnvelopeTestObjectsV1(t)

	second := first
	second.PayloadHash =
		crypto.Keccak256Hash(
			[]byte("different-public-commitment"),
		)
	second.Signature = nil

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
			"same sender singleton message produced different equivocation slot",
		)
	}

	if firstID == secondID {
		t.Fatal(
			"different payloads produced same canonical envelope identity",
		)
	}

	if second.SenderShareID != member.ShareID {
		t.Fatal("test sender ShareID changed unexpectedly")
	}
}

func TestRabbitVRFDKGEnvelopeV1RejectsTampering(
	t *testing.T,
) {
	context,
		member,
		_,
		envelope :=
		rabbitVRFDKGEnvelopeTestObjectsV1(t)

	tests := []struct {
		name   string
		mutate func(
			*RabbitVRFDKGEnvelopeV1,
		)
	}{
		{
			name: "wrong session",
			mutate: func(
				item *RabbitVRFDKGEnvelopeV1,
			) {
				item.SessionID =
					crypto.Keccak256Hash(
						[]byte("wrong-session"),
					)
			},
		},
		{
			name: "wrong message type",
			mutate: func(
				item *RabbitVRFDKGEnvelopeV1,
			) {
				item.MessageType = 99
			},
		},
		{
			name: "wrong sender share id",
			mutate: func(
				item *RabbitVRFDKGEnvelopeV1,
			) {
				item.SenderShareID = 8
			},
		},
		{
			name: "wrong participant",
			mutate: func(
				item *RabbitVRFDKGEnvelopeV1,
			) {
				item.Participant =
					common.HexToAddress(
						"0x1234",
					)
			},
		},
		{
			name: "wrong payload",
			mutate: func(
				item *RabbitVRFDKGEnvelopeV1,
			) {
				item.PayloadHash =
					crypto.Keccak256Hash(
						[]byte("wrong-payload"),
					)
			},
		},
		{
			name: "bad signature",
			mutate: func(
				item *RabbitVRFDKGEnvelopeV1,
			) {
				item.Signature =
					append(
						[]byte(nil),
						item.Signature...,
					)
				item.Signature[5] ^= 0x01
			},
		},
		{
			name: "short signature",
			mutate: func(
				item *RabbitVRFDKGEnvelopeV1,
			) {
				item.Signature =
					item.Signature[:64]
			},
		},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				changed := envelope
				changed.Signature =
					append(
						[]byte(nil),
						envelope.Signature...,
					)

				test.mutate(&changed)

				if err :=
					VerifyRabbitVRFDKGEnvelopeV1(
						context,
						member,
						changed,
					); err == nil {
					t.Fatal("tampered envelope accepted")
				}
			},
		)
	}
}

func TestRabbitVRFDKGEnvelopeV1RejectsWrongCanonicalMember(
	t *testing.T,
) {
	context,
		member,
		_,
		envelope :=
		rabbitVRFDKGEnvelopeTestObjectsV1(t)

	wrongShareID := member
	wrongShareID.ShareID = 8

	if err := VerifyRabbitVRFDKGEnvelopeV1(
		context,
		wrongShareID,
		envelope,
	); !errors.Is(
		err,
		ErrRabbitVRFDKGEnvelopeSenderMismatchV1,
	) {
		t.Fatalf(
			"wrong ShareID error=%v",
			err,
		)
	}

	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}

	wrongParticipant := member
	wrongParticipant.Participant =
		crypto.PubkeyToAddress(
			key.PublicKey,
		)

	if err := VerifyRabbitVRFDKGEnvelopeV1(
		context,
		wrongParticipant,
		envelope,
	); !errors.Is(
		err,
		ErrRabbitVRFDKGEnvelopeSenderMismatchV1,
	) {
		t.Fatalf(
			"wrong Participant error=%v",
			err,
		)
	}
}

func TestRabbitVRFDKGEnvelopeV1BindsSession(
	t *testing.T,
) {
	context,
		member,
		_,
		envelope :=
		rabbitVRFDKGEnvelopeTestObjectsV1(t)

	otherContext :=
		rabbitVRFDKGTestSessionContextV1(
			t,
			9280,
			12,
		)

	if err := VerifyRabbitVRFDKGEnvelopeV1(
		otherContext,
		member,
		envelope,
	); err == nil {
		t.Fatal("cross-session envelope accepted")
	}

	firstSlot, err :=
		RabbitVRFDKGEnvelopeSlotIDV1(
			context,
			envelope,
		)
	if err != nil {
		t.Fatal(err)
	}

	otherEnvelope := envelope
	otherSessionID, err :=
		RabbitVRFDKGSessionIDV1(
			otherContext,
		)
	if err != nil {
		t.Fatal(err)
	}
	otherEnvelope.SessionID =
		otherSessionID

	otherSlot, err :=
		RabbitVRFDKGEnvelopeSlotIDV1(
			otherContext,
			otherEnvelope,
		)
	if err != nil {
		t.Fatal(err)
	}

	if firstSlot == otherSlot {
		t.Fatal(
			"different DKG sessions produced same slot identity",
		)
	}
}

func TestRabbitVRFDKGEnvelopeV1RejectsCrossChainReplay(
	t *testing.T,
) {
	context,
		member,
		_,
		envelope :=
		rabbitVRFDKGEnvelopeTestObjectsV1(t)

	otherContext :=
		rabbitVRFDKGTestSessionContextV1(
			t,
			9281,
			11,
		)

	if err := VerifyRabbitVRFDKGEnvelopeV1(
		otherContext,
		member,
		envelope,
	); err == nil {
		t.Fatal("cross-chain envelope accepted")
	}

	firstSlot, err :=
		RabbitVRFDKGEnvelopeSlotIDV1(
			context,
			envelope,
		)
	if err != nil {
		t.Fatal(err)
	}

	otherEnvelope := envelope

	otherSessionID, err :=
		RabbitVRFDKGSessionIDV1(
			otherContext,
		)
	if err != nil {
		t.Fatal(err)
	}

	otherEnvelope.SessionID =
		otherSessionID

	otherSlot, err :=
		RabbitVRFDKGEnvelopeSlotIDV1(
			otherContext,
			otherEnvelope,
		)
	if err != nil {
		t.Fatal(err)
	}

	if firstSlot == otherSlot {
		t.Fatal(
			"different chains produced same DKG envelope slot",
		)
	}
}

func TestRabbitVRFDKGEnvelopeV1SignedEquivocationEvidence(
	t *testing.T,
) {
	context :=
		rabbitVRFDKGTestSessionContextV1(
			t,
			9280,
			11,
		)

	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}

	participant :=
		crypto.PubkeyToAddress(
			key.PublicKey,
		)

	member := RabbitVRFCommitteeMemberV1{
		ShareID: 7,
		TicketHash: crypto.Keccak256Hash(
			[]byte(
				"rabbit-vrf-dkg-signed-equivocation-ticket",
			),
		),
		Participant: participant,
	}

	firstPayload :=
		crypto.Keccak256Hash(
			[]byte("first-public-dkg-object"),
		)

	secondPayload :=
		crypto.Keccak256Hash(
			[]byte("second-public-dkg-object"),
		)

	first, err :=
		NewRabbitVRFDKGEnvelopeV1(
			context,
			member,
			RabbitVRFDKGMessagePolynomialCommitmentV1,
			firstPayload,
		)
	if err != nil {
		t.Fatal(err)
	}

	second, err :=
		NewRabbitVRFDKGEnvelopeV1(
			context,
			member,
			RabbitVRFDKGMessagePolynomialCommitmentV1,
			secondPayload,
		)
	if err != nil {
		t.Fatal(err)
	}

	firstHash, err :=
		RabbitVRFDKGEnvelopeSigningHashV1(
			context,
			first,
		)
	if err != nil {
		t.Fatal(err)
	}

	first.Signature, err =
		crypto.Sign(
			firstHash[:],
			key,
		)
	if err != nil {
		t.Fatal(err)
	}

	secondHash, err :=
		RabbitVRFDKGEnvelopeSigningHashV1(
			context,
			second,
		)
	if err != nil {
		t.Fatal(err)
	}

	second.Signature, err =
		crypto.Sign(
			secondHash[:],
			key,
		)
	if err != nil {
		t.Fatal(err)
	}

	if err := VerifyRabbitVRFDKGEnvelopeV1(
		context,
		member,
		first,
	); err != nil {
		t.Fatalf(
			"first signed envelope rejected: %v",
			err,
		)
	}

	if err := VerifyRabbitVRFDKGEnvelopeV1(
		context,
		member,
		second,
	); err != nil {
		t.Fatalf(
			"second signed envelope rejected: %v",
			err,
		)
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
			"signed equivocation did not share the same slot",
		)
	}

	if firstID == secondID {
		t.Fatal(
			"signed equivocation produced the same envelope identity",
		)
	}
}

func TestRabbitVRFDKGEnvelopeV1WalletSignDataMatchesVerification(
	t *testing.T,
) {
	context :=
		rabbitVRFDKGTestSessionContextV1(
			t,
			9280,
			11,
		)

	store := keystore.NewKeyStore(
		t.TempDir(),
		keystore.LightScryptN,
		keystore.LightScryptP,
	)

	account, err :=
		store.NewAccount(
			"rabbit-vrf-dkg-test",
		)
	if err != nil {
		t.Fatal(err)
	}

	if err := store.Unlock(
		account,
		"rabbit-vrf-dkg-test",
	); err != nil {
		t.Fatal(err)
	}

	member := RabbitVRFCommitteeMemberV1{
		ShareID: 7,
		TicketHash: crypto.Keccak256Hash(
			[]byte(
				"rabbit-vrf-dkg-wallet-signing-ticket",
			),
		),
		Participant: account.Address,
	}

	coefficients :=
		rabbitVRFDKGTestCommitmentCoefficientsV1(
			context.Threshold,
		)

	_, payloadHash, err :=
		NewRabbitVRFDKGPolynomialCommitmentV1(
			context,
			member.ShareID,
			coefficients,
		)
	if err != nil {
		t.Fatal(err)
	}

	envelope, err :=
		NewRabbitVRFDKGEnvelopeV1(
			context,
			member,
			RabbitVRFDKGMessagePolynomialCommitmentV1,
			payloadHash,
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

	if err := VerifyRabbitVRFDKGEnvelopeV1(
		context,
		member,
		envelope,
	); err != nil {
		t.Fatalf(
			"Wallet.SignData signature rejected by DKG envelope verifier: %v",
			err,
		)
	}
}
