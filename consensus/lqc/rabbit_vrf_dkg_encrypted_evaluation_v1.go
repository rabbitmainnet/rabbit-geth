package lqc

import (
	"bytes"
	"crypto/ecdsa"
	cryptorand "crypto/rand"
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/ecies"
	rabbitvrf "github.com/ethereum/go-ethereum/crypto/rabbitvrf"
	"github.com/ethereum/go-ethereum/rlp"
)

const (
	RabbitVRFDKGEncryptedEvaluationVersionV1 uint8 = 1

	// geth ECIES on secp256k1 for a 32-byte plaintext:
	//
	//   65 bytes ephemeral uncompressed secp256k1 public key
	//   16 bytes AES-CTR IV
	//   32 bytes encrypted evaluation
	//   32 bytes HMAC-SHA-256
	//
	// Total: 145 bytes.
	RabbitVRFDKGEncryptedEvaluationCiphertextSizeV1 = 145
)

var (
	ErrInvalidRabbitVRFDKGEncryptedEvaluationV1 = errors.New(
		"invalid rabbit vrf dkg encrypted evaluation v1",
	)

	ErrRabbitVRFDKGEncryptedEvaluationSenderMismatchV1 = errors.New(
		"rabbit vrf dkg encrypted evaluation sender mismatch v1",
	)

	ErrRabbitVRFDKGEncryptedEvaluationRecipientMismatchV1 = errors.New(
		"rabbit vrf dkg encrypted evaluation recipient mismatch v1",
	)

	ErrRabbitVRFDKGEncryptedEvaluationTransportKeyMismatchV1 = errors.New(
		"rabbit vrf dkg encrypted evaluation transport key mismatch v1",
	)
)

var (
	rabbitVRFDKGEncryptedEvaluationKDFDomainV1 = []byte(
		"RABBIT-VRF-DKG-EVAL-ECIES-KDF-V1",
	)

	rabbitVRFDKGEncryptedEvaluationMACDomainV1 = []byte(
		"RABBIT-VRF-DKG-EVAL-ECIES-MAC-V1",
	)

	rabbitVRFDKGEncryptedEvaluationSlotDomainV1 = []byte(
		"RABBIT-VRF-DKG-EVAL-CIPHERTEXT-SLOT-V1",
	)

	rabbitVRFDKGEncryptedEvaluationIDDomainV1 = []byte(
		"RABBIT-VRF-DKG-EVAL-CIPHERTEXT-ID-V1",
	)

	rabbitVRFDKGEncryptedEvaluationSignDomainV1 = []byte(
		"RABBIT-VRF-DKG-EVAL-CIPHERTEXT-SIGN-V1",
	)
)

// RabbitVRFDKGEncryptedEvaluationV1 is one dealer's private polynomial
// evaluation encrypted specifically for one immutable DKG recipient.
//
// The ciphertext is intentionally NOT a public DKG envelope payload.
//
// ECIES encryption is randomized. Two valid encryptions of the same evaluation
// for the same recipient therefore have the same SlotID but normally different
// MessageIDs. That fact alone is NOT dealer equivocation.
type RabbitVRFDKGEncryptedEvaluationV1 struct {
	Version                   uint8
	SessionID                 common.Hash
	DealerShareID             uint64
	DealerParticipant         common.Address
	RecipientShareID          uint64
	RecipientParticipant      common.Address
	CommitmentRoot            common.Hash
	RecipientTransportKeyRoot common.Hash
	Ciphertext                []byte
	Signature                 []byte
}

type rabbitVRFDKGEncryptedEvaluationContextV1 struct {
	Domain                    []byte
	Version                   uint8
	SessionID                 common.Hash
	DealerShareID             uint64
	DealerParticipant         common.Address
	RecipientShareID          uint64
	RecipientParticipant      common.Address
	CommitmentRoot            common.Hash
	RecipientTransportKeyRoot common.Hash
}

type rabbitVRFDKGEncryptedEvaluationIDPayloadV1 struct {
	Domain         []byte
	SlotID         common.Hash
	CiphertextHash common.Hash
}

type rabbitVRFDKGEncryptedEvaluationSignPayloadV1 struct {
	Domain                    []byte
	Version                   uint8
	SessionID                 common.Hash
	DealerShareID             uint64
	DealerParticipant         common.Address
	RecipientShareID          uint64
	RecipientParticipant      common.Address
	CommitmentRoot            common.Hash
	RecipientTransportKeyRoot common.Hash
	CiphertextHash            common.Hash
}

