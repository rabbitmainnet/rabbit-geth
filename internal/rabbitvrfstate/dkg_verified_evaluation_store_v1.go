package rabbitvrfstate

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	rabbitvrf "github.com/ethereum/go-ethereum/crypto/rabbitvrf"
	"github.com/ethereum/go-ethereum/rlp"
)

const DKGVerifiedEvaluationStoreVersionV1 uint8 = 1

var (
	ErrInvalidDKGVerifiedEvaluationStoreV1          = errors.New("invalid rabbit vrf dkg verified evaluation store v1")
	ErrDKGVerifiedEvaluationStoreConflictV1         = errors.New("rabbit vrf dkg verified evaluation conflict v1")
	ErrDKGVerifiedEvaluationStoreMetadataMismatchV1 = errors.New("rabbit vrf dkg verified evaluation metadata mismatch v1")
	ErrDKGVerifiedEvaluationStoreDecryptV1          = errors.New("rabbit vrf dkg verified evaluation decrypt failed v1")
)

var dkgVerifiedEvaluationStoreDomainV1 = []byte("RABBIT-VRF-DKG-VERIFIED-EVALUATION-STORE-V1")

type DKGVerifiedEvaluationStoreV1 struct {
	dir     string
	scryptN int
	scryptP int
	mu      sync.Mutex
}

type dkgVerifiedEvaluationStoreFileV1 struct {
	StoreVersion     uint8               `json:"storeVersion"`
	SessionID        common.Hash         `json:"sessionId"`
	RecipientShareID uint64              `json:"recipientShareId"`
	Recipient        common.Address      `json:"recipient"`
	DealerShareID    uint64              `json:"dealerShareId"`
	Dealer           common.Address      `json:"dealer"`
	Crypto           keystore.CryptoJSON `json:"crypto"`
}

type dkgVerifiedEvaluationStoreSecretV1 struct {
	Domain           []byte
	StoreVersion     uint8
	SessionID        common.Hash
	RecipientShareID uint64
	Recipient        common.Address
	DealerShareID    uint64
	Dealer           common.Address
	Evaluation       []byte
}

func NewDKGVerifiedEvaluationStoreV1(dir string, scryptN int, scryptP int) (*DKGVerifiedEvaluationStoreV1, error) {
	if dir == "" || scryptN <= 1 || scryptP <= 0 {
		return nil, ErrInvalidDKGVerifiedEvaluationStoreV1
	}
	absolute, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("%w: resolve directory: %v", ErrInvalidDKGVerifiedEvaluationStoreV1, err)
	}
	return &DKGVerifiedEvaluationStoreV1{dir: absolute, scryptN: scryptN, scryptP: scryptP}, nil
}

func NewStandardDKGVerifiedEvaluationStoreV1(dir string) (*DKGVerifiedEvaluationStoreV1, error) {
	return NewDKGVerifiedEvaluationStoreV1(dir, keystore.StandardScryptN, keystore.StandardScryptP)
}

func (store *DKGVerifiedEvaluationStoreV1) Path(context lqc.RabbitVRFDKGSessionContextV1, recipient lqc.RabbitVRFCommitteeMemberV1, dealer lqc.RabbitVRFCommitteeMemberV1) (string, error) {
	if store == nil || store.dir == "" || recipient.ShareID == 0 || dealer.ShareID == 0 || recipient.ShareID > context.CommitteeSize || dealer.ShareID > context.CommitteeSize || recipient.Participant == (common.Address{}) || dealer.Participant == (common.Address{}) || recipient.TicketHash == (common.Hash{}) || dealer.TicketHash == (common.Hash{}) {
		return "", ErrInvalidDKGVerifiedEvaluationStoreV1
	}
	if err := lqc.ValidateRabbitVRFDKGSessionContextV1(context); err != nil {
		return "", ErrInvalidDKGVerifiedEvaluationStoreV1
	}
	sessionID, err := lqc.RabbitVRFDKGSessionIDV1(context)
	if err != nil {
		return "", ErrInvalidDKGVerifiedEvaluationStoreV1
	}
	return filepath.Join(store.dir, fmt.Sprintf("evaluation-session-%x-recipient-%d-dealer-%d.json", sessionID[:], recipient.ShareID, dealer.ShareID)), nil
}

