package rabbitvrfstate

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
)

const (
	DKGTransportKeyStoreVersionV1 uint8 = 1

	maxDKGTransportKeyStoreFileSizeV1 = 1 << 20
)

var (
	ErrInvalidDKGTransportKeyStoreV1 = errors.New(
		"invalid rabbit vrf dkg transport key store v1",
	)

	ErrDKGTransportKeyStoreAlreadyExistsV1 = errors.New(
		"rabbit vrf dkg transport key store already exists v1",
	)

	ErrDKGTransportKeyStoreMissingV1 = errors.New(
		"rabbit vrf dkg transport key store missing v1",
	)

	ErrDKGTransportKeyStoreMetadataMismatchV1 = errors.New(
		"rabbit vrf dkg transport key store metadata mismatch v1",
	)

	ErrDKGTransportKeyStoreDecryptV1 = errors.New(
		"rabbit vrf dkg transport key store decrypt failed v1",
	)
)

var dkgTransportKeyStoreDomainV1 = []byte(
	"RABBIT-VRF-DKG-TRANSPORT-KEY-STORE-V1",
)

type DKGTransportKeyStoreV1 struct {
	dir     string
	scryptN int
	scryptP int
	mu      sync.Mutex
}

type dkgTransportKeyStorePublicKeyV1 lqc.RabbitVRFDKGTransportPublicKeyV1

func (key dkgTransportKeyStorePublicKeyV1) MarshalText() ([]byte, error) {
	encoded := make([]byte, 2+hex.EncodedLen(len(key)))
	copy(encoded, "0x")
	hex.Encode(encoded[2:], key[:])
	return encoded, nil
}

func (key *dkgTransportKeyStorePublicKeyV1) UnmarshalText(text []byte) error {
	if len(text) != 68 || string(text[:2]) != "0x" {
		return ErrInvalidDKGTransportKeyStoreV1
	}

	decoded, err := hex.DecodeString(string(text[2:]))
	if err != nil {
		return ErrInvalidDKGTransportKeyStoreV1
	}

	publicKey, err :=
		lqc.RabbitVRFDKGTransportPublicKeyV1FromBytes(
			decoded,
		)
	if err != nil {
		return ErrInvalidDKGTransportKeyStoreV1
	}

	*key = dkgTransportKeyStorePublicKeyV1(publicKey)
	return nil
}

type dkgTransportKeyStoreFileV1 struct {
	StoreVersion   uint8                           `json:"storeVersion"`
	BindingVersion uint8                           `json:"bindingVersion"`
	SessionID      common.Hash                     `json:"sessionId"`
	ShareID        uint64                          `json:"shareId"`
	Participant    common.Address                  `json:"participant"`
	Scheme         uint8                           `json:"scheme"`
	PublicKey      dkgTransportKeyStorePublicKeyV1 `json:"publicKey"`
	Crypto         keystore.CryptoJSON             `json:"crypto"`
}

type dkgTransportKeySecretV1 struct {
	Domain         []byte
	StoreVersion   uint8
	BindingVersion uint8
	SessionID      common.Hash
	ShareID        uint64
	Participant    common.Address
	Scheme         uint8
	PublicKey      lqc.RabbitVRFDKGTransportPublicKeyV1
	PrivateKey     []byte
}

func NewDKGTransportKeyStoreV1(
	dir string,
	scryptN int,
	scryptP int,
) (*DKGTransportKeyStoreV1, error) {
	if dir == "" ||
		scryptN <= 1 ||
		scryptP <= 0 {
		return nil, ErrInvalidDKGTransportKeyStoreV1
	}

	absolute, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf(
			"%w: resolve directory: %v",
			ErrInvalidDKGTransportKeyStoreV1,
			err,
		)
	}

	return &DKGTransportKeyStoreV1{
		dir:     absolute,
		scryptN: scryptN,
		scryptP: scryptP,
	}, nil
}

func NewStandardDKGTransportKeyStoreV1(
	dir string,
) (*DKGTransportKeyStoreV1, error) {
	return NewDKGTransportKeyStoreV1(
		dir,
		keystore.StandardScryptN,
		keystore.StandardScryptP,
	)
}