func validateRabbitVRFDKGEncryptedEvaluationMetadataV1(
	message RabbitVRFDKGEncryptedEvaluationV1,
	requireCiphertext bool,
) error {
	if message.Version !=
		RabbitVRFDKGEncryptedEvaluationVersionV1 ||
		message.SessionID == (common.Hash{}) ||
		message.DealerShareID == 0 ||
		message.DealerParticipant == (common.Address{}) ||
		message.RecipientShareID == 0 ||
		message.RecipientParticipant == (common.Address{}) ||
		message.CommitmentRoot == (common.Hash{}) ||
		message.RecipientTransportKeyRoot == (common.Hash{}) {
		return ErrInvalidRabbitVRFDKGEncryptedEvaluationV1
	}

	if requireCiphertext &&
		len(message.Ciphertext) !=
			RabbitVRFDKGEncryptedEvaluationCiphertextSizeV1 {
		return ErrInvalidRabbitVRFDKGEncryptedEvaluationV1
	}

	return nil
}

func rabbitVRFDKGEncryptedEvaluationContextBytesV1(
	domain []byte,
	message RabbitVRFDKGEncryptedEvaluationV1,
) ([]byte, error) {
	if len(domain) == 0 {
		return nil, ErrInvalidRabbitVRFDKGEncryptedEvaluationV1
	}

	if err :=
		validateRabbitVRFDKGEncryptedEvaluationMetadataV1(
			message,
			false,
		); err != nil {
		return nil, err
	}

	encoded, err :=
		rlp.EncodeToBytes(
			rabbitVRFDKGEncryptedEvaluationContextV1{
				Domain:                    domain,
				Version:                   message.Version,
				SessionID:                 message.SessionID,
				DealerShareID:             message.DealerShareID,
				DealerParticipant:         message.DealerParticipant,
				RecipientShareID:          message.RecipientShareID,
				RecipientParticipant:      message.RecipientParticipant,
				CommitmentRoot:            message.CommitmentRoot,
				RecipientTransportKeyRoot: message.RecipientTransportKeyRoot,
			},
		)
	if err != nil {
		return nil, ErrInvalidRabbitVRFDKGEncryptedEvaluationV1
	}

	return crypto.Keccak256(encoded), nil
}

func rabbitVRFDKGEncryptedEvaluationSharedInfoV1(
	message RabbitVRFDKGEncryptedEvaluationV1,
) (
	[]byte,
	[]byte,
	error,
) {
	s1, err :=
		rabbitVRFDKGEncryptedEvaluationContextBytesV1(
			rabbitVRFDKGEncryptedEvaluationKDFDomainV1,
			message,
		)
	if err != nil {
		return nil, nil, err
	}

	s2, err :=
		rabbitVRFDKGEncryptedEvaluationContextBytesV1(
			rabbitVRFDKGEncryptedEvaluationMACDomainV1,
			message,
		)
	if err != nil {
		return nil, nil, err
	}

	if bytes.Equal(s1, s2) {
		return nil, nil, ErrInvalidRabbitVRFDKGEncryptedEvaluationV1
	}

	return s1, s2, nil
}

