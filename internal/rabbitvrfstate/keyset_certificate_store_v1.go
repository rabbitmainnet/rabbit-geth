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
)

const RabbitVRFKeysetCertificateStoreVersionV1 uint8 = 1

var (
	ErrInvalidRabbitVRFKeysetCertificateStoreV1 = errors.New("invalid rabbit vrf keyset certificate store v1")

	ErrRabbitVRFKeysetCertificateStoreConflictV1 = errors.New("rabbit vrf keyset certificate store conflict v1")
)

type RabbitVRFKeysetCertificateStoreV1 struct {
	dir string
}

type rabbitVRFKeysetCertificateStoreFileV1 struct {
	StoreVersion uint8                            `json:"storeVersion"`
	SessionID    common.Hash                      `json:"sessionId"`
	Certificate  lqc.RabbitVRFKeysetCertificateV1 `json:"certificate"`
}

func NewRabbitVRFKeysetCertificateStoreV1(
	dir string,
) (*RabbitVRFKeysetCertificateStoreV1, error) {
	if dir == "" {
		return nil, ErrInvalidRabbitVRFKeysetCertificateStoreV1
	}
	return &RabbitVRFKeysetCertificateStoreV1{dir: dir}, nil
}

func (store *RabbitVRFKeysetCertificateStoreV1) Path(
	context lqc.RabbitVRFDKGSessionContextV1,
) (string, error) {
	if store == nil {
		return "", ErrInvalidRabbitVRFKeysetCertificateStoreV1
	}

	sessionID, err := lqc.RabbitVRFDKGSessionIDV1(context)
	if err != nil || sessionID == (common.Hash{}) {
		return "", ErrInvalidRabbitVRFKeysetCertificateStoreV1
	}

	return filepath.Join(
		store.dir,
		fmt.Sprintf(
			"keyset-certificate-session-%x.json",
			sessionID[:],
		),
	), nil
}

func equalRabbitVRFKeysetCertificatesV1(
	a lqc.RabbitVRFKeysetCertificateV1,
	b lqc.RabbitVRFKeysetCertificateV1,
) bool {
	if a.Version != b.Version ||
		a.SessionID != b.SessionID ||
		a.KeysetRoot != b.KeysetRoot ||
		a.ThresholdPublicKey != b.ThresholdPublicKey ||
		a.TranscriptRoot != b.TranscriptRoot ||
		len(a.Signatures) != len(b.Signatures) {
		return false
	}

	for index := range a.Signatures {
		if !bytes.Equal(a.Signatures[index], b.Signatures[index]) {
			return false
		}
	}

	return true
}

func (store *RabbitVRFKeysetCertificateStoreV1) Load(
	context lqc.RabbitVRFDKGSessionContextV1,
	members []lqc.RabbitVRFCommitteeMemberV1,
) (lqc.RabbitVRFKeysetCertificateV1, error) {
	var zero lqc.RabbitVRFKeysetCertificateV1

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

	var record rabbitVRFKeysetCertificateStoreFileV1
	if err := decoder.Decode(&record); err != nil {
		return zero, ErrInvalidRabbitVRFKeysetCertificateStoreV1
	}

	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return zero, ErrInvalidRabbitVRFKeysetCertificateStoreV1
	}

	sessionID, err := lqc.RabbitVRFDKGSessionIDV1(context)
	if err != nil ||
		record.StoreVersion != RabbitVRFKeysetCertificateStoreVersionV1 ||
		record.SessionID != sessionID {
		return zero, ErrInvalidRabbitVRFKeysetCertificateStoreV1
	}

	validated, err := lqc.ValidateRabbitVRFKeysetCertificateV1(
		context,
		members,
		record.Certificate,
	)
	if err != nil {
		return zero, ErrInvalidRabbitVRFKeysetCertificateStoreV1
	}

	return validated, nil
}

func (store *RabbitVRFKeysetCertificateStoreV1) Store(
	context lqc.RabbitVRFDKGSessionContextV1,
	members []lqc.RabbitVRFCommitteeMemberV1,
	certificate lqc.RabbitVRFKeysetCertificateV1,
) error {
	validated, err := lqc.ValidateRabbitVRFKeysetCertificateV1(
		context,
		members,
		certificate,
	)
	if err != nil {
		return ErrInvalidRabbitVRFKeysetCertificateStoreV1
	}

	path, err := store.Path(context)
	if err != nil {
		return err
	}

	if _, err := os.Stat(path); err == nil {
		existing, loadErr := store.Load(context, members)
		if loadErr != nil {
			return loadErr
		}

		if equalRabbitVRFKeysetCertificatesV1(
			existing,
			validated,
		) {
			return nil
		}

		return ErrRabbitVRFKeysetCertificateStoreConflictV1
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	sessionID, err := lqc.RabbitVRFDKGSessionIDV1(context)
	if err != nil {
		return ErrInvalidRabbitVRFKeysetCertificateStoreV1
	}

	record := rabbitVRFKeysetCertificateStoreFileV1{
		StoreVersion: RabbitVRFKeysetCertificateStoreVersionV1,
		SessionID:    sessionID,
		Certificate:  validated,
	}

	encoded, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	encoded = append(encoded, byte(10))

	if err := atomicWritePrivateFileNoReplaceV1(
		path,
		encoded,
	); err != nil {
		if _, statErr := os.Stat(path); statErr == nil {
			existing, loadErr := store.Load(context, members)

			if loadErr == nil &&
				equalRabbitVRFKeysetCertificatesV1(
					existing,
					validated,
				) {
				return nil
			}

			if loadErr == nil {
				return ErrRabbitVRFKeysetCertificateStoreConflictV1
			}
		}

		return err
	}

	return nil
}
