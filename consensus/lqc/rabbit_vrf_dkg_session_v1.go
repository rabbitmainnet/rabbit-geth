package lqc

import (
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
)

const RabbitVRFDKGSessionVersionV1 uint8 = 1

var ErrInvalidRabbitVRFDKGSessionV1 = errors.New(
	"invalid rabbit vrf dkg session v1",
)

var rabbitVRFDKGSessionDomainV1 = []byte(
	"RABBIT-VRF-DKG-SESSION-V1",
)

// RabbitVRFThresholdPolicyV1 freezes the threshold before DKG begins.
//
// Let N be the original deterministic VRF committee size:
//
//	f = floor((N - 1) / 3)
//	threshold = N - f
//
// A completed DKG must later have at least threshold qualified share holders.
// Complaints, absence or disqualification MUST NOT lower this threshold.
func RabbitVRFThresholdPolicyV1(
	committeeSize uint64,
) (
	threshold uint64,
	maxFaults uint64,
	err error,
) {
	if committeeSize == 0 {
		return 0, 0, ErrInvalidRabbitVRFDKGSessionV1
	}

	maxFaults = (committeeSize - 1) / 3
	threshold = committeeSize - maxFaults

	if threshold == 0 ||
		threshold > committeeSize ||
		threshold <= maxFaults {
		return 0, 0, ErrInvalidRabbitVRFDKGSessionV1
	}

	return threshold, maxFaults, nil
}

// RabbitVRFDKGSessionContextV1 is the immutable public context established
// before any DKG polynomial commitment or private-share exchange.
//
// CommitteeSize is the original deterministic committee size, not the number
// of members that may later survive DKG qualification.
type RabbitVRFDKGSessionContextV1 struct {
	Version        uint8
	ChainID        *big.Int
	TargetVRFEpoch uint64
	CommitteeRoot  common.Hash
	CommitteeSize  uint64
	Threshold      uint64
	MaxFaults      uint64
}

type rabbitVRFDKGSessionPayloadV1 struct {
	Domain         []byte
	Version        uint8
	ChainID        *big.Int
	TargetVRFEpoch uint64
	CommitteeRoot  common.Hash
	CommitteeSize  uint64
	Threshold      uint64
	MaxFaults      uint64
}

// NewRabbitVRFDKGSessionContextV1 constructs the only valid V1 session context
// for an original deterministic committee.
func NewRabbitVRFDKGSessionContextV1(
	chainID *big.Int,
	targetVRFEpoch uint64,
	committeeRoot common.Hash,
	committeeSize uint64,
) (
	RabbitVRFDKGSessionContextV1,
	error,
) {
	if chainID == nil ||
		chainID.Sign() <= 0 ||
		targetVRFEpoch == 0 ||
		committeeRoot == (common.Hash{}) {
		return RabbitVRFDKGSessionContextV1{},
			ErrInvalidRabbitVRFDKGSessionV1
	}

	threshold, maxFaults, err :=
		RabbitVRFThresholdPolicyV1(committeeSize)
	if err != nil {
		return RabbitVRFDKGSessionContextV1{}, err
	}

	return RabbitVRFDKGSessionContextV1{
		Version:        RabbitVRFDKGSessionVersionV1,
		ChainID:        new(big.Int).Set(chainID),
		TargetVRFEpoch: targetVRFEpoch,
		CommitteeRoot:  committeeRoot,
		CommitteeSize:  committeeSize,
		Threshold:      threshold,
		MaxFaults:      maxFaults,
	}, nil
}

// ValidateRabbitVRFDKGSessionContextV1 rejects any context whose threshold or
// fault budget differs from the deterministic V1 policy.
func ValidateRabbitVRFDKGSessionContextV1(
	context RabbitVRFDKGSessionContextV1,
) error {
	if context.Version != RabbitVRFDKGSessionVersionV1 ||
		context.ChainID == nil ||
		context.ChainID.Sign() <= 0 ||
		context.TargetVRFEpoch == 0 ||
		context.CommitteeRoot == (common.Hash{}) ||
		context.CommitteeSize == 0 {
		return ErrInvalidRabbitVRFDKGSessionV1
	}

	threshold, maxFaults, err :=
		RabbitVRFThresholdPolicyV1(context.CommitteeSize)
	if err != nil {
		return err
	}

	if context.Threshold != threshold ||
		context.MaxFaults != maxFaults {
		return ErrInvalidRabbitVRFDKGSessionV1
	}

	return nil
}

// RabbitVRFDKGSessionIDV1 derives the canonical session identifier.
//
// It deliberately excludes producer identity, heartbeat/liveness state,
// network arrival order, local peer ordering and administrator input.
func RabbitVRFDKGSessionIDV1(
	context RabbitVRFDKGSessionContextV1,
) (
	common.Hash,
	error,
) {
	if err := ValidateRabbitVRFDKGSessionContextV1(
		context,
	); err != nil {
		return common.Hash{}, err
	}

	encoded, err := rlp.EncodeToBytes(
		rabbitVRFDKGSessionPayloadV1{
			Domain:         rabbitVRFDKGSessionDomainV1,
			Version:        context.Version,
			ChainID:        new(big.Int).Set(context.ChainID),
			TargetVRFEpoch: context.TargetVRFEpoch,
			CommitteeRoot:  context.CommitteeRoot,
			CommitteeSize:  context.CommitteeSize,
			Threshold:      context.Threshold,
			MaxFaults:      context.MaxFaults,
		},
	)
	if err != nil {
		return common.Hash{}, err
	}

	sessionID := crypto.Keccak256Hash(encoded)
	if sessionID == (common.Hash{}) {
		return common.Hash{}, ErrInvalidRabbitVRFDKGSessionV1
	}

	return sessionID, nil
}