func validateRabbitVRFDKGEncryptedEvaluationArtifactsV1(
	context RabbitVRFDKGSessionContextV1,
	dealer RabbitVRFCommitteeMemberV1,
	commitment RabbitVRFDKGPolynomialCommitmentV1,
	recipient RabbitVRFCommitteeMemberV1,
	recipientBinding RabbitVRFDKGTransportKeyBindingV1,
	recipientBindingEnvelope RabbitVRFDKGEnvelopeV1,
) (
	common.Hash,
	common.Hash,
	common.Hash,
	*ecdsa.PublicKey,
	error,
) {
	if err :=
		ValidateRabbitVRFDKGSessionContextV1(
			context,
		); err != nil {
		return common.Hash{},
			common.Hash{},
			common.Hash{},
			nil,
			ErrInvalidRabbitVRFDKGEncryptedEvaluationV1
	}

	sessionID, err :=
		RabbitVRFDKGSessionIDV1(context)
	if err != nil {
		return common.Hash{},
			common.Hash{},
			common.Hash{},
			nil,
			ErrInvalidRabbitVRFDKGEncryptedEvaluationV1
	}

	if dealer.ShareID == 0 ||
		dealer.ShareID > context.CommitteeSize ||
		dealer.TicketHash == (common.Hash{}) ||
		dealer.Participant == (common.Address{}) ||
		dealer.ShareID != commitment.DealerShareID {
		return common.Hash{},
			common.Hash{},
			common.Hash{},
			nil,
			ErrRabbitVRFDKGEncryptedEvaluationSenderMismatchV1
	}

	if recipient.ShareID == 0 ||
		recipient.ShareID > context.CommitteeSize ||
		recipient.TicketHash == (common.Hash{}) ||
		recipient.Participant == (common.Address{}) {
		return common.Hash{},
			common.Hash{},
			common.Hash{},
			nil,
			ErrRabbitVRFDKGEncryptedEvaluationRecipientMismatchV1
	}

	commitmentRoot, err :=
		ValidateRabbitVRFDKGPolynomialCommitmentV1(
			context,
			commitment,
		)
	if err != nil {
		return common.Hash{},
			common.Hash{},
			common.Hash{},
			nil,
			ErrInvalidRabbitVRFDKGEncryptedEvaluationV1
	}

	if err :=
		VerifyRabbitVRFDKGTransportKeyEnvelopeV1(
			context,
			recipient,
			recipientBinding,
			recipientBindingEnvelope,
		); err != nil {
		return common.Hash{},
			common.Hash{},
			common.Hash{},
			nil,
			ErrRabbitVRFDKGEncryptedEvaluationTransportKeyMismatchV1
	}

	transportRoot, err :=
		VerifyRabbitVRFDKGTransportKeyBindingV1(
			context,
			recipient,
			recipientBinding,
		)
	if err != nil {
		return common.Hash{},
			common.Hash{},
			common.Hash{},
			nil,
			ErrRabbitVRFDKGEncryptedEvaluationTransportKeyMismatchV1
	}

	transportPublicKey, err :=
		parseRabbitVRFDKGTransportPublicKeyV1(
			recipientBinding.PublicKey,
		)
	if err != nil {
		return common.Hash{},
			common.Hash{},
			common.Hash{},
			nil,
			ErrRabbitVRFDKGEncryptedEvaluationTransportKeyMismatchV1
	}

	return sessionID,
		commitmentRoot,
		transportRoot,
		transportPublicKey,
		nil
}

func NewRabbitVRFDKGEncryptedEvaluationV1(
	context RabbitVRFDKGSessionContextV1,
	dealer RabbitVRFCommitteeMemberV1,
	commitment RabbitVRFDKGPolynomialCommitmentV1,
	recipient RabbitVRFCommitteeMemberV1,
	recipientBinding RabbitVRFDKGTransportKeyBindingV1,
	recipientBindingEnvelope RabbitVRFDKGEnvelopeV1,
	evaluation rabbitvrf.DKGPolynomialEvaluationV1,
) (
	RabbitVRFDKGEncryptedEvaluationV1,
	common.Hash,
	common.Hash,
	error,
) {
	var out RabbitVRFDKGEncryptedEvaluationV1

	sessionID,
		commitmentRoot,
		transportRoot,
		transportPublicKey,
		err :=
		validateRabbitVRFDKGEncryptedEvaluationArtifactsV1(
			context,
			dealer,
			commitment,
			recipient,
			recipientBinding,
			recipientBindingEnvelope,
		)
	if err != nil {
		return out,
			common.Hash{},
			common.Hash{},
			err
	}

	if err :=
		VerifyRabbitVRFDKGPrivateEvaluationV1(
			context,
			commitment,
			recipient.ShareID,
			evaluation,
		); err != nil {
		return out,
			common.Hash{},
			common.Hash{},
			ErrInvalidRabbitVRFDKGEncryptedEvaluationV1
	}

	out = RabbitVRFDKGEncryptedEvaluationV1{
		Version:                   RabbitVRFDKGEncryptedEvaluationVersionV1,
		SessionID:                 sessionID,
		DealerShareID:             dealer.ShareID,
		DealerParticipant:         dealer.Participant,
		RecipientShareID:          recipient.ShareID,
		RecipientParticipant:      recipient.Participant,
		CommitmentRoot:            commitmentRoot,
		RecipientTransportKeyRoot: transportRoot,
	}

	s1, s2, err :=
		rabbitVRFDKGEncryptedEvaluationSharedInfoV1(
			out,
		)
	if err != nil {
		return RabbitVRFDKGEncryptedEvaluationV1{},
			common.Hash{},
			common.Hash{},
			err
	}

	ciphertext, err :=
		ecies.Encrypt(
			cryptorand.Reader,
			ecies.ImportECDSAPublic(
				transportPublicKey,
			),
			evaluation[:],
			s1,
			s2,
		)
	if err != nil ||
		len(ciphertext) !=
			RabbitVRFDKGEncryptedEvaluationCiphertextSizeV1 {
		return RabbitVRFDKGEncryptedEvaluationV1{},
			common.Hash{},
			common.Hash{},
			ErrInvalidRabbitVRFDKGEncryptedEvaluationV1
	}

	out.Ciphertext =
		append([]byte(nil), ciphertext...)

	slotID, err :=
		RabbitVRFDKGEncryptedEvaluationSlotIDV1(
			out,
		)
	if err != nil {
		return RabbitVRFDKGEncryptedEvaluationV1{},
			common.Hash{},
			common.Hash{},
			err
	}

	messageID, err :=
		RabbitVRFDKGEncryptedEvaluationIDV1(
			out,
		)
	if err != nil {
		return RabbitVRFDKGEncryptedEvaluationV1{},
			common.Hash{},
			common.Hash{},
			err
	}

	return out,
		slotID,
		messageID,
		nil
}

