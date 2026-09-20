package lqc

import (
	"bytes"
	"crypto/ecdsa"
	"testing"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"

	rabbitvrf "github.com/ethereum/go-ethereum/crypto/rabbitvrf"
)

func rabbitVRFDKGEncryptedEvaluationDealerKeyV1(
	t *testing.T,
) *ecdsa.PrivateKey {
	t.Helper()

	key, err :=
		crypto.HexToECDSA(
			"0000000000000000000000000000000000000000000000000000000000000003",
		)
	if err != nil {
		t.Fatal(err)
	}

	return key
}

func rabbitVRFDKGEncryptedEvaluationFixtureV1(
	t *testing.T,
) (
	RabbitVRFDKGSessionContextV1,
	RabbitVRFCommitteeMemberV1,
	RabbitVRFDKGPolynomialCommitmentV1,
	RabbitVRFCommitteeMemberV1,
	RabbitVRFDKGTransportKeyBindingV1,
	RabbitVRFDKGEnvelopeV1,
	[]byte,
	rabbitvrf.DKGPolynomialEvaluationV1,
	RabbitVRFDKGEncryptedEvaluationV1,
	common.Hash,
	common.Hash,
) {
	t.Helper()

	context,
		recipient,
		_,
		recipientPrivateKey,
		recipientBinding,
		recipientBindingEnvelope :=
		rabbitVRFDKGTransportKeyFixtureV1(t)

	dealerPrivateKey :=
		rabbitVRFDKGEncryptedEvaluationDealerKeyV1(
			t,
		)

	dealer := RabbitVRFCommitteeMemberV1{
		ShareID: 3,
		TicketHash: crypto.Keccak256Hash(
			[]byte(
				"rabbit-vrf-dkg-encrypted-eval-dealer-ticket",
			),
		),
		Participant: crypto.PubkeyToAddress(
			dealerPrivateKey.PublicKey,
		),
	}

	coefficients :=
		rabbitVRFDKGTestCommitmentCoefficientsV1(
			context.Threshold,
		)

	commitment, _, err :=
		NewRabbitVRFDKGPolynomialCommitmentV1(
			context,
			dealer.ShareID,
			coefficients,
		)
	if err != nil {
		t.Fatal(err)
	}

	evaluation :=
		rabbitVRFDKGEvaluationForTestV1(
			context.Threshold,
			recipient.ShareID,
		)

	message,
		slotID,
		messageID,
		err :=
		NewRabbitVRFDKGEncryptedEvaluationV1(
			context,
			dealer,
			commitment,
			recipient,
			recipientBinding,
			recipientBindingEnvelope,
			evaluation,
		)
	if err != nil {
		t.Fatal(err)
	}

	signingHash, err :=
		RabbitVRFDKGEncryptedEvaluationSigningHashV1(
			context,
			message,
		)
	if err != nil {
		t.Fatal(err)
	}

	message.Signature, err =
		crypto.Sign(
			signingHash[:],
			dealerPrivateKey,
		)
	if err != nil {
		t.Fatal(err)
	}

	if err :=
		VerifyRabbitVRFDKGEncryptedEvaluationSignatureV1(
			context,
			dealer,
			message,
		); err != nil {
		t.Fatal(err)
	}

	recipientPrivateKeyBytes :=
		crypto.FromECDSA(
			recipientPrivateKey,
		)

	return context,
		dealer,
		commitment,
		recipient,
		recipientBinding,
		recipientBindingEnvelope,
		recipientPrivateKeyBytes,
		evaluation,
		message,
		slotID,
		messageID
}

func rabbitVRFDKGEncryptedEvaluationPrivateKeyV1(
	t *testing.T,
	encoded []byte,
) *ecdsa.PrivateKey {
	t.Helper()

	key, err :=
		crypto.ToECDSA(encoded)
	if err != nil {
		t.Fatal(err)
	}

	return key
}

