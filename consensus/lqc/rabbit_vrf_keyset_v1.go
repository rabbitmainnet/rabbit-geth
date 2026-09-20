package lqc

import (
	"errors"
	"math/big"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	rabbitvrf "github.com/ethereum/go-ethereum/crypto/rabbitvrf"
	"github.com/ethereum/go-ethereum/rlp"
)

const RabbitVRFKeysetVersionV1 uint8 = 1

var ErrInvalidRabbitVRFKeysetV1 = errors.New(
	"invalid rabbit vrf keyset v1",
)

var rabbitVRFKeysetRootDomainV1 = []byte(
	"RABBIT-VRF-KEYSET-ROOT-V1",
)

// RabbitVRFVerificationShareV1 is one public DKG verification share bound to
// the immutable ShareID assigned by the canonical VRF committee.
//
// Missing ShareIDs are not renumbered. A future DKG qualification state machine
// decides which committee members may appear in an activatable keyset.
type RabbitVRFVerificationShareV1 struct {
	ShareID   uint64
	PublicKey rabbitvrf.PublicKey
}

type rabbitVRFKeysetRootPayloadV1 struct {
	Domain             []byte
	Version            uint8
	ChainID            *big.Int
	VRFEpoch           uint64
	CommitteeRoot      common.Hash
	CommitteeSize      uint64
	Threshold          uint64
	ThresholdPublicKey rabbitvrf.PublicKey
	TranscriptRoot     common.Hash
	VerificationShares []RabbitVRFVerificationShareV1
}

func canonicalRabbitVRFVerificationSharesV1(
	committeeSize uint64,
	input []RabbitVRFVerificationShareV1,
) ([]RabbitVRFVerificationShareV1, error) {
	if committeeSize == 0 ||
		len(input) == 0 ||
		uint64(len(input)) > committeeSize {
		return nil, ErrInvalidRabbitVRFKeysetV1
	}

	out := append(
		[]RabbitVRFVerificationShareV1(nil),
		input...,
	)

	sort.Slice(out, func(i, j int) bool {
		return out[i].ShareID < out[j].ShareID
	})

	var previous uint64

	for index, share := range out {
		if share.ShareID == 0 ||
			share.ShareID > committeeSize {
			return nil, ErrInvalidRabbitVRFKeysetV1
		}

		if index > 0 &&
			share.ShareID == previous {
			return nil, ErrInvalidRabbitVRFKeysetV1
		}

		if _, err := rabbitvrf.NewVerificationShare(
			share.ShareID,
			share.PublicKey,
		); err != nil {
			return nil, ErrInvalidRabbitVRFKeysetV1
		}

		previous = share.ShareID
	}

	return out, nil
}

// RabbitVRFKeysetRootV1 commits to public threshold-key material produced by a
// future decentralized DKG.
//
// This function is a commitment primitive only. It does not decide the Rabbit
// VRF threshold formula, DKG qualification policy, epoch schedule or whether a
// keyset is eligible for activation.
func RabbitVRFKeysetRootV1(
	chainID *big.Int,
	vrfEpoch uint64,
	committeeRoot common.Hash,
	committeeSize uint64,
	threshold uint64,
	thresholdPublicKey rabbitvrf.PublicKey,
	transcriptRoot common.Hash,
	verificationShares []RabbitVRFVerificationShareV1,
) (
	common.Hash,
	[]RabbitVRFVerificationShareV1,
	error,
) {
	if chainID == nil ||
		chainID.Sign() <= 0 ||
		vrfEpoch == 0 ||
		committeeRoot == (common.Hash{}) ||
		committeeSize == 0 ||
		threshold == 0 ||
		transcriptRoot == (common.Hash{}) {
		return common.Hash{},
			nil,
			ErrInvalidRabbitVRFKeysetV1
	}

	if _, err := rabbitvrf.NewVerificationShare(
		1,
		thresholdPublicKey,
	); err != nil {
		return common.Hash{},
			nil,
			ErrInvalidRabbitVRFKeysetV1
	}

	canonical, err := canonicalRabbitVRFVerificationSharesV1(
		committeeSize,
		verificationShares,
	)
	if err != nil {
		return common.Hash{}, nil, err
	}

	if threshold > uint64(len(canonical)) {
		return common.Hash{},
			nil,
			ErrInvalidRabbitVRFKeysetV1
	}

	encoded, err := rlp.EncodeToBytes(
		rabbitVRFKeysetRootPayloadV1{
			Domain:             rabbitVRFKeysetRootDomainV1,
			Version:            RabbitVRFKeysetVersionV1,
			ChainID:            new(big.Int).Set(chainID),
			VRFEpoch:           vrfEpoch,
			CommitteeRoot:      committeeRoot,
			CommitteeSize:      committeeSize,
			Threshold:          threshold,
			ThresholdPublicKey: thresholdPublicKey,
			TranscriptRoot:     transcriptRoot,
			VerificationShares: canonical,
		},
	)
	if err != nil {
		return common.Hash{}, nil, err
	}

	root := crypto.Keccak256Hash(encoded)
	if root == (common.Hash{}) {
		return common.Hash{},
			nil,
			ErrInvalidRabbitVRFKeysetV1
	}

	return root, canonical, nil
}