func RabbitVRFDKGEncryptedEvaluationSlotIDV1(
	message RabbitVRFDKGEncryptedEvaluationV1,
) (
	common.Hash,
	error,
) {
	if err :=
		validateRabbitVRFDKGEncryptedEvaluationMetadataV1(
			message,
			true,
		); err != nil {
		return common.Hash{}, err
	}

	encoded, err :=
		rlp.EncodeToBytes(
			rabbitVRFDKGEncryptedEvaluationContextV1{
				Domain:                    rabbitVRFDKGEncryptedEvaluationSlotDomainV1,
				Version:                   message.Version,
				SessionID:                 message.SessionID,
				DealerShareID:             message.DealerShareID,
				DealerParticipant:         message.DealerParticipant,
				RecipientShareID:          message.RecipientShareID,
				RecipientParticipant:      message.RecipientParticipant,
				CommitmentRoot:            message.CommitmentRoot,
				RecipientTransportKeyRoot: message.RecipientTransportKeyRoot,
			},
		)
	if err != nil {
		return common.Hash{},
			ErrInvalidRabbitVRFDKGEncryptedEvaluationV1
	}

	slotID := crypto.Keccak256Hash(encoded)
	if slotID == (common.Hash{}) {
		return common.Hash{},
			ErrInvalidRabbitVRFDKGEncryptedEvaluationV1
	}

	return slotID, nil
}

func RabbitVRFDKGEncryptedEvaluationIDV1(
	message RabbitVRFDKGEncryptedEvaluationV1,
) (
	common.Hash,
	error,
) {
	slotID, err :=
		RabbitVRFDKGEncryptedEvaluationSlotIDV1(
			message,
		)
	if err != nil {
		return common.Hash{}, err
	}

	ciphertextHash :=
		crypto.Keccak256Hash(
			message.Ciphertext,
		)

	encoded, err :=
		rlp.EncodeToBytes(
			rabbitVRFDKGEncryptedEvaluationIDPayloadV1{
				Domain:         rabbitVRFDKGEncryptedEvaluationIDDomainV1,
				SlotID:         slotID,
				CiphertextHash: ciphertextHash,
			},
		)
	if err != nil {
		return common.Hash{},
			ErrInvalidRabbitVRFDKGEncryptedEvaluationV1
	}

	messageID := crypto.Keccak256Hash(encoded)
	if messageID == (common.Hash{}) {
		return common.Hash{},
			ErrInvalidRabbitVRFDKGEncryptedEvaluationV1
	}

	return messageID, nil
}