func (store *DKGTransportKeyStoreV1) Path(
	context lqc.RabbitVRFDKGSessionContextV1,
	member lqc.RabbitVRFCommitteeMemberV1,
) (string, error) {
	if store == nil ||
		store.dir == "" ||
		member.ShareID == 0 ||
		member.ShareID > context.CommitteeSize ||
		member.TicketHash == (common.Hash{}) ||
		member.Participant == (common.Address{}) {
		return "", ErrInvalidDKGTransportKeyStoreV1
	}

	if err :=
		lqc.ValidateRabbitVRFDKGSessionContextV1(
			context,
		); err != nil {
		return "", ErrInvalidDKGTransportKeyStoreV1
	}

	sessionID, err :=
		lqc.RabbitVRFDKGSessionIDV1(
			context,
		)
	if err != nil {
		return "", ErrInvalidDKGTransportKeyStoreV1
	}

	return filepath.Join(
		store.dir,
		fmt.Sprintf(
			"session-%x-share-%d.json",
			sessionID[:],
			member.ShareID,
		),
	), nil
}

func (store *DKGTransportKeyStoreV1) Create(
	context lqc.RabbitVRFDKGSessionContextV1,
	member lqc.RabbitVRFCommitteeMemberV1,
	password string,
) (
	*ecdsa.PrivateKey,
	lqc.RabbitVRFDKGTransportKeyBindingV1,
	error,
) {
	var empty lqc.RabbitVRFDKGTransportKeyBindingV1

	if store == nil ||
		password == "" {
		return nil,
			empty,
			ErrInvalidDKGTransportKeyStoreV1
	}

	store.mu.Lock()
	defer store.mu.Unlock()

	path, err := store.Path(
		context,
		member,
	)
	if err != nil {
		return nil, empty, err
	}

	if err := requireTransportKeyPathAbsentV1(path); err != nil {
		return nil, empty, err
	}

	privateKey, err := crypto.GenerateKey()
	if err != nil {
		return nil,
			empty,
			fmt.Errorf(
				"%w: generate key: %v",
				ErrInvalidDKGTransportKeyStoreV1,
				err,
			)
	}

	binding, err :=
		store.saveLocked(
			path,
			context,
			member,
			privateKey,
			password,
		)
	if err != nil {
		if privateKey.D != nil {
			privateKey.D.SetInt64(0)
		}
		return nil, empty, err
	}

	return privateKey, binding, nil
}

func (store *DKGTransportKeyStoreV1) Save(
	context lqc.RabbitVRFDKGSessionContextV1,
	member lqc.RabbitVRFCommitteeMemberV1,
	privateKey *ecdsa.PrivateKey,
	password string,
) (
	lqc.RabbitVRFDKGTransportKeyBindingV1,
	error,
) {
	var empty lqc.RabbitVRFDKGTransportKeyBindingV1

	if store == nil ||
		privateKey == nil ||
		privateKey.D == nil ||
		password == "" {
		return empty, ErrInvalidDKGTransportKeyStoreV1
	}

	store.mu.Lock()
	defer store.mu.Unlock()

	path, err := store.Path(
		context,
		member,
	)
	if err != nil {
		return empty, err
	}

	if err := requireTransportKeyPathAbsentV1(path); err != nil {
		return empty, err
	}

	return store.saveLocked(
		path,
		context,
		member,
		privateKey,
		password,
	)
}