func TestRabbitVRFDKGEncryptedEvaluationV1RoundTrip(
	t *testing.T,
) {
	context,
		dealer,
		commitment,
		recipient,
		binding,
		bindingEnvelope,
		privateKeyBytes,
		expected,
		message,
		slotID,
		messageID :=
		rabbitVRFDKGEncryptedEvaluationFixtureV1(t)

	if slotID == (common.Hash{}) {
		t.Fatal("zero encrypted-evaluation SlotID")
	}

	if messageID == (common.Hash{}) {
		t.Fatal("zero encrypted-evaluation MessageID")
	}

	if len(message.Ciphertext) !=
		RabbitVRFDKGEncryptedEvaluationCiphertextSizeV1 {
		t.Fatalf(
			"ciphertext size=%d want=%d",
			len(message.Ciphertext),
			RabbitVRFDKGEncryptedEvaluationCiphertextSizeV1,
		)
	}

	recipientPrivateKey :=
		rabbitVRFDKGEncryptedEvaluationPrivateKeyV1(
			t,
			privateKeyBytes,
		)

	recovered, err :=
		DecryptRabbitVRFDKGEncryptedEvaluationV1(
			context,
			dealer,
			commitment,
			recipient,
			binding,
			bindingEnvelope,
			recipientPrivateKey,
			message,
		)
	if err != nil {
		t.Fatal(err)
	}

	if recovered != expected {
		t.Fatal(
			"decrypted evaluation differs from original",
		)
	}
}

func TestRabbitVRFDKGEncryptedEvaluationV1SharedInfoDomainSeparation(
	t *testing.T,
) {
	_, _, _, _, _, _, _, _, message, _, _ :=
		rabbitVRFDKGEncryptedEvaluationFixtureV1(t)

	s1, s2, err :=
		rabbitVRFDKGEncryptedEvaluationSharedInfoV1(
			message,
		)
	if err != nil {
		t.Fatal(err)
	}

	if len(s1) != common.HashLength ||
		len(s2) != common.HashLength {
		t.Fatal(
			"shared information is not 32-byte hash material",
		)
	}

	if bytes.Equal(s1, s2) {
		t.Fatal(
			"ECIES s1 and s2 are not domain separated",
		)
	}

	changed := message
	changed.RecipientShareID++

	changedS1, changedS2, err :=
		rabbitVRFDKGEncryptedEvaluationSharedInfoV1(
			changed,
		)
	if err != nil {
		t.Fatal(err)
	}

	if bytes.Equal(s1, changedS1) ||
		bytes.Equal(s2, changedS2) {
		t.Fatal(
			"recipient mutation did not alter shared information",
		)
	}
}

func TestRabbitVRFDKGEncryptedEvaluationV1RandomizedCiphertextStableSlot(
	t *testing.T,
) {
	context,
		dealer,
		commitment,
		recipient,
		binding,
		bindingEnvelope,
		privateKeyBytes,
		evaluation,
		first,
		firstSlot,
		firstID :=
		rabbitVRFDKGEncryptedEvaluationFixtureV1(t)

	second,
		secondSlot,
		secondID,
		err :=
		NewRabbitVRFDKGEncryptedEvaluationV1(
			context,
			dealer,
			commitment,
			recipient,
			binding,
			bindingEnvelope,
			evaluation,
		)
	if err != nil {
		t.Fatal(err)
	}

	dealerPrivateKey :=
		rabbitVRFDKGEncryptedEvaluationDealerKeyV1(
			t,
		)

	secondSigningHash, err :=
		RabbitVRFDKGEncryptedEvaluationSigningHashV1(
			context,
			second,
		)
	if err != nil {
		t.Fatal(err)
	}

	second.Signature, err =
		crypto.Sign(
			secondSigningHash[:],
			dealerPrivateKey,
		)
	if err != nil {
		t.Fatal(err)
	}

	if err :=
		VerifyRabbitVRFDKGEncryptedEvaluationSignatureV1(
			context,
			dealer,
			second,
		); err != nil {
		t.Fatalf(
			"second randomized ciphertext signature rejected: %v",
			err,
		)
	}

	if firstSlot != secondSlot {
		t.Fatal(
			"same dealer/recipient evaluation changed SlotID",
		)
	}

	if firstID == secondID {
		t.Fatal(
			"independent ECIES encryptions produced same MessageID",
		)
	}

	if bytes.Equal(
		first.Ciphertext,
		second.Ciphertext,
	) {
		t.Fatal(
			"independent ECIES encryptions produced same ciphertext",
		)
	}

	recipientPrivateKey :=
		rabbitVRFDKGEncryptedEvaluationPrivateKeyV1(
			t,
			privateKeyBytes,
		)

	for index, message := range []RabbitVRFDKGEncryptedEvaluationV1{
		first,
		second,
	} {
		recovered, err :=
			DecryptRabbitVRFDKGEncryptedEvaluationV1(
				context,
				dealer,
				commitment,
				recipient,
				binding,
				bindingEnvelope,
				recipientPrivateKey,
				message,
			)
		if err != nil {
			t.Fatalf(
				"message=%d err=%v",
				index,
				err,
			)
		}

		if recovered != evaluation {
			t.Fatalf(
				"message=%d evaluation mismatch",
				index,
			)
		}
	}
}

