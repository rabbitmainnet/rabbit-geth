package rabbitvrfstate

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sync"

	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	rabbitvrf "github.com/ethereum/go-ethereum/crypto/rabbitvrf"
	"github.com/ethereum/go-ethereum/rlp"
)

const DKGDealerPolynomialStoreVersionV1 uint8 = 1

var (
	ErrInvalidDKGDealerPolynomialStoreV1 = errors.New(
		"invalid rabbit vrf dkg dealer polynomial store v1",
	)
	ErrDKGDealerPolynomialStoreMetadataMismatchV1 = errors.New(
		"rabbit vrf dkg dealer polynomial store metadata mismatch v1",
	)
	ErrDKGDealerPolynomialStoreDecryptV1 = errors.New(
		"rabbit vrf dkg dealer polynomial store decrypt failed v1",
	)
)

var dkgDealerPolynomialStoreDomainV1 = []byte(
	"RABBIT-VRF-DKG-DEALER-POLYNOMIAL-STORE-V1",
)

type DKGDealerPolynomialStoreV1 struct {
	dir     string
	scryptN int
	scryptP int
	mu      sync.Mutex
}

type dkgDealerPolynomialStoreFileV1 struct {
	StoreVersion   uint8               `json:"storeVersion"`
	SessionID      common.Hash         `json:"sessionId"`
	ShareID        uint64              `json:"shareId"`
	Participant    common.Address      `json:"participant"`
	Threshold      uint64              `json:"threshold"`
	CommitmentRoot common.Hash         `json:"commitmentRoot"`
	Crypto         keystore.CryptoJSON `json:"crypto"`
}

type dkgDealerPolynomialStoreSecretV1 struct {
	Domain         []byte
	StoreVersion   uint8
	SessionID      common.Hash
	ShareID        uint64
	Participant    common.Address
	Threshold      uint64
	CommitmentRoot common.Hash
	Polynomial     []byte
}

func NewDKGDealerPolynomialStoreV1(
	dir string,
	scryptN int,
	scryptP int,
) (*DKGDealerPolynomialStoreV1, error) {
	if dir == "" || scryptN <= 1 || scryptP <= 0 {
		return nil, ErrInvalidDKGDealerPolynomialStoreV1
	}

	absolute, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf(
			"%w: resolve directory: %v",
			ErrInvalidDKGDealerPolynomialStoreV1,
			err,
		)
	}

	return &DKGDealerPolynomialStoreV1{
		dir:     absolute,
		scryptN: scryptN,
		scryptP: scryptP,
	}, nil
}

func NewStandardDKGDealerPolynomialStoreV1(
	dir string,
) (*DKGDealerPolynomialStoreV1, error) {
	return NewDKGDealerPolynomialStoreV1(
		dir,
		keystore.StandardScryptN,
		keystore.StandardScryptP,
	)
}

func (store *DKGDealerPolynomialStoreV1) Path(
	context lqc.RabbitVRFDKGSessionContextV1,
	member lqc.RabbitVRFCommitteeMemberV1,
) (string, error) {
	if store == nil ||
		store.dir == "" ||
		member.ShareID == 0 ||
		member.ShareID > context.CommitteeSize ||
		member.TicketHash == (common.Hash{}) ||
		member.Participant == (common.Address{}) {
		return "", ErrInvalidDKGDealerPolynomialStoreV1
	}

	if err := lqc.ValidateRabbitVRFDKGSessionContextV1(context); err != nil {
		return "", ErrInvalidDKGDealerPolynomialStoreV1
	}

	sessionID, err := lqc.RabbitVRFDKGSessionIDV1(context)
	if err != nil {
		return "", ErrInvalidDKGDealerPolynomialStoreV1
	}

	return filepath.Join(
		store.dir,
		fmt.Sprintf(
			"dealer-session-%x-share-%d.json",
			sessionID[:],
			member.ShareID,
		),
	), nil
}