func (store *DKGTransportKeyStoreV1) saveLocked(
	path string,
	context lqc.RabbitVRFDKGSessionContextV1,
	member lqc.RabbitVRFCommitteeMemberV1,
	privateKey *ecdsa.PrivateKey,
	password string,
) (
	lqc.RabbitVRFDKGTransportKeyBindingV1,
	error,
) {
	var empty lqc.RabbitVRFDKGTransportKeyBindingV1

	secretBytes,
		canonicalPrivateKey,
		err :=
		canonicalDKGTransportPrivateKeyV1(
			privateKey,
		)
	if err != nil {
		return empty, err
	}
	defer zeroBytesV1(secretBytes)
	defer canonicalPrivateKey.D.SetInt64(0)

	publicKey, err :=
		lqc.RabbitVRFDKGTransportPublicKeyV1FromBytes(
			crypto.CompressPubkey(
				&canonicalPrivateKey.PublicKey,
			),
		)
	if err != nil {
		return empty,
			fmt.Errorf(
				"%w: public key: %v",
				ErrInvalidDKGTransportKeyStoreV1,
				err,
			)
	}

	binding, _, err :=
		lqc.NewRabbitVRFDKGTransportKeyBindingV1(
			context,
			member,
			publicKey,
		)
	if err != nil {
		return empty, err
	}

	secret := dkgTransportKeySecretV1{
		Domain: append(
			[]byte(nil),
			dkgTransportKeyStoreDomainV1...,
		),
		StoreVersion:   DKGTransportKeyStoreVersionV1,
		BindingVersion: binding.Version,
		SessionID:      binding.SessionID,
		ShareID:        binding.ShareID,
		Participant:    binding.Participant,
		Scheme:         binding.Scheme,
		PublicKey:      binding.PublicKey,
		PrivateKey: append(
			[]byte(nil),
			secretBytes...,
		),
	}
	defer zeroBytesV1(secret.PrivateKey)

	plaintext, err :=
		rlp.EncodeToBytes(
			secret,
		)
	if err != nil {
		return empty,
			fmt.Errorf(
				"%w: encode secret: %v",
				ErrInvalidDKGTransportKeyStoreV1,
				err,
			)
	}
	defer zeroBytesV1(plaintext)

	auth := []byte(password)
	defer zeroBytesV1(auth)

	encrypted, err :=
		keystore.EncryptDataV3(
			plaintext,
			auth,
			store.scryptN,
			store.scryptP,
		)
	if err != nil {
		return empty,
			fmt.Errorf(
				"%w: encrypt: %v",
				ErrInvalidDKGTransportKeyStoreV1,
				err,
			)
	}

	record := dkgTransportKeyStoreFileV1{
		StoreVersion:   DKGTransportKeyStoreVersionV1,
		BindingVersion: binding.Version,
		SessionID:      binding.SessionID,
		ShareID:        binding.ShareID,
		Participant:    binding.Participant,
		Scheme:         binding.Scheme,
		PublicKey:      dkgTransportKeyStorePublicKeyV1(binding.PublicKey),
		Crypto:         encrypted,
	}

	encoded, err :=
		json.MarshalIndent(
			record,
			"",
			"  ",
		)
	if err != nil {
		return empty,
			fmt.Errorf(
				"%w: encode file: %v",
				ErrInvalidDKGTransportKeyStoreV1,
				err,
			)
	}
	encoded = append(encoded, '\n')

	if err :=
		atomicWritePrivateFileNoReplaceV1(
			path,
			encoded,
		); err != nil {
		return empty, err
	}

	return binding, nil
}

