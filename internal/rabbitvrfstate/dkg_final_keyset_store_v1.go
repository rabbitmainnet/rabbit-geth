package rabbitvrfstate

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	rabbitvrf "github.com/ethereum/go-ethereum/crypto/rabbitvrf"
)

const DKGFinalKeysetStoreVersionV1 uint8 = 1

var (
	ErrInvalidDKGFinalKeysetStoreV1  = errors.New("invalid rabbit vrf dkg final keyset store v1")
	ErrDKGFinalKeysetStoreConflictV1 = errors.New("rabbit vrf dkg final keyset store conflict v1")
)

type DKGFinalKeysetV1 struct {
	KeysetRoot         common.Hash
	ThresholdPublicKey rabbitvrf.PublicKey
	TranscriptRoot     common.Hash
	VerificationShares []lqc.RabbitVRFVerificationShareV1
}

type DKGFinalKeysetStoreV1 struct {
	dir string
}

type dkgFinalKeysetStoreFileV1 struct {
	StoreVersion       uint8                              `json:"storeVersion"`
	SessionID          common.Hash                        `json:"sessionId"`
	KeysetRoot         common.Hash                        `json:"keysetRoot"`
	ThresholdPublicKey rabbitvrf.PublicKey                `json:"thresholdPublicKey"`
	TranscriptRoot     common.Hash                        `json:"transcriptRoot"`
	VerificationShares []lqc.RabbitVRFVerificationShareV1 `json:"verificationShares"`
}

func NewDKGFinalKeysetStoreV1(dir string) (*DKGFinalKeysetStoreV1, error) {
	if dir == "" {
		return nil, ErrInvalidDKGFinalKeysetStoreV1
	}
	return &DKGFinalKeysetStoreV1{dir: dir}, nil
}

func (store *DKGFinalKeysetStoreV1) Path(context lqc.RabbitVRFDKGSessionContextV1) (string, error) {
	if store == nil {
		return "", ErrInvalidDKGFinalKeysetStoreV1
	}
	sessionID, err := lqc.RabbitVRFDKGSessionIDV1(context)
	if err != nil || sessionID == (common.Hash{}) {
		return "", ErrInvalidDKGFinalKeysetStoreV1
	}
	return filepath.Join(store.dir, fmt.Sprintf("final-keyset-session-%x.json", sessionID[:])), nil
}

func equalDKGVerificationSharesV1(a, b []lqc.RabbitVRFVerificationShareV1) bool {
	if len(a) != len(b) {
		return false
	}
	for index := range a {
		if a[index] != b[index] {
			return false
		}
	}
	return true
}

func validateDKGFinalKeysetV1(context lqc.RabbitVRFDKGSessionContextV1, value DKGFinalKeysetV1) (DKGFinalKeysetV1, error) {
	if value.KeysetRoot == (common.Hash{}) || value.TranscriptRoot == (common.Hash{}) {
		return DKGFinalKeysetV1{}, ErrInvalidDKGFinalKeysetStoreV1
	}
	root, canonical, err := lqc.RabbitVRFKeysetRootV1(
		context.ChainID,
		context.TargetVRFEpoch,
		context.CommitteeRoot,
		context.CommitteeSize,
		context.Threshold,
		value.ThresholdPublicKey,
		value.TranscriptRoot,
		value.VerificationShares,
	)
	if err != nil || root != value.KeysetRoot {
		return DKGFinalKeysetV1{}, ErrInvalidDKGFinalKeysetStoreV1
	}
	value.VerificationShares = canonical
	return value, nil
}

func (store *DKGFinalKeysetStoreV1) Load(context lqc.RabbitVRFDKGSessionContextV1) (DKGFinalKeysetV1, error) {
	var zero DKGFinalKeysetV1
	path, err := store.Path(context)
	if err != nil {
		return zero, err
	}
	encoded, err := readPrivateFileV1(path)
	if err != nil {
		return zero, err
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var record dkgFinalKeysetStoreFileV1
	if err := decoder.Decode(&record); err != nil {
		return zero, ErrInvalidDKGFinalKeysetStoreV1
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return zero, ErrInvalidDKGFinalKeysetStoreV1
	}
	sessionID, err := lqc.RabbitVRFDKGSessionIDV1(context)
	if err != nil || record.StoreVersion != DKGFinalKeysetStoreVersionV1 || record.SessionID != sessionID {
		return zero, ErrInvalidDKGFinalKeysetStoreV1
	}
	return validateDKGFinalKeysetV1(context, DKGFinalKeysetV1{
		KeysetRoot:         record.KeysetRoot,
		ThresholdPublicKey: record.ThresholdPublicKey,
		TranscriptRoot:     record.TranscriptRoot,
		VerificationShares: append([]lqc.RabbitVRFVerificationShareV1(nil), record.VerificationShares...),
	})
}

func (store *DKGFinalKeysetStoreV1) Store(context lqc.RabbitVRFDKGSessionContextV1, value DKGFinalKeysetV1) error {
	validated, err := validateDKGFinalKeysetV1(context, value)
	if err != nil {
		return err
	}
	path, err := store.Path(context)
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		existing, loadErr := store.Load(context)
		if loadErr != nil {
			return loadErr
		}
		if existing.KeysetRoot == validated.KeysetRoot &&
			existing.ThresholdPublicKey == validated.ThresholdPublicKey &&
			existing.TranscriptRoot == validated.TranscriptRoot &&
			equalDKGVerificationSharesV1(existing.VerificationShares, validated.VerificationShares) {
			return nil
		}
		return ErrDKGFinalKeysetStoreConflictV1
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	sessionID, err := lqc.RabbitVRFDKGSessionIDV1(context)
	if err != nil {
		return ErrInvalidDKGFinalKeysetStoreV1
	}
	record := dkgFinalKeysetStoreFileV1{
		StoreVersion:       DKGFinalKeysetStoreVersionV1,
		SessionID:          sessionID,
		KeysetRoot:         validated.KeysetRoot,
		ThresholdPublicKey: validated.ThresholdPublicKey,
		TranscriptRoot:     validated.TranscriptRoot,
		VerificationShares: append([]lqc.RabbitVRFVerificationShareV1(nil), validated.VerificationShares...),
	}
	encoded, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	encoded = append(encoded, byte(10))
	if err := atomicWritePrivateFileNoReplaceV1(path, encoded); err != nil {
		if _, statErr := os.Stat(path); statErr == nil {
			existing, loadErr := store.Load(context)
			if loadErr == nil &&
				existing.KeysetRoot == validated.KeysetRoot &&
				existing.ThresholdPublicKey == validated.ThresholdPublicKey &&
				existing.TranscriptRoot == validated.TranscriptRoot &&
				equalDKGVerificationSharesV1(existing.VerificationShares, validated.VerificationShares) {
				return nil
			}
			if loadErr == nil {
				return ErrDKGFinalKeysetStoreConflictV1
			}
		}
		return err
	}
	return nil
}
