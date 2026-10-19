package rabbitvrfstate

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	rabbitvrf "github.com/ethereum/go-ethereum/crypto/rabbitvrf"
	"github.com/ethereum/go-ethereum/rlp"
)

const DKGSecretShareStoreVersionV1 uint8 = 1

var (
	ErrInvalidDKGSecretShareStoreV1          = errors.New("invalid rabbit vrf dkg secret share store v1")
	ErrDKGSecretShareStoreConflictV1         = errors.New("rabbit vrf dkg secret share store conflict v1")
	ErrDKGSecretShareStoreMetadataMismatchV1 = errors.New("rabbit vrf dkg secret share store metadata mismatch v1")
	ErrDKGSecretShareStoreDecryptV1          = errors.New("rabbit vrf dkg secret share store decrypt v1")
)

var dkgSecretShareStoreDomainV1 = []byte("RABBIT-VRF-DKG-SECRET-SHARE-STORE-V1")

type DKGSecretShareStoreV1 struct {
	dir     string
	scryptN int
	scryptP int
}

type dkgSecretShareStoreFileV1 struct {
	StoreVersion uint8               `json:"storeVersion"`
	SessionID    common.Hash         `json:"sessionId"`
	ShareID      uint64              `json:"shareId"`
	Participant  common.Address      `json:"participant"`
	Crypto       keystore.CryptoJSON `json:"crypto"`
}

type dkgSecretShareStoreSecretV1 struct {
	Domain       []byte
	StoreVersion uint8
	SessionID    common.Hash
	ShareID      uint64
	Participant  common.Address
	SecretShare  []byte
}

func NewDKGSecretShareStoreV1(dir string, scryptN, scryptP int) (*DKGSecretShareStoreV1, error) {
	if dir == "" || scryptN <= 1 || scryptP <= 0 {
		return nil, ErrInvalidDKGSecretShareStoreV1
	}
	return &DKGSecretShareStoreV1{dir: dir, scryptN: scryptN, scryptP: scryptP}, nil
}

func NewStandardDKGSecretShareStoreV1(dir string) (*DKGSecretShareStoreV1, error) {
	return NewDKGSecretShareStoreV1(dir, keystore.StandardScryptN, keystore.StandardScryptP)
}

func (store *DKGSecretShareStoreV1) Path(context lqc.RabbitVRFDKGSessionContextV1, member lqc.RabbitVRFCommitteeMemberV1) (string, error) {
	if store == nil || member.ShareID == 0 || member.Participant == (common.Address{}) {
		return "", ErrInvalidDKGSecretShareStoreV1
	}
	sessionID, err := lqc.RabbitVRFDKGSessionIDV1(context)
	if err != nil || sessionID == (common.Hash{}) {
		return "", ErrInvalidDKGSecretShareStoreV1
	}
	return filepath.Join(store.dir, fmt.Sprintf("secret-share-session-%x-share-%d.json", sessionID[:], member.ShareID)), nil
}