func (store *DKGTransportKeyStoreV1) Load(
	context lqc.RabbitVRFDKGSessionContextV1,
	member lqc.RabbitVRFCommitteeMemberV1,
	password string,
) (
	*ecdsa.PrivateKey,
	lqc.RabbitVRFDKGTransportKeyBindingV1,
	error,
) {
	var empty lqc.RabbitVRFDKGTransportKeyBindingV1

	if store == nil ||
		password == "" {
		return nil,
			empty,
			ErrInvalidDKGTransportKeyStoreV1
	}

	store.mu.Lock()
	defer store.mu.Unlock()

	path, err := store.Path(
		context,
		member,
	)
	if err != nil {
		return nil, empty, err
	}

	encoded, err :=
		readPrivateFileV1(
			path,
		)
	if err != nil {
		return nil, empty, err
	}

	var record dkgTransportKeyStoreFileV1

	if err :=
		decodeTransportKeyStoreFileV1(
			encoded,
			&record,
		); err != nil {
		return nil, empty, err
	}

	expectedSessionID, err :=
		lqc.RabbitVRFDKGSessionIDV1(
			context,
		)
	if err != nil {
		return nil,
			empty,
			ErrInvalidDKGTransportKeyStoreV1
	}

	if record.StoreVersion !=
		DKGTransportKeyStoreVersionV1 ||
		record.BindingVersion !=
			lqc.RabbitVRFDKGTransportKeyVersionV1 ||
		record.SessionID != expectedSessionID ||
		record.ShareID != member.ShareID ||
		record.Participant != member.Participant ||
		record.Scheme !=
			lqc.RabbitVRFDKGTransportSchemeECIESSecp256k1AES128SHA256V1 {
		return nil,
			empty,
			ErrDKGTransportKeyStoreMetadataMismatchV1
	}

	binding := lqc.RabbitVRFDKGTransportKeyBindingV1{
		Version:     record.BindingVersion,
		SessionID:   record.SessionID,
		ShareID:     record.ShareID,
		Participant: record.Participant,
		Scheme:      record.Scheme,
		PublicKey:   lqc.RabbitVRFDKGTransportPublicKeyV1(record.PublicKey),
	}

	if _, err :=
		lqc.VerifyRabbitVRFDKGTransportKeyBindingV1(
			context,
			member,
			binding,
		); err != nil {
		return nil,
			empty,
			fmt.Errorf(
				"%w: binding: %v",
				ErrDKGTransportKeyStoreMetadataMismatchV1,
				err,
			)
	}

	if err := validateDKGTransportKeyKDFV1(store, record.Crypto); err != nil {
		return nil, empty, err
	}

	plaintext, err :=
		keystore.DecryptDataV3(
			record.Crypto,
			password,
		)
	if err != nil {
		return nil,
			empty,
			fmt.Errorf(
				"%w: %v",
				ErrDKGTransportKeyStoreDecryptV1,
				err,
			)
	}
	defer zeroBytesV1(plaintext)

	var secret dkgTransportKeySecretV1

	if err :=
		rlp.DecodeBytes(
			plaintext,
			&secret,
		); err != nil {
		return nil,
			empty,
			fmt.Errorf(
				"%w: decode secret: %v",
				ErrDKGTransportKeyStoreDecryptV1,
				err,
			)
	}
	defer zeroBytesV1(secret.PrivateKey)

	if !bytes.Equal(
		secret.Domain,
		dkgTransportKeyStoreDomainV1,
	) ||
		secret.StoreVersion !=
			record.StoreVersion ||
		secret.BindingVersion !=
			record.BindingVersion ||
		secret.SessionID !=
			record.SessionID ||
		secret.ShareID !=
			record.ShareID ||
		secret.Participant !=
			record.Participant ||
		secret.Scheme !=
			record.Scheme ||
		secret.PublicKey !=
			lqc.RabbitVRFDKGTransportPublicKeyV1(record.PublicKey) {
		return nil,
			empty,
			ErrDKGTransportKeyStoreMetadataMismatchV1
	}

	if len(secret.PrivateKey) != 32 {
		return nil,
			empty,
			ErrDKGTransportKeyStoreDecryptV1
	}

	privateKey, err :=
		crypto.ToECDSA(
			secret.PrivateKey,
		)
	if err != nil {
		return nil,
			empty,
			fmt.Errorf(
				"%w: private key: %v",
				ErrDKGTransportKeyStoreDecryptV1,
				err,
			)
	}

	publicKey, err :=
		lqc.RabbitVRFDKGTransportPublicKeyV1FromBytes(
			crypto.CompressPubkey(
				&privateKey.PublicKey,
			),
		)
	if err != nil ||
		publicKey != lqc.RabbitVRFDKGTransportPublicKeyV1(record.PublicKey) {
		if privateKey.D != nil {
			privateKey.D.SetInt64(0)
		}
		return nil,
			empty,
			ErrDKGTransportKeyStoreMetadataMismatchV1
	}

	reconstructed, _, err :=
		lqc.NewRabbitVRFDKGTransportKeyBindingV1(
			context,
			member,
			publicKey,
		)
	if err != nil ||
		reconstructed != binding {
		if privateKey.D != nil {
			privateKey.D.SetInt64(0)
		}
		return nil,
			empty,
			ErrDKGTransportKeyStoreMetadataMismatchV1
	}

	return privateKey,
		binding,
		nil
}

func requireTransportKeyPathAbsentV1(
	path string,
) error {
	_, err := os.Lstat(path)

	switch {
	case err == nil:
		return ErrDKGTransportKeyStoreAlreadyExistsV1

	case os.IsNotExist(err):
		return nil

	default:
		return fmt.Errorf(
			"%w: inspect destination: %v",
			ErrInvalidDKGTransportKeyStoreV1,
			err,
		)
	}
}