func (store *DKGDealerPolynomialStoreV1) Create(
	context lqc.RabbitVRFDKGSessionContextV1,
	member lqc.RabbitVRFCommitteeMemberV1,
	password string,
) (
	*rabbitvrf.DKGDealerPolynomialV1,
	lqc.RabbitVRFDKGPolynomialCommitmentV1,
	common.Hash,
	error,
) {
	var empty lqc.RabbitVRFDKGPolynomialCommitmentV1

	if store == nil || password == "" {
		return nil, empty, common.Hash{},
			ErrInvalidDKGDealerPolynomialStoreV1
	}

	store.mu.Lock()
	defer store.mu.Unlock()

	path, err := store.Path(context, member)
	if err != nil {
		return nil, empty, common.Hash{}, err
	}

	if err := requireTransportKeyPathAbsentV1(path); err != nil {
		return nil, empty, common.Hash{}, err
	}

	polynomial, err :=
		rabbitvrf.GenerateDKGDealerPolynomialV1(
			context.Threshold,
		)
	if err != nil {
		return nil, empty, common.Hash{}, fmt.Errorf(
			"%w: generate polynomial: %v",
			ErrInvalidDKGDealerPolynomialStoreV1,
			err,
		)
	}

	commitment, root, err :=
		dkgDealerPolynomialCommitmentV1(
			context,
			member,
			polynomial,
		)
	if err != nil {
		polynomial.Destroy()
		return nil, empty, common.Hash{}, err
	}

	polynomialBytes, err := polynomial.Bytes()
	if err != nil {
		polynomial.Destroy()
		return nil, empty, common.Hash{}, err
	}
	defer zeroBytesV1(polynomialBytes)

	secret := dkgDealerPolynomialStoreSecretV1{
		Domain: append(
			[]byte(nil),
			dkgDealerPolynomialStoreDomainV1...,
		),
		StoreVersion:   DKGDealerPolynomialStoreVersionV1,
		SessionID:      commitment.SessionID,
		ShareID:        member.ShareID,
		Participant:    member.Participant,
		Threshold:      context.Threshold,
		CommitmentRoot: root,
		Polynomial: append(
			[]byte(nil),
			polynomialBytes...,
		),
	}
	defer zeroBytesV1(secret.Polynomial)

	plaintext, err := rlp.EncodeToBytes(secret)
	if err != nil {
		polynomial.Destroy()
		return nil, empty, common.Hash{}, fmt.Errorf(
			"%w: encode secret: %v",
			ErrInvalidDKGDealerPolynomialStoreV1,
			err,
		)
	}
	defer zeroBytesV1(plaintext)

	auth := []byte(password)
	defer zeroBytesV1(auth)

	encrypted, err := keystore.EncryptDataV3(
		plaintext,
		auth,
		store.scryptN,
		store.scryptP,
	)
	if err != nil {
		polynomial.Destroy()
		return nil, empty, common.Hash{}, fmt.Errorf(
			"%w: encrypt: %v",
			ErrInvalidDKGDealerPolynomialStoreV1,
			err,
		)
	}

	record := dkgDealerPolynomialStoreFileV1{
		StoreVersion:   DKGDealerPolynomialStoreVersionV1,
		SessionID:      commitment.SessionID,
		ShareID:        member.ShareID,
		Participant:    member.Participant,
		Threshold:      context.Threshold,
		CommitmentRoot: root,
		Crypto:         encrypted,
	}

	encoded, err := json.MarshalIndent(
		record,
		"",
		"  ",
	)
	if err != nil {
		polynomial.Destroy()
		return nil, empty, common.Hash{}, fmt.Errorf(
			"%w: encode file: %v",
			ErrInvalidDKGDealerPolynomialStoreV1,
			err,
		)
	}
	encoded = append(encoded, '\n')

	if err := atomicWritePrivateFileNoReplaceV1(
		path,
		encoded,
	); err != nil {
		polynomial.Destroy()
		return nil, empty, common.Hash{}, err
	}

	return polynomial, commitment, root, nil
}

func (store *DKGDealerPolynomialStoreV1) Load(
	context lqc.RabbitVRFDKGSessionContextV1,
	member lqc.RabbitVRFCommitteeMemberV1,
	password string,
) (
	*rabbitvrf.DKGDealerPolynomialV1,
	lqc.RabbitVRFDKGPolynomialCommitmentV1,
	common.Hash,
	error,
) {
	var empty lqc.RabbitVRFDKGPolynomialCommitmentV1

	if store == nil || password == "" {
		return nil, empty, common.Hash{},
			ErrInvalidDKGDealerPolynomialStoreV1
	}

	store.mu.Lock()
	defer store.mu.Unlock()

	path, err := store.Path(context, member)
	if err != nil {
		return nil, empty, common.Hash{}, err
	}

	encoded, err := readPrivateFileV1(path)
	if err != nil {
		return nil, empty, common.Hash{}, err
	}

	var record dkgDealerPolynomialStoreFileV1

	if err := decodeDKGDealerPolynomialStoreFileV1(
		encoded,
		&record,
	); err != nil {
		return nil, empty, common.Hash{}, err
	}

	expectedSessionID, err :=
		lqc.RabbitVRFDKGSessionIDV1(context)
	if err != nil {
		return nil, empty, common.Hash{},
			ErrInvalidDKGDealerPolynomialStoreV1
	}

	if record.StoreVersion !=
		DKGDealerPolynomialStoreVersionV1 ||
		record.SessionID != expectedSessionID ||
		record.ShareID != member.ShareID ||
		record.Participant != member.Participant ||
		record.Threshold != context.Threshold ||
		record.CommitmentRoot == (common.Hash{}) {
		return nil, empty, common.Hash{},
			ErrDKGDealerPolynomialStoreMetadataMismatchV1
	}

	if err := validateDKGDealerPolynomialKDFV1(
		store,
		record.Crypto,
	); err != nil {
		return nil, empty, common.Hash{}, err
	}

	plaintext, err := keystore.DecryptDataV3(
		record.Crypto,
		password,
	)
	if err != nil {
		return nil, empty, common.Hash{}, fmt.Errorf(
			"%w: %v",
			ErrDKGDealerPolynomialStoreDecryptV1,
			err,
		)
	}
	defer zeroBytesV1(plaintext)

	var secret dkgDealerPolynomialStoreSecretV1

	if err := rlp.DecodeBytes(
		plaintext,
		&secret,
	); err != nil {
		return nil, empty, common.Hash{}, fmt.Errorf(
			"%w: decode secret: %v",
			ErrDKGDealerPolynomialStoreDecryptV1,
			err,
		)
	}
	defer zeroBytesV1(secret.Polynomial)

	if !bytes.Equal(
		secret.Domain,
		dkgDealerPolynomialStoreDomainV1,
	) ||
		secret.StoreVersion != record.StoreVersion ||
		secret.SessionID != record.SessionID ||
		secret.ShareID != record.ShareID ||
		secret.Participant != record.Participant ||
		secret.Threshold != record.Threshold ||
		secret.CommitmentRoot != record.CommitmentRoot {
		return nil, empty, common.Hash{},
			ErrDKGDealerPolynomialStoreMetadataMismatchV1
	}

	polynomial, err :=
		rabbitvrf.DKGDealerPolynomialV1FromBytes(
			secret.Polynomial,
			secret.Threshold,
		)
	if err != nil {
		return nil, empty, common.Hash{},
			ErrDKGDealerPolynomialStoreDecryptV1
	}

	commitment, root, err :=
		dkgDealerPolynomialCommitmentV1(
			context,
			member,
			polynomial,
		)
	if err != nil {
		polynomial.Destroy()
		return nil, empty, common.Hash{}, err
	}

	if root != record.CommitmentRoot {
		polynomial.Destroy()
		return nil, empty, common.Hash{},
			ErrDKGDealerPolynomialStoreMetadataMismatchV1
	}

	return polynomial, commitment, root, nil
}