func (store *DKGVerifiedEvaluationStoreV1) Load(context lqc.RabbitVRFDKGSessionContextV1, recipient lqc.RabbitVRFCommitteeMemberV1, dealer lqc.RabbitVRFCommitteeMemberV1, password string) (rabbitvrf.DKGPolynomialEvaluationV1, error) {
	var zero rabbitvrf.DKGPolynomialEvaluationV1
	if store == nil || password == "" {
		return zero, ErrInvalidDKGVerifiedEvaluationStoreV1
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	path, err := store.Path(context, recipient, dealer)
	if err != nil {
		return zero, err
	}
	encoded, err := readPrivateFileV1(path)
	if err != nil {
		return zero, err
	}
	var record dkgVerifiedEvaluationStoreFileV1
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&record); err != nil {
		return zero, ErrInvalidDKGVerifiedEvaluationStoreV1
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return zero, ErrInvalidDKGVerifiedEvaluationStoreV1
	}
	sessionID, err := lqc.RabbitVRFDKGSessionIDV1(context)
	if err != nil {
		return zero, ErrInvalidDKGVerifiedEvaluationStoreV1
	}
	if record.StoreVersion != DKGVerifiedEvaluationStoreVersionV1 || record.SessionID != sessionID || record.RecipientShareID != recipient.ShareID || record.Recipient != recipient.Participant || record.DealerShareID != dealer.ShareID || record.Dealer != dealer.Participant {
		return zero, ErrDKGVerifiedEvaluationStoreMetadataMismatchV1
	}
	plaintext, err := keystore.DecryptDataV3(record.Crypto, password)
	if err != nil {
		return zero, ErrDKGVerifiedEvaluationStoreDecryptV1
	}
	defer zeroBytesV1(plaintext)
	var secret dkgVerifiedEvaluationStoreSecretV1
	if err := rlp.DecodeBytes(plaintext, &secret); err != nil {
		return zero, ErrInvalidDKGVerifiedEvaluationStoreV1
	}
	if !bytes.Equal(secret.Domain, dkgVerifiedEvaluationStoreDomainV1) || secret.StoreVersion != DKGVerifiedEvaluationStoreVersionV1 || secret.SessionID != sessionID || secret.RecipientShareID != recipient.ShareID || secret.Recipient != recipient.Participant || secret.DealerShareID != dealer.ShareID || secret.Dealer != dealer.Participant {
		zeroBytesV1(secret.Evaluation)
		return zero, ErrDKGVerifiedEvaluationStoreMetadataMismatchV1
	}
	defer zeroBytesV1(secret.Evaluation)
	evaluation, err := rabbitvrf.DKGPolynomialEvaluationV1FromBytes(secret.Evaluation)
	if err != nil {
		return zero, ErrInvalidDKGVerifiedEvaluationStoreV1
	}
	return evaluation, nil
}

func (store *DKGVerifiedEvaluationStoreV1) Store(context lqc.RabbitVRFDKGSessionContextV1, recipient lqc.RabbitVRFCommitteeMemberV1, dealer lqc.RabbitVRFCommitteeMemberV1, evaluation rabbitvrf.DKGPolynomialEvaluationV1, password string) error {
	if store == nil || password == "" {
		return ErrInvalidDKGVerifiedEvaluationStoreV1
	}
	if _, err := rabbitvrf.DKGPolynomialEvaluationV1FromBytes(evaluation[:]); err != nil {
		return ErrInvalidDKGVerifiedEvaluationStoreV1
	}
	path, err := store.Path(context, recipient, dealer)
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		existing, loadErr := store.Load(context, recipient, dealer, password)
		if loadErr != nil {
			return loadErr
		}
		if existing == evaluation {
			return nil
		}
		return ErrDKGVerifiedEvaluationStoreConflictV1
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	sessionID, err := lqc.RabbitVRFDKGSessionIDV1(context)
	if err != nil {
		return ErrInvalidDKGVerifiedEvaluationStoreV1
	}
	evaluationBytes := append([]byte(nil), evaluation[:]...)
	defer zeroBytesV1(evaluationBytes)
	secret := dkgVerifiedEvaluationStoreSecretV1{Domain: append([]byte(nil), dkgVerifiedEvaluationStoreDomainV1...), StoreVersion: DKGVerifiedEvaluationStoreVersionV1, SessionID: sessionID, RecipientShareID: recipient.ShareID, Recipient: recipient.Participant, DealerShareID: dealer.ShareID, Dealer: dealer.Participant, Evaluation: append([]byte(nil), evaluationBytes...)}
	defer zeroBytesV1(secret.Evaluation)
	plaintext, err := rlp.EncodeToBytes(secret)
	if err != nil {
		return fmt.Errorf("%w: encode secret: %v", ErrInvalidDKGVerifiedEvaluationStoreV1, err)
	}
	defer zeroBytesV1(plaintext)
	auth := []byte(password)
	defer zeroBytesV1(auth)
	encrypted, err := keystore.EncryptDataV3(plaintext, auth, store.scryptN, store.scryptP)
	if err != nil {
		return fmt.Errorf("%w: encrypt: %v", ErrInvalidDKGVerifiedEvaluationStoreV1, err)
	}
	record := dkgVerifiedEvaluationStoreFileV1{StoreVersion: DKGVerifiedEvaluationStoreVersionV1, SessionID: sessionID, RecipientShareID: recipient.ShareID, Recipient: recipient.Participant, DealerShareID: dealer.ShareID, Dealer: dealer.Participant, Crypto: encrypted}
	encoded, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("%w: encode file: %v", ErrInvalidDKGVerifiedEvaluationStoreV1, err)
	}
	encoded = append(encoded, byte(10))
	if err := atomicWritePrivateFileNoReplaceV1(path, encoded); err != nil {
		if _, statErr := os.Stat(path); statErr == nil {
			existing, loadErr := store.Load(context, recipient, dealer, password)
			if loadErr == nil && existing == evaluation {
				return nil
			}
			if loadErr == nil {
				return ErrDKGVerifiedEvaluationStoreConflictV1
			}
		}
		return err
	}
	return nil
}