func atomicWritePrivateFileNoReplaceV1(
	path string,
	content []byte,
) error {
	dir := filepath.Dir(path)

	if err :=
		os.MkdirAll(
			dir,
			0o700,
		); err != nil {
		return fmt.Errorf(
			"%w: create directory: %v",
			ErrInvalidDKGTransportKeyStoreV1,
			err,
		)
	}

	dirInfo, err :=
		os.Lstat(
			dir,
		)
	if err != nil ||
		!dirInfo.IsDir() ||
		dirInfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf(
			"%w: invalid private directory",
			ErrInvalidDKGTransportKeyStoreV1,
		)
	}

	if err :=
		os.Chmod(
			dir,
			0o700,
		); err != nil {
		return fmt.Errorf(
			"%w: secure directory: %v",
			ErrInvalidDKGTransportKeyStoreV1,
			err,
		)
	}

	if err :=
		requireTransportKeyPathAbsentV1(
			path,
		); err != nil {
		return err
	}

	temp, err :=
		os.CreateTemp(
			dir,
			"."+filepath.Base(path)+".tmp-*",
		)
	if err != nil {
		return fmt.Errorf(
			"%w: create temporary file: %v",
			ErrInvalidDKGTransportKeyStoreV1,
			err,
		)
	}

	tempName := temp.Name()
	keepTemp := true

	defer func() {
		if keepTemp {
			temp.Close()
			os.Remove(tempName)
		}
	}()

	if err :=
		temp.Chmod(
			0o600,
		); err != nil {
		return fmt.Errorf(
			"%w: secure temporary file: %v",
			ErrInvalidDKGTransportKeyStoreV1,
			err,
		)
	}

	if _, err :=
		temp.Write(
			content,
		); err != nil {
		return fmt.Errorf(
			"%w: write temporary file: %v",
			ErrInvalidDKGTransportKeyStoreV1,
			err,
		)
	}

	if err := temp.Sync(); err != nil {
		return fmt.Errorf(
			"%w: sync temporary file: %v",
			ErrInvalidDKGTransportKeyStoreV1,
			err,
		)
	}

	if err := temp.Close(); err != nil {
		return fmt.Errorf(
			"%w: close temporary file: %v",
			ErrInvalidDKGTransportKeyStoreV1,
			err,
		)
	}

	if err :=
		syncDirectoryV1(
			dir,
		); err != nil {
		return fmt.Errorf(
			"%w: sync temporary directory entry: %v",
			ErrInvalidDKGTransportKeyStoreV1,
			err,
		)
	}

	if err :=
		requireTransportKeyPathAbsentV1(
			path,
		); err != nil {
		return err
	}

	// Link installs the already-fsynced inode under the final name without
	// replacement semantics. If another process wins this race, Link fails
	// instead of replacing the transport key that was already persisted.
	if err :=
		os.Link(
			tempName,
			path,
		); err != nil {
		if os.IsExist(err) {
			return ErrDKGTransportKeyStoreAlreadyExistsV1
		}

		return fmt.Errorf(
			"%w: install file without replacement: %v",
			ErrInvalidDKGTransportKeyStoreV1,
			err,
		)
	}

	if err :=
		syncDirectoryV1(
			dir,
		); err != nil {
		return fmt.Errorf(
			"%w: sync installed file entry: %v",
			ErrInvalidDKGTransportKeyStoreV1,
			err,
		)
	}

	if err :=
		os.Remove(
			tempName,
		); err != nil {
		return fmt.Errorf(
			"%w: remove temporary link: %v",
			ErrInvalidDKGTransportKeyStoreV1,
			err,
		)
	}

	keepTemp = false

	if err :=
		syncDirectoryV1(
			dir,
		); err != nil {
		return fmt.Errorf(
			"%w: sync final directory state: %v",
			ErrInvalidDKGTransportKeyStoreV1,
			err,
		)
	}

	return nil
}