func dkgDealerPolynomialCommitmentV1(
	context lqc.RabbitVRFDKGSessionContextV1,
	member lqc.RabbitVRFCommitteeMemberV1,
	polynomial *rabbitvrf.DKGDealerPolynomialV1,
) (
	lqc.RabbitVRFDKGPolynomialCommitmentV1,
	common.Hash,
	error,
) {
	var empty lqc.RabbitVRFDKGPolynomialCommitmentV1

	if polynomial == nil ||
		polynomial.Threshold() != context.Threshold {
		return empty, common.Hash{},
			ErrInvalidDKGDealerPolynomialStoreV1
	}

	coefficients, err := polynomial.Commitments()
	if err != nil {
		return empty, common.Hash{}, err
	}

	commitment, root, err :=
		lqc.NewRabbitVRFDKGPolynomialCommitmentV1(
			context,
			member.ShareID,
			coefficients,
		)
	if err != nil {
		return empty, common.Hash{}, err
	}

	return commitment, root, nil
}

func decodeDKGDealerPolynomialStoreFileV1(
	encoded []byte,
	record *dkgDealerPolynomialStoreFileV1,
) error {
	if record == nil {
		return ErrInvalidDKGDealerPolynomialStoreV1
	}

	decoder := json.NewDecoder(
		bytes.NewReader(encoded),
	)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(record); err != nil {
		return fmt.Errorf(
			"%w: decode file: %v",
			ErrInvalidDKGDealerPolynomialStoreV1,
			err,
		)
	}

	var trailing interface{}

	if err := decoder.Decode(&trailing); err != io.EOF {
		return ErrInvalidDKGDealerPolynomialStoreV1
	}

	return nil
}

func validateDKGDealerPolynomialKDFV1(
	store *DKGDealerPolynomialStoreV1,
	value keystore.CryptoJSON,
) error {
	if store == nil ||
		value.Cipher != "aes-128-ctr" ||
		value.KDF != "scrypt" ||
		len(value.KDFParams) != 5 {
		return ErrInvalidDKGDealerPolynomialStoreV1
	}

	exactInt := func(value interface{}, expected int) bool {
		switch typed := value.(type) {
		case int:
			return typed == expected
		case float64:
			return typed == float64(expected)
		default:
			return false
		}
	}

	if !exactInt(value.KDFParams["n"], store.scryptN) ||
		!exactInt(value.KDFParams["r"], 8) ||
		!exactInt(value.KDFParams["p"], store.scryptP) ||
		!exactInt(value.KDFParams["dklen"], 32) {
		return ErrInvalidDKGDealerPolynomialStoreV1
	}

	exactHex := func(value string, size int) bool {
		decoded, err := hex.DecodeString(value)
		return err == nil && len(decoded) == size
	}

	salt, ok := value.KDFParams["salt"].(string)
	if !ok ||
		!exactHex(salt, 32) ||
		!exactHex(value.CipherParams.IV, 16) ||
		!exactHex(value.MAC, 32) {
		return ErrInvalidDKGDealerPolynomialStoreV1
	}

	ciphertext, err := hex.DecodeString(value.CipherText)
	if err != nil || len(ciphertext) == 0 {
		return ErrInvalidDKGDealerPolynomialStoreV1
	}

	return nil
}
