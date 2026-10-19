package rabbitvrfstate

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	"github.com/ethereum/go-ethereum/crypto"
)

const ThresholdPartialStoreVersionV1 uint8 = 1

var (
	ErrInvalidThresholdPartialStoreV1  = errors.New("invalid rabbit vrf threshold partial store v1")
	ErrThresholdPartialStoreConflictV1 = errors.New("rabbit vrf threshold partial store conflict v1")
)

type ThresholdPartialStoreV1 struct {
	dir string
}

type thresholdPartialStoreFileV1 struct {
	StoreVersion uint8                           `json:"storeVersion"`
	SessionID    common.Hash                     `json:"sessionId"`
	KeysetRoot   common.Hash                     `json:"keysetRoot"`
	RequestID    common.Hash                     `json:"requestId"`
	ShareID      uint64                          `json:"shareId"`
	Packet       lqc.RabbitVRFThresholdPartialV1 `json:"packet"`
}

func NewThresholdPartialStoreV1(dir string) (*ThresholdPartialStoreV1, error) {
	if dir == "" {
		return nil, ErrInvalidThresholdPartialStoreV1
	}
	return &ThresholdPartialStoreV1{dir: dir}, nil
}

func validateThresholdPartialV1(
	context lqc.RabbitVRFDKGSessionContextV1,
	packet lqc.RabbitVRFThresholdPartialV1,
) (lqc.RabbitVRFThresholdPartialV1, error) {
	sessionID, err := lqc.RabbitVRFDKGSessionIDV1(context)
	if err != nil ||
		sessionID == (common.Hash{}) ||
		packet.SessionID != sessionID ||
		packet.KeysetRoot == (common.Hash{}) ||
		packet.RequestID == (common.Hash{}) ||
		packet.MessageHash == (common.Hash{}) ||
		packet.ShareID == 0 ||
		packet.ShareID > context.CommitteeSize {
		return lqc.RabbitVRFThresholdPartialV1{}, ErrInvalidThresholdPartialStoreV1
	}

	_, messageHash, err := lqc.RabbitVRFThresholdMessageV1(
		context,
		packet.KeysetRoot,
		packet.RequestID,
	)
	if err != nil || messageHash != packet.MessageHash {
		return lqc.RabbitVRFThresholdPartialV1{}, ErrInvalidThresholdPartialStoreV1
	}

	if _, err := lqc.RabbitVRFThresholdPartialMessageIDV1(packet); err != nil {
		return lqc.RabbitVRFThresholdPartialV1{}, ErrInvalidThresholdPartialStoreV1
	}

	return packet, nil
}

func (store *ThresholdPartialStoreV1) Path(
	context lqc.RabbitVRFDKGSessionContextV1,
	keysetRoot common.Hash,
	requestID common.Hash,
	shareID uint64,
) (string, error) {
	if store == nil ||
		keysetRoot == (common.Hash{}) ||
		requestID == (common.Hash{}) ||
		shareID == 0 ||
		shareID > context.CommitteeSize {
		return "", ErrInvalidThresholdPartialStoreV1
	}

	sessionID, err := lqc.RabbitVRFDKGSessionIDV1(context)
	if err != nil || sessionID == (common.Hash{}) {
		return "", ErrInvalidThresholdPartialStoreV1
	}

	var shareIDBytes [8]byte
	binary.BigEndian.PutUint64(shareIDBytes[:], shareID)

	slotID := crypto.Keccak256Hash(
		[]byte("RABBIT-VRF-THRESHOLD-PARTIAL-STORE-SLOT-V1"),
		sessionID[:],
		keysetRoot[:],
		requestID[:],
		shareIDBytes[:],
	)

	return filepath.Join(
		store.dir,
		fmt.Sprintf("threshold-partial-%x.json", slotID[:]),
	), nil
}

func (store *ThresholdPartialStoreV1) Load(
	context lqc.RabbitVRFDKGSessionContextV1,
	keysetRoot common.Hash,
	requestID common.Hash,
	shareID uint64,
) (lqc.RabbitVRFThresholdPartialV1, error) {
	var zero lqc.RabbitVRFThresholdPartialV1

	path, err := store.Path(context, keysetRoot, requestID, shareID)
	if err != nil {
		return zero, err
	}

	encoded, err := readPrivateFileV1(path)
	if err != nil {
		return zero, err
	}

	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()

	var record thresholdPartialStoreFileV1
	if err := decoder.Decode(&record); err != nil {
		return zero, ErrInvalidThresholdPartialStoreV1
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return zero, ErrInvalidThresholdPartialStoreV1
	}

	sessionID, err := lqc.RabbitVRFDKGSessionIDV1(context)
	if err != nil ||
		record.StoreVersion != ThresholdPartialStoreVersionV1 ||
		record.SessionID != sessionID ||
		record.KeysetRoot != keysetRoot ||
		record.RequestID != requestID ||
		record.ShareID != shareID {
		return zero, ErrInvalidThresholdPartialStoreV1
	}

	return validateThresholdPartialV1(context, record.Packet)
}

func (store *ThresholdPartialStoreV1) Store(
	context lqc.RabbitVRFDKGSessionContextV1,
	packet lqc.RabbitVRFThresholdPartialV1,
) error {
	validated, err := validateThresholdPartialV1(context, packet)
	if err != nil {
		return err
	}

	path, err := store.Path(
		context,
		validated.KeysetRoot,
		validated.RequestID,
		validated.ShareID,
	)
	if err != nil {
		return err
	}

	if _, err := os.Stat(path); err == nil {
		existing, loadErr := store.Load(
			context,
			validated.KeysetRoot,
			validated.RequestID,
			validated.ShareID,
		)
		if loadErr != nil {
			return loadErr
		}
		if existing == validated {
			return nil
		}
		return ErrThresholdPartialStoreConflictV1
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	record := thresholdPartialStoreFileV1{
		StoreVersion: ThresholdPartialStoreVersionV1,
		SessionID:    validated.SessionID,
		KeysetRoot:   validated.KeysetRoot,
		RequestID:    validated.RequestID,
		ShareID:      validated.ShareID,
		Packet:       validated,
	}

	encoded, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	encoded = append(encoded, byte(10))

	if err := atomicWritePrivateFileNoReplaceV1(path, encoded); err != nil {
		if _, statErr := os.Stat(path); statErr == nil {
			existing, loadErr := store.Load(
				context,
				validated.KeysetRoot,
				validated.RequestID,
				validated.ShareID,
			)
			if loadErr == nil && existing == validated {
				return nil
			}
			if loadErr == nil {
				return ErrThresholdPartialStoreConflictV1
			}
		}
		return err
	}

	return nil
}