// RabbitVRFDKGEncryptedEvaluationSigningDataV1 returns the exact canonical
// bytes supplied to accounts.Wallet.SignData by the dealer Participant wallet.
//
// Signature bytes themselves are deliberately excluded.
func RabbitVRFDKGEncryptedEvaluationSigningDataV1(
	context RabbitVRFDKGSessionContextV1,
	message RabbitVRFDKGEncryptedEvaluationV1,
) (
	[]byte,
	error,
) {
	if err :=
		ValidateRabbitVRFDKGSessionContextV1(
			context,
		); err != nil {
		return nil,
			ErrInvalidRabbitVRFDKGEncryptedEvaluationV1
	}

	sessionID, err :=
		RabbitVRFDKGSessionIDV1(context)
	if err != nil ||
		message.SessionID != sessionID {
		return nil,
			ErrInvalidRabbitVRFDKGEncryptedEvaluationV1
	}

	if err :=
		validateRabbitVRFDKGEncryptedEvaluationMetadataV1(
			message,
			true,
		); err != nil {
		return nil, err
	}

	ciphertextHash :=
		crypto.Keccak256Hash(
			message.Ciphertext,
		)

	encoded, err :=
		rlp.EncodeToBytes(
			rabbitVRFDKGEncryptedEvaluationSignPayloadV1{
				Domain:                    rabbitVRFDKGEncryptedEvaluationSignDomainV1,
				Version:                   message.Version,
				SessionID:                 message.SessionID,
				DealerShareID:             message.DealerShareID,
				DealerParticipant:         message.DealerParticipant,
				RecipientShareID:          message.RecipientShareID,
				RecipientParticipant:      message.RecipientParticipant,
				CommitmentRoot:            message.CommitmentRoot,
				RecipientTransportKeyRoot: message.RecipientTransportKeyRoot,
				CiphertextHash:            ciphertextHash,
			},
		)
	if err != nil {
		return nil,
			ErrInvalidRabbitVRFDKGEncryptedEvaluationV1
	}

	return encoded, nil
}

// RabbitVRFDKGEncryptedEvaluationSigningHashV1 matches the Keccak256 performed
// by accounts.Wallet.SignData over the canonical signing data.
func RabbitVRFDKGEncryptedEvaluationSigningHashV1(
	context RabbitVRFDKGSessionContextV1,
	message RabbitVRFDKGEncryptedEvaluationV1,
) (
	common.Hash,
	error,
) {
	signingData, err :=
		RabbitVRFDKGEncryptedEvaluationSigningDataV1(
			context,
			message,
		)
	if err != nil {
		return common.Hash{}, err
	}

	return crypto.Keccak256Hash(
		signingData,
	), nil
}

// VerifyRabbitVRFDKGEncryptedEvaluationSignatureV1 proves that the canonical
// Participant wallet controlling the dealer ShareID authenticated this exact
// ciphertext and recipient binding.
//
// A P2P node key and a DKG transport key are never accepted as substitutes.
func VerifyRabbitVRFDKGEncryptedEvaluationSignatureV1(
	context RabbitVRFDKGSessionContextV1,
	expectedDealer RabbitVRFCommitteeMemberV1,
	message RabbitVRFDKGEncryptedEvaluationV1,
) error {
	if err :=
		ValidateRabbitVRFDKGSessionContextV1(
			context,
		); err != nil {
		return ErrInvalidRabbitVRFDKGEncryptedEvaluationV1
	}

	sessionID, err :=
		RabbitVRFDKGSessionIDV1(context)
	if err != nil ||
		message.SessionID != sessionID {
		return ErrInvalidRabbitVRFDKGEncryptedEvaluationV1
	}

	if err :=
		validateRabbitVRFDKGEncryptedEvaluationMetadataV1(
			message,
			true,
		); err != nil {
		return err
	}

	if expectedDealer.ShareID == 0 ||
		expectedDealer.ShareID >
			context.CommitteeSize ||
		expectedDealer.TicketHash ==
			(common.Hash{}) ||
		expectedDealer.Participant ==
			(common.Address{}) ||
		expectedDealer.ShareID !=
			message.DealerShareID ||
		expectedDealer.Participant !=
			message.DealerParticipant {
		return ErrRabbitVRFDKGEncryptedEvaluationSenderMismatchV1
	}

	if len(message.Signature) !=
		crypto.SignatureLength {
		return ErrInvalidRabbitVRFDKGEncryptedEvaluationV1
	}

	r := new(big.Int).SetBytes(
		message.Signature[:32],
	)

	sigS := new(big.Int).SetBytes(
		message.Signature[32:64],
	)

	if !crypto.ValidateSignatureValues(
		message.Signature[64],
		r,
		sigS,
		true,
	) {
		return ErrInvalidRabbitVRFDKGEncryptedEvaluationV1
	}

	signingHash, err :=
		RabbitVRFDKGEncryptedEvaluationSigningHashV1(
			context,
			message,
		)
	if err != nil {
		return err
	}

	publicKey, err :=
		crypto.SigToPub(
			signingHash[:],
			message.Signature,
		)
	if err != nil {
		return ErrInvalidRabbitVRFDKGEncryptedEvaluationV1
	}

	signer :=
		crypto.PubkeyToAddress(
			*publicKey,
		)

	if signer !=
		expectedDealer.Participant {
		return ErrInvalidRabbitVRFDKGEncryptedEvaluationV1
	}

	return nil
}