func (store *DKGSecretShareStoreV1) Load(context lqc.RabbitVRFDKGSessionContextV1, member lqc.RabbitVRFCommitteeMemberV1, password string) (*rabbitvrf.SecretShare, error) {
	if store == nil || password == "" {
		return nil, ErrInvalidDKGSecretShareStoreV1
	}
	path, err := store.Path(context, member)
	if err != nil {
		return nil, err
	}
	encoded, err := readPrivateFileV1(path)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var record dkgSecretShareStoreFileV1
	if err := decoder.Decode(&record); err != nil {
		return nil, ErrInvalidDKGSecretShareStoreV1
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, ErrInvalidDKGSecretShareStoreV1
	}
	sessionID, err := lqc.RabbitVRFDKGSessionIDV1(context)
	if err != nil {
		return nil, ErrInvalidDKGSecretShareStoreV1
	}
	if record.StoreVersion != DKGSecretShareStoreVersionV1 || record.SessionID != sessionID || record.ShareID != member.ShareID || record.Participant != member.Participant {
		return nil, ErrDKGSecretShareStoreMetadataMismatchV1
	}
	plaintext, err := keystore.DecryptDataV3(record.Crypto, password)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDKGSecretShareStoreDecryptV1, err)
	}
	defer zeroBytesV1(plaintext)
	var secret dkgSecretShareStoreSecretV1
	if err := rlp.DecodeBytes(plaintext, &secret); err != nil {
		return nil, ErrInvalidDKGSecretShareStoreV1
	}
	defer zeroBytesV1(secret.SecretShare)
	if !bytes.Equal(secret.Domain, dkgSecretShareStoreDomainV1) || secret.StoreVersion != DKGSecretShareStoreVersionV1 || secret.SessionID != sessionID || secret.ShareID != member.ShareID || secret.Participant != member.Participant {
		return nil, ErrDKGSecretShareStoreMetadataMismatchV1
	}
	return rabbitvrf.SecretShareFromBytes(member.ShareID, secret.SecretShare)
}

func (store *DKGSecretShareStoreV1) Store(context lqc.RabbitVRFDKGSessionContextV1, member lqc.RabbitVRFCommitteeMemberV1, share *rabbitvrf.SecretShare, password string) error {
	if store == nil || share == nil || password == "" || share.ID() != member.ShareID {
		return ErrInvalidDKGSecretShareStoreV1
	}
	raw, err := share.Bytes()
	if err != nil {
		return ErrInvalidDKGSecretShareStoreV1
	}
	secretBytes := append([]byte(nil), raw[:]...)
	defer zeroBytesV1(secretBytes)
	path, err := store.Path(context, member)
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		existing, loadErr := store.Load(context, member, password)
		if loadErr != nil {
			return loadErr
		}
		existingBytes, loadErr := existing.Bytes()
		if loadErr != nil {
			return loadErr
		}
		if bytes.Equal(existingBytes[:], raw[:]) {
			return nil
		}
		return ErrDKGSecretShareStoreConflictV1
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	sessionID, err := lqc.RabbitVRFDKGSessionIDV1(context)
	if err != nil {
		return ErrInvalidDKGSecretShareStoreV1
	}
	secret := dkgSecretShareStoreSecretV1{
		Domain:       append([]byte(nil), dkgSecretShareStoreDomainV1...),
		StoreVersion: DKGSecretShareStoreVersionV1,
		SessionID:    sessionID,
		ShareID:      member.ShareID,
		Participant:  member.Participant,
		SecretShare:  append([]byte(nil), secretBytes...),
	}
	defer zeroBytesV1(secret.SecretShare)
	plaintext, err := rlp.EncodeToBytes(secret)
	if err != nil {
		return err
	}
	defer zeroBytesV1(plaintext)
	auth := []byte(password)
	defer zeroBytesV1(auth)
	encrypted, err := keystore.EncryptDataV3(plaintext, auth, store.scryptN, store.scryptP)
	if err != nil {
		return err
	}
	record := dkgSecretShareStoreFileV1{
		StoreVersion: DKGSecretShareStoreVersionV1,
		SessionID:    sessionID,
		ShareID:      member.ShareID,
		Participant:  member.Participant,
		Crypto:       encrypted,
	}
	encoded, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	encoded = append(encoded, byte(10))
	if err := atomicWritePrivateFileNoReplaceV1(path, encoded); err != nil {
		if _, statErr := os.Stat(path); statErr == nil {
			existing, loadErr := store.Load(context, member, password)
			if loadErr == nil {
				existingBytes, bytesErr := existing.Bytes()
				if bytesErr == nil && bytes.Equal(existingBytes[:], raw[:]) {
					return nil
				}
				if bytesErr == nil {
					return ErrDKGSecretShareStoreConflictV1
				}
			}
		}
		return err
	}
	return nil
}