func TestRabbitVRFDKGEncryptedEvaluationV1RejectsCiphertextTampering(
	t *testing.T,
) {
	context,
		dealer,
		commitment,
		recipient,
		binding,
		bindingEnvelope,
		privateKeyBytes,
		_,
		message,
		_,
		_ :=
		rabbitVRFDKGEncryptedEvaluationFixtureV1(t)

	recipientPrivateKey :=
		rabbitVRFDKGEncryptedEvaluationPrivateKeyV1(
			t,
			privateKeyBytes,
		)

	changed := message
	changed.Ciphertext =
		append([]byte(nil), message.Ciphertext...)

	changed.Ciphertext[len(changed.Ciphertext)/2] ^= 0x01

	if _, err :=
		DecryptRabbitVRFDKGEncryptedEvaluationV1(
			context,
			dealer,
			commitment,
			recipient,
			binding,
			bindingEnvelope,
			recipientPrivateKey,
			changed,
		); err == nil {
		t.Fatal(
			"tampered ECIES ciphertext accepted",
		)
	}

	changedID, err :=
		RabbitVRFDKGEncryptedEvaluationIDV1(
			changed,
		)
	if err != nil {
		t.Fatal(err)
	}

	originalID, err :=
		RabbitVRFDKGEncryptedEvaluationIDV1(
			message,
		)
	if err != nil {
		t.Fatal(err)
	}

	if changedID == originalID {
		t.Fatal(
			"ciphertext tampering preserved MessageID",
		)
	}
}

func TestRabbitVRFDKGEncryptedEvaluationV1RejectsWrongTransportPrivateKey(
	t *testing.T,
) {
	context,
		dealer,
		commitment,
		recipient,
		binding,
		bindingEnvelope,
		_,
		_,
		message,
		_,
		_ :=
		rabbitVRFDKGEncryptedEvaluationFixtureV1(t)

	wrongKey, err :=
		crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}

	if _, err :=
		DecryptRabbitVRFDKGEncryptedEvaluationV1(
			context,
			dealer,
			commitment,
			recipient,
			binding,
			bindingEnvelope,
			wrongKey,
			message,
		); err == nil {
		t.Fatal(
			"wrong recipient transport private key accepted",
		)
	}
}