func readPrivateFileV1(
	path string,
) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil,
				ErrDKGTransportKeyStoreMissingV1
		}

		return nil,
			fmt.Errorf(
				"%w: inspect file: %v",
				ErrInvalidDKGTransportKeyStoreV1,
				err,
			)
	}

	if !info.Mode().IsRegular() {
		return nil,
			ErrInvalidDKGTransportKeyStoreV1
	}

	if runtime.GOOS != "windows" &&
		info.Mode().Perm()&0o077 != 0 {
		return nil,
			fmt.Errorf(
				"%w: unsafe file permissions %04o",
				ErrInvalidDKGTransportKeyStoreV1,
				info.Mode().Perm(),
			)
	}

	file, err := os.Open(path)
	if err != nil {
		return nil,
			fmt.Errorf(
				"%w: open file: %v",
				ErrInvalidDKGTransportKeyStoreV1,
				err,
			)
	}
	defer file.Close()

	encoded, err :=
		io.ReadAll(
			io.LimitReader(
				file,
				maxDKGTransportKeyStoreFileSizeV1+1,
			),
		)
	if err != nil {
		return nil,
			fmt.Errorf(
				"%w: read file: %v",
				ErrInvalidDKGTransportKeyStoreV1,
				err,
			)
	}

	if len(encoded) == 0 ||
		len(encoded) >
			maxDKGTransportKeyStoreFileSizeV1 {
		return nil,
			ErrInvalidDKGTransportKeyStoreV1
	}

	return encoded, nil
}

func decodeTransportKeyStoreFileV1(
	encoded []byte,
	record *dkgTransportKeyStoreFileV1,
) error {
	if record == nil {
		return ErrInvalidDKGTransportKeyStoreV1
	}

	decoder :=
		json.NewDecoder(
			bytes.NewReader(
				encoded,
			),
		)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(record); err != nil {
		return fmt.Errorf(
			"%w: decode file: %v",
			ErrInvalidDKGTransportKeyStoreV1,
			err,
		)
	}

	var trailing interface{}

	if err := decoder.Decode(&trailing); err != io.EOF {
		return ErrInvalidDKGTransportKeyStoreV1
	}

	return nil
}

func validateDKGTransportKeyKDFV1(
	store *DKGTransportKeyStoreV1,
	value keystore.CryptoJSON,
) error {
	if store == nil ||
		value.Cipher != "aes-128-ctr" ||
		value.KDF != "scrypt" ||
		len(value.KDFParams) != 5 {
		return ErrInvalidDKGTransportKeyStoreV1
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
		return ErrInvalidDKGTransportKeyStoreV1
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
		return ErrInvalidDKGTransportKeyStoreV1
	}

	ciphertext, err := hex.DecodeString(value.CipherText)
	if err != nil || len(ciphertext) == 0 {
		return ErrInvalidDKGTransportKeyStoreV1
	}

	return nil
}

func canonicalDKGTransportPrivateKeyV1(
	privateKey *ecdsa.PrivateKey,
) (
	[]byte,
	*ecdsa.PrivateKey,
	error,
) {
	if privateKey == nil ||
		privateKey.D == nil ||
		privateKey.D.Sign() <= 0 ||
		privateKey.PublicKey.X == nil ||
		privateKey.PublicKey.Y == nil {
		return nil,
			nil,
			ErrInvalidDKGTransportKeyStoreV1
	}

	scalar :=
		privateKey.D.Bytes()
	defer zeroBytesV1(scalar)

	if len(scalar) == 0 ||
		len(scalar) > 32 {
		return nil,
			nil,
			ErrInvalidDKGTransportKeyStoreV1
	}

	encoded :=
		make(
			[]byte,
			32,
		)

	copy(
		encoded[32-len(scalar):],
		scalar,
	)

	canonical, err :=
		crypto.ToECDSA(
			encoded,
		)
	if err != nil {
		zeroBytesV1(encoded)

		return nil,
			nil,
			fmt.Errorf(
				"%w: private scalar: %v",
				ErrInvalidDKGTransportKeyStoreV1,
				err,
			)
	}

	if privateKey.PublicKey.X.Cmp(
		canonical.PublicKey.X,
	) != 0 ||
		privateKey.PublicKey.Y.Cmp(
			canonical.PublicKey.Y,
		) != 0 {
		zeroBytesV1(encoded)
		canonical.D.SetInt64(0)

		return nil,
			nil,
			ErrInvalidDKGTransportKeyStoreV1
	}

	return encoded,
		canonical,
		nil
}

func zeroBytesV1(
	value []byte,
) {
	for index := range value {
		value[index] = 0
	}
}