func DecryptRabbitVRFDKGEncryptedEvaluationV1(
	context RabbitVRFDKGSessionContextV1,
	dealer RabbitVRFCommitteeMemberV1,
	commitment RabbitVRFDKGPolynomialCommitmentV1,
	recipient RabbitVRFCommitteeMemberV1,
	recipientBinding RabbitVRFDKGTransportKeyBindingV1,
	recipientBindingEnvelope RabbitVRFDKGEnvelopeV1,
	recipientPrivateKey *ecdsa.PrivateKey,
	message RabbitVRFDKGEncryptedEvaluationV1,
) (
	rabbitvrf.DKGPolynomialEvaluationV1,
	error,
) {
	var zero rabbitvrf.DKGPolynomialEvaluationV1

	sessionID,
		commitmentRoot,
		transportRoot,
		_,
		err :=
		validateRabbitVRFDKGEncryptedEvaluationArtifactsV1(
			context,
			dealer,
			commitment,
			recipient,
			recipientBinding,
			recipientBindingEnvelope,
		)
	if err != nil {
		return zero, err
	}

	if err :=
		validateRabbitVRFDKGEncryptedEvaluationMetadataV1(
			message,
			true,
		); err != nil {
		return zero, err
	}

	if message.SessionID != sessionID ||
		message.Version !=
			RabbitVRFDKGEncryptedEvaluationVersionV1 ||
		message.CommitmentRoot !=
			commitmentRoot ||
		message.RecipientTransportKeyRoot !=
			transportRoot {
		return zero,
			ErrInvalidRabbitVRFDKGEncryptedEvaluationV1
	}

	if message.DealerShareID !=
		dealer.ShareID ||
		message.DealerParticipant !=
			dealer.Participant {
		return zero,
			ErrRabbitVRFDKGEncryptedEvaluationSenderMismatchV1
	}

	if err :=
		VerifyRabbitVRFDKGEncryptedEvaluationSignatureV1(
			context,
			dealer,
			message,
		); err != nil {
		return zero, err
	}

	if message.RecipientShareID !=
		recipient.ShareID ||
		message.RecipientParticipant !=
			recipient.Participant {
		return zero,
			ErrRabbitVRFDKGEncryptedEvaluationRecipientMismatchV1
	}

	if recipientPrivateKey == nil ||
		recipientPrivateKey.PublicKey.X == nil ||
		recipientPrivateKey.PublicKey.Y == nil {
		return zero,
			ErrRabbitVRFDKGEncryptedEvaluationTransportKeyMismatchV1
	}

	localPublicKey :=
		crypto.CompressPubkey(
			&recipientPrivateKey.PublicKey,
		)

	if !bytes.Equal(
		localPublicKey,
		recipientBinding.PublicKey[:],
	) {
		return zero,
			ErrRabbitVRFDKGEncryptedEvaluationTransportKeyMismatchV1
	}

	s1, s2, err :=
		rabbitVRFDKGEncryptedEvaluationSharedInfoV1(
			message,
		)
	if err != nil {
		return zero, err
	}

	plaintext, err :=
		ecies.ImportECDSA(
			recipientPrivateKey,
		).Decrypt(
			message.Ciphertext,
			s1,
			s2,
		)
	if err != nil ||
		len(plaintext) !=
			rabbitvrf.DKGPolynomialEvaluationSizeV1 {
		return zero,
			ErrInvalidRabbitVRFDKGEncryptedEvaluationV1
	}

	evaluation, err :=
		rabbitvrf.DKGPolynomialEvaluationV1FromBytes(
			plaintext,
		)
	if err != nil {
		return zero,
			ErrInvalidRabbitVRFDKGEncryptedEvaluationV1
	}

	if err :=
		VerifyRabbitVRFDKGPrivateEvaluationV1(
			context,
			commitment,
			recipient.ShareID,
			evaluation,
		); err != nil {
		return zero,
			ErrInvalidRabbitVRFDKGEncryptedEvaluationV1
	}

	return evaluation, nil
}