func TestRabbitVRFDKGEncryptedEvaluationV1RejectsMetadataTampering(
	t *testing.T,
) {
	context,
		dealer,
		commitment,
		recipient,
		binding,
		bindingEnvelope,
		privateKeyBytes,
		_,
		message,
		originalSlot,
		_ :=
		rabbitVRFDKGEncryptedEvaluationFixtureV1(t)

	recipientPrivateKey :=
		rabbitVRFDKGEncryptedEvaluationPrivateKeyV1(
			t,
			privateKeyBytes,
		)

	tests := []struct {
		name   string
		mutate func(*RabbitVRFDKGEncryptedEvaluationV1)
	}{
		{
			name: "dealer-share",
			mutate: func(m *RabbitVRFDKGEncryptedEvaluationV1) {
				m.DealerShareID++
			},
		},
		{
			name: "recipient-share",
			mutate: func(m *RabbitVRFDKGEncryptedEvaluationV1) {
				m.RecipientShareID++
			},
		},
		{
			name: "dealer-participant",
			mutate: func(m *RabbitVRFDKGEncryptedEvaluationV1) {
				m.DealerParticipant =
					common.HexToAddress("0x1234")
			},
		},
		{
			name: "recipient-participant",
			mutate: func(m *RabbitVRFDKGEncryptedEvaluationV1) {
				m.RecipientParticipant =
					common.HexToAddress("0x5678")
			},
		},
		{
			name: "commitment-root",
			mutate: func(m *RabbitVRFDKGEncryptedEvaluationV1) {
				m.CommitmentRoot =
					crypto.Keccak256Hash(
						[]byte("wrong-commitment"),
					)
			},
		},
		{
			name: "transport-root",
			mutate: func(m *RabbitVRFDKGEncryptedEvaluationV1) {
				m.RecipientTransportKeyRoot =
					crypto.Keccak256Hash(
						[]byte("wrong-transport-key"),
					)
			},
		},
	}

	for _, tc := range tests {
		t.Run(
			tc.name,
			func(t *testing.T) {
				changed := message
				changed.Ciphertext =
					append(
						[]byte(nil),
						message.Ciphertext...,
					)

				tc.mutate(&changed)

				if _, err :=
					DecryptRabbitVRFDKGEncryptedEvaluationV1(
						context,
						dealer,
						commitment,
						recipient,
						binding,
						bindingEnvelope,
						recipientPrivateKey,
						changed,
					); err == nil {
					t.Fatal(
						"tampered metadata accepted",
					)
				}

				changedSlot, err :=
					RabbitVRFDKGEncryptedEvaluationSlotIDV1(
						changed,
					)
				if err != nil {
					t.Fatal(err)
				}

				if changedSlot == originalSlot {
					t.Fatal(
						"metadata tampering preserved SlotID",
					)
				}
			},
		)
	}
}

func TestRabbitVRFDKGEncryptedEvaluationV1RejectsCrossSessionAndChain(
	t *testing.T,
) {
	_,
		dealer,
		commitment,
		recipient,
		binding,
		bindingEnvelope,
		privateKeyBytes,
		_,
		message,
		_,
		_ :=
		rabbitVRFDKGEncryptedEvaluationFixtureV1(t)

	recipientPrivateKey :=
		rabbitVRFDKGEncryptedEvaluationPrivateKeyV1(
			t,
			privateKeyBytes,
		)

	contexts := []RabbitVRFDKGSessionContextV1{
		rabbitVRFDKGTestSessionContextV1(
			t,
			9280,
			12,
		),
		rabbitVRFDKGTestSessionContextV1(
			t,
			9281,
			11,
		),
	}

	for index, context := range contexts {
		if _, err :=
			DecryptRabbitVRFDKGEncryptedEvaluationV1(
				context,
				dealer,
				commitment,
				recipient,
				binding,
				bindingEnvelope,
				recipientPrivateKey,
				message,
			); err == nil {
			t.Fatalf(
				"cross-context message accepted index=%d",
				index,
			)
		}
	}
}

func TestRabbitVRFDKGEncryptedEvaluationV1RequiresAuthenticatedRecipientKey(
	t *testing.T,
) {
	context,
		dealer,
		commitment,
		recipient,
		_,
		bindingEnvelope,
		_,
		evaluation,
		_,
		_,
		_ :=
		rabbitVRFDKGEncryptedEvaluationFixtureV1(t)

	attackerKey, err :=
		crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}

	attackerPublic, err :=
		RabbitVRFDKGTransportPublicKeyV1FromBytes(
			crypto.CompressPubkey(
				&attackerKey.PublicKey,
			),
		)
	if err != nil {
		t.Fatal(err)
	}

	forgedBinding, _, err :=
		NewRabbitVRFDKGTransportKeyBindingV1(
			context,
			recipient,
			attackerPublic,
		)
	if err != nil {
		t.Fatal(err)
	}

	if _, _, _, err :=
		NewRabbitVRFDKGEncryptedEvaluationV1(
			context,
			dealer,
			commitment,
			recipient,
			forgedBinding,
			bindingEnvelope,
			evaluation,
		); err == nil {
		t.Fatal(
			"unauthenticated replacement transport key accepted",
		)
	}
}

