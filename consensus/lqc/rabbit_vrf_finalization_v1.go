package lqc

import (
	"bytes"
	"errors"
	"fmt"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	rabbitvrf "github.com/ethereum/go-ethereum/crypto/rabbitvrf"
	"github.com/ethereum/go-ethereum/rlp"
)

const RabbitVRFFinalizationVersionV1 uint8 = 1

var (
	ErrInvalidRabbitVRFFinalizationV1       = errors.New("invalid rabbit vrf finalization v1")
	ErrDuplicateRabbitVRFFinalizationV1     = errors.New("duplicate rabbit vrf finalization v1")
	ErrNonCanonicalRabbitVRFFinalizationsV1 = errors.New("non-canonical rabbit vrf finalizations v1")
)

type RabbitVRFFinalizationV1 struct {
	Version    uint8
	RequestID  common.Hash
	KeysetRoot common.Hash
	Epoch      uint64
	Round      uint64
	Randomness common.Hash
	ProofHash  common.Hash
	Signature  rabbitvrf.Signature
}

func (value RabbitVRFFinalizationV1) Validate() error {
	if value.Version != RabbitVRFFinalizationVersionV1 ||
		value.RequestID == (common.Hash{}) ||
		value.KeysetRoot == (common.Hash{}) ||
		value.Epoch == 0 ||
		value.Randomness == (common.Hash{}) ||
		value.ProofHash == (common.Hash{}) ||
		value.Signature == (rabbitvrf.Signature{}) {
		return ErrInvalidRabbitVRFFinalizationV1
	}
	return nil
}

func ValidateRabbitVRFFinalizationProofV1(
	context RabbitVRFDKGSessionContextV1,
	certificate RabbitVRFKeysetCertificateV1,
	value RabbitVRFFinalizationV1,
) error {
	if err := value.Validate(); err != nil {
		return err
	}
	if err := ValidateRabbitVRFKeysetCertificateShapeV1(certificate); err != nil {
		return err
	}
	if value.KeysetRoot != certificate.KeysetRoot || value.Epoch != context.TargetVRFEpoch {
		return ErrInvalidRabbitVRFFinalizationV1
	}
	message, _, err := RabbitVRFThresholdMessageV1(context, value.KeysetRoot, value.RequestID)
	if err != nil {
		return ErrInvalidRabbitVRFFinalizationV1
	}
	randomness, err := rabbitvrf.VerifyAndDeriveRandomness(certificate.ThresholdPublicKey, message, value.Signature)
	if err != nil || randomness != value.Randomness {
		return ErrInvalidRabbitVRFFinalizationV1
	}
	proofHash, err := RabbitVRFFinalizationProofHashV1(value.RequestID, value.Epoch, value.Round, value.Signature[:])
	if err != nil || proofHash != value.ProofHash {
		return ErrInvalidRabbitVRFFinalizationV1
	}
	return nil
}

func RabbitVRFFinalizationProofHashV1(
	requestID common.Hash,
	epoch uint64,
	round uint64,
	signature []byte,
) (common.Hash, error) {
	if requestID == (common.Hash{}) || epoch == 0 || len(signature) == 0 {
		return common.Hash{}, ErrInvalidRabbitVRFFinalizationV1
	}

	payload, err := rlp.EncodeToBytes([]interface{}{
		[]byte("RABBIT-VRF-FINALIZATION-PROOF-V1"),
		requestID,
		epoch,
		round,
		signature,
	})
	if err != nil {
		return common.Hash{}, fmt.Errorf("%w: %v", ErrInvalidRabbitVRFFinalizationV1, err)
	}
	return crypto.Keccak256Hash(payload), nil
}

func CanonicalRabbitVRFFinalizationsV1(
	values []RabbitVRFFinalizationV1,
) ([]RabbitVRFFinalizationV1, error) {
	out := append([]RabbitVRFFinalizationV1(nil), values...)

	for _, value := range out {
		if err := value.Validate(); err != nil {
			return nil, err
		}
	}

	sort.Slice(out, func(i, j int) bool {
		return bytes.Compare(out[i].RequestID[:], out[j].RequestID[:]) < 0
	})

	for index := 1; index < len(out); index++ {
		if out[index-1].RequestID == out[index].RequestID {
			return nil, ErrDuplicateRabbitVRFFinalizationV1
		}
	}
	return out, nil
}

func ValidateCanonicalRabbitVRFFinalizationsV1(
	values []RabbitVRFFinalizationV1,
) error {
	canonical, err := CanonicalRabbitVRFFinalizationsV1(values)
	if err != nil {
		return err
	}
	for index := range values {
		if canonical[index] != values[index] {
			return ErrNonCanonicalRabbitVRFFinalizationsV1
		}
	}
	return nil
}
