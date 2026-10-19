package lqc

import (
	"bytes"
	"errors"
	"fmt"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
)

const RabbitVRFCompactParticipationVersionV1 uint8 = 1

const RabbitVRFCompactParticipationBitmapBytesV1 = 16

var ErrInvalidRabbitVRFCompactParticipationV1 = errors.New("invalid rabbit vrf compact participation v1")

var rabbitVRFCompactParticipationDomainV1 = []byte("RABBIT-VRF-COMPACT-PARTICIPATION-V1")

type rabbitVRFCompactParticipationMessagePayloadV1 struct {
	Domain      []byte
	Version     uint8
	SessionID   common.Hash
	KeysetRoot  common.Hash
	RequestID   common.Hash
	MessageHash common.Hash
	ShareID     uint64
	Participant common.Address
}

// RabbitVRFCompactParticipationMessageV1 returns the canonical, distinct
// per-share message used by the compact BLS participation proof.
//
// The message deliberately does not contain the ordinary threshold partial
// signature. This keeps the final proof reconstructible from canonical chain
// state while still binding the attestation to the session, keyset, request,
// threshold message, share and participant.
func RabbitVRFCompactParticipationMessageV1(
	context RabbitVRFDKGSessionContextV1,
	keysetRoot common.Hash,
	requestID common.Hash,
	messageHash common.Hash,
	member RabbitVRFCommitteeMemberV1,
) ([]byte, common.Hash, error) {
	if err := ValidateRabbitVRFDKGSessionContextV1(context); err != nil ||
		keysetRoot == (common.Hash{}) ||
		requestID == (common.Hash{}) ||
		messageHash == (common.Hash{}) ||
		member.ShareID == 0 ||
		member.ShareID > context.CommitteeSize ||
		member.Participant == (common.Address{}) {
		return nil, common.Hash{}, ErrInvalidRabbitVRFCompactParticipationV1
	}

	sessionID, err := RabbitVRFDKGSessionIDV1(context)
	if err != nil || sessionID == (common.Hash{}) {
		return nil, common.Hash{}, ErrInvalidRabbitVRFCompactParticipationV1
	}

	encoded, err := rlp.EncodeToBytes(rabbitVRFCompactParticipationMessagePayloadV1{
		Domain:      append([]byte(nil), rabbitVRFCompactParticipationDomainV1...),
		Version:     RabbitVRFCompactParticipationVersionV1,
		SessionID:   sessionID,
		KeysetRoot:  keysetRoot,
		RequestID:   requestID,
		MessageHash: messageHash,
		ShareID:     member.ShareID,
		Participant: member.Participant,
	})
	if err != nil {
		return nil, common.Hash{}, fmt.Errorf("%w: %v", ErrInvalidRabbitVRFCompactParticipationV1, err)
	}

	hash := crypto.Keccak256Hash(encoded)
	if hash == (common.Hash{}) {
		return nil, common.Hash{}, ErrInvalidRabbitVRFCompactParticipationV1
	}
	return encoded, hash, nil
}

// RabbitVRFCompactParticipationBitmapV1 creates the canonical bitmap for
// exactly threshold distinct ShareIDs. Input order does not affect the result.
func RabbitVRFCompactParticipationBitmapV1(
	context RabbitVRFDKGSessionContextV1,
	shareIDs []uint64,
) ([]byte, error) {
	if err := ValidateRabbitVRFDKGSessionContextV1(context); err != nil ||
		uint64(len(shareIDs)) != context.Threshold {
		return nil, ErrInvalidRabbitVRFCompactParticipationV1
	}

	ordered := append([]uint64(nil), shareIDs...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })

	bitmap := make([]byte, rabbitVRFParticipationBitmapLenV1(context.CommitteeSize))
	for index, shareID := range ordered {
		if shareID == 0 || shareID > context.CommitteeSize {
			return nil, ErrInvalidRabbitVRFCompactParticipationV1
		}
		if index > 0 && ordered[index-1] == shareID {
			return nil, ErrInvalidRabbitVRFCompactParticipationV1
		}
		bit := shareID - 1
		bitmap[bit/8] |= byte(1) << uint(bit%8)
	}

	decoded, err := rabbitVRFParticipationShareIDsV1(context, bitmap)
	if err != nil || !equalRabbitVRFShareIDsV1(decoded, ordered) {
		return nil, ErrInvalidRabbitVRFCompactParticipationV1
	}
	return bitmap, nil
}

// RabbitVRFCompactParticipationShareIDsV1 validates a compact participation
// bitmap and returns exactly threshold ShareIDs in canonical ascending order.
func RabbitVRFCompactParticipationShareIDsV1(
	context RabbitVRFDKGSessionContextV1,
	bitmap []byte,
) ([]uint64, error) {
	shareIDs, err := rabbitVRFParticipationShareIDsV1(context, bitmap)
	if err != nil {
		return nil, ErrInvalidRabbitVRFCompactParticipationV1
	}
	return append([]uint64(nil), shareIDs...), nil
}

func RabbitVRFCompactParticipationFixedBitmapV1(
	context RabbitVRFDKGSessionContextV1,
	shareIDs []uint64,
) ([RabbitVRFCompactParticipationBitmapBytesV1]byte, error) {
	var fixed [RabbitVRFCompactParticipationBitmapBytesV1]byte

	if context.CommitteeSize == 0 || context.CommitteeSize > RabbitVRFCompactParticipationBitmapBytesV1*8 {
		return fixed, ErrInvalidRabbitVRFCompactParticipationV1
	}
	canonical, err := RabbitVRFCompactParticipationBitmapV1(context, shareIDs)
	if err != nil {
		return fixed, err
	}
	if len(canonical) > len(fixed) {
		return fixed, ErrInvalidRabbitVRFCompactParticipationV1
	}
	copy(fixed[:], canonical)
	return fixed, nil
}

func RabbitVRFCompactParticipationShareIDsFromFixedBitmapV1(
	context RabbitVRFDKGSessionContextV1,
	fixed [RabbitVRFCompactParticipationBitmapBytesV1]byte,
) ([]uint64, error) {
	if context.CommitteeSize == 0 || context.CommitteeSize > RabbitVRFCompactParticipationBitmapBytesV1*8 {
		return nil, ErrInvalidRabbitVRFCompactParticipationV1
	}
	canonicalLen := rabbitVRFParticipationBitmapLenV1(context.CommitteeSize)
	if canonicalLen <= 0 || canonicalLen > len(fixed) {
		return nil, ErrInvalidRabbitVRFCompactParticipationV1
	}
	for _, value := range fixed[canonicalLen:] {
		if value != 0 {
			return nil, ErrInvalidRabbitVRFCompactParticipationV1
		}
	}
	return RabbitVRFCompactParticipationShareIDsV1(context, fixed[:canonicalLen])
}

func equalRabbitVRFShareIDsV1(a, b []uint64) bool {
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

// Keep bytes imported deliberately here: the equality check below protects
// against accidental future domain aliasing without exporting mutable state.
func init() {
	if len(rabbitVRFCompactParticipationDomainV1) == 0 ||
		bytes.Equal(rabbitVRFCompactParticipationDomainV1, []byte("RABBIT-VRF-PARTICIPATION-SIGN-V1")) {
		panic("invalid Rabbit VRF compact participation domain")
	}
}