func TestRabbitVRFDKGEncryptedEvaluationV1RejectsSignatureTampering(
	t *testing.T,
) {
	context,
		dealer,
		_,
		_,
		_,
		_,
		_,
		_,
		message,
		_,
		originalID :=
		rabbitVRFDKGEncryptedEvaluationFixtureV1(t)

	changed := message
	changed.Signature =
		append(
			[]byte(nil),
			message.Signature...,
		)

	changed.Signature[10] ^= 0x01

	if err :=
		VerifyRabbitVRFDKGEncryptedEvaluationSignatureV1(
			context,
			dealer,
			changed,
		); err == nil {
		t.Fatal(
			"tampered dealer signature accepted",
		)
	}

	changedID, err :=
		RabbitVRFDKGEncryptedEvaluationIDV1(
			changed,
		)
	if err != nil {
		t.Fatal(err)
	}

	if changedID != originalID {
		t.Fatal(
			"signature bytes changed ciphertext MessageID",
		)
	}
}

func TestRabbitVRFDKGEncryptedEvaluationV1RejectsWrongDealerSigner(
	t *testing.T,
) {
	context,
		dealer,
		_,
		_,
		_,
		_,
		_,
		_,
		message,
		_,
		_ :=
		rabbitVRFDKGEncryptedEvaluationFixtureV1(t)

	wrongKey, err :=
		crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}

	signingHash, err :=
		RabbitVRFDKGEncryptedEvaluationSigningHashV1(
			context,
			message,
		)
	if err != nil {
		t.Fatal(err)
	}

	message.Signature, err =
		crypto.Sign(
			signingHash[:],
			wrongKey,
		)
	if err != nil {
		t.Fatal(err)
	}

	if err :=
		VerifyRabbitVRFDKGEncryptedEvaluationSignatureV1(
			context,
			dealer,
			message,
		); err == nil {
		t.Fatal(
			"ciphertext signed by wrong dealer key accepted",
		)
	}
}

func TestRabbitVRFDKGEncryptedEvaluationV1WalletSignData(
	t *testing.T,
) {
	context,
		recipient,
		_,
		_,
		recipientBinding,
		recipientBindingEnvelope :=
		rabbitVRFDKGTransportKeyFixtureV1(t)

	store :=
		keystore.NewKeyStore(
			t.TempDir(),
			keystore.LightScryptN,
			keystore.LightScryptP,
		)

	account, err :=
		store.NewAccount(
			"rabbit-vrf-dkg-encrypted-evaluation-test",
		)
	if err != nil {
		t.Fatal(err)
	}

	if err :=
		store.Unlock(
			account,
			"rabbit-vrf-dkg-encrypted-evaluation-test",
		); err != nil {
		t.Fatal(err)
	}

	dealer := RabbitVRFCommitteeMemberV1{
		ShareID: 3,
		TicketHash: crypto.Keccak256Hash(
			[]byte(
				"rabbit-vrf-dkg-encrypted-eval-wallet-ticket",
			),
		),
		Participant: account.Address,
	}

	coefficients :=
		rabbitVRFDKGTestCommitmentCoefficientsV1(
			context.Threshold,
		)

	commitment, _, err :=
		NewRabbitVRFDKGPolynomialCommitmentV1(
			context,
			dealer.ShareID,
			coefficients,
		)
	if err != nil {
		t.Fatal(err)
	}

	evaluation :=
		rabbitVRFDKGEvaluationForTestV1(
			context.Threshold,
			recipient.ShareID,
		)

	message, _, _, err :=
		NewRabbitVRFDKGEncryptedEvaluationV1(
			context,
			dealer,
			commitment,
			recipient,
			recipientBinding,
			recipientBindingEnvelope,
			evaluation,
		)
	if err != nil {
		t.Fatal(err)
	}

	signingData, err :=
		RabbitVRFDKGEncryptedEvaluationSigningDataV1(
			context,
			message,
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

	message.Signature, err =
		wallets[0].SignData(
			account,
			accounts.MimetypeClique,
			signingData,
		)
	if err != nil {
		t.Fatal(err)
	}

	if err :=
		VerifyRabbitVRFDKGEncryptedEvaluationSignatureV1(
			context,
			dealer,
			message,
		); err != nil {
		t.Fatalf(
			"Wallet.SignData encrypted-evaluation signature rejected: %v",
			err,
		)
	}
}

func TestRabbitVRFDKGEncryptedEvaluationV1DeterministicVectors(
	t *testing.T,
) {
	ciphertext :=
		make(
			[]byte,
			RabbitVRFDKGEncryptedEvaluationCiphertextSizeV1,
		)

	for index := range ciphertext {
		ciphertext[index] =
			byte(
				(index*17 + 3) & 0xff,
			)
	}

	message := RabbitVRFDKGEncryptedEvaluationV1{
		Version: RabbitVRFDKGEncryptedEvaluationVersionV1,
		SessionID: common.HexToHash(
			"0x111122223333444455556666777788889999aaaabbbbccccddddeeeeffff0000",
		),
		DealerShareID: 3,
		DealerParticipant: common.HexToAddress(
			"0x111122223333444455556666777788889999aAaA",
		),
		RecipientShareID: 7,
		RecipientParticipant: common.HexToAddress(
			"0x22223333444455556666777788889999AAAAbBbB",
		),
		CommitmentRoot: common.HexToHash(
			"0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		),
		RecipientTransportKeyRoot: common.HexToHash(
			"0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		),
		Ciphertext: ciphertext,
	}

	s1, s2, err :=
		rabbitVRFDKGEncryptedEvaluationSharedInfoV1(
			message,
		)
	if err != nil {
		t.Fatal(err)
	}

	slotID, err :=
		RabbitVRFDKGEncryptedEvaluationSlotIDV1(
			message,
		)
	if err != nil {
		t.Fatal(err)
	}

	messageID, err :=
		RabbitVRFDKGEncryptedEvaluationIDV1(
			message,
		)
	if err != nil {
		t.Fatal(err)
	}

	wantS1 :=
		common.HexToHash(
			"0x34df7bf1ef6075d8e8c6449c6961e6b850886bdb90918bba26e524c475d093f1",
		)

	wantS2 :=
		common.HexToHash(
			"0xae091fafba901fb3aeedeb492fece886e9908be3254ae7de204523eeb846b650",
		)

	wantSlot :=
		common.HexToHash(
			"0x03e875a0f8d5ccbafa3d8c29c412be93259d8d6979c1b1616dd9e962334f8210",
		)

	wantID :=
		common.HexToHash(
			"0x4a89ae79a11e1e3808eb76e16a0c6fc7c01bc925203be9dec8580aa9e619d43b",
		)

	if !bytes.Equal(
		s1,
		wantS1[:],
	) {
		t.Fatalf(
			"s1 vector mismatch: have=0x%x want=%s",
			s1,
			wantS1.Hex(),
		)
	}

	if !bytes.Equal(
		s2,
		wantS2[:],
	) {
		t.Fatalf(
			"s2 vector mismatch: have=0x%x want=%s",
			s2,
			wantS2.Hex(),
		)
	}

	if slotID != wantSlot {
		t.Fatalf(
			"SlotID vector mismatch: have=%s want=%s",
			slotID.Hex(),
			wantSlot.Hex(),
		)
	}

	if messageID != wantID {
		t.Fatalf(
			"MessageID vector mismatch: have=%s want=%s",
			messageID.Hex(),
			wantID.Hex(),
		)
	}
}
