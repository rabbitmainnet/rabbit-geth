package lqc

import (
	"errors"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
)

const RabbitVRFDKGTransportKeySetVersionV1 uint8 = 1

var ErrInvalidRabbitVRFDKGTransportKeySetV1 = errors.New(
	"invalid rabbit vrf dkg transport key set v1",
)

var rabbitVRFDKGTransportKeySetDomainV1 = []byte(
	"RABBIT-VRF-DKG-TRANSPORT-KEY-SET-V1",
)

type rabbitVRFDKGTransportKeySetEntryV1 struct {
	ShareID     uint64
	Participant common.Address
	KeyRoot     common.Hash
}

type rabbitVRFDKGTransportKeySetPayloadV1 struct {
	Domain    []byte
	Version   uint8
	SessionID common.Hash
	Entries   []rabbitVRFDKGTransportKeySetEntryV1
}

type rabbitVRFDKGTransportKeySetPairV1 struct {
	member   RabbitVRFCommitteeMemberV1
	binding  RabbitVRFDKGTransportKeyBindingV1
	envelope RabbitVRFDKGEnvelopeV1
}

func RabbitVRFDKGTransportKeySetRootV1(
	context RabbitVRFDKGSessionContextV1,
	members []RabbitVRFCommitteeMemberV1,
	bindings []RabbitVRFDKGTransportKeyBindingV1,
	envelopes []RabbitVRFDKGEnvelopeV1,
) (common.Hash, error) {
	if err := ValidateRabbitVRFDKGSessionContextV1(context); err != nil {
		return common.Hash{}, ErrInvalidRabbitVRFDKGTransportKeySetV1
	}

	if context.CommitteeSize == 0 ||
		uint64(len(members)) != context.CommitteeSize ||
		len(bindings) != len(members) ||
		len(envelopes) != len(members) {
		return common.Hash{}, ErrInvalidRabbitVRFDKGTransportKeySetV1
	}

	sessionID, err := RabbitVRFDKGSessionIDV1(context)
	if err != nil {
		return common.Hash{}, ErrInvalidRabbitVRFDKGTransportKeySetV1
	}

	pairs := make(
		[]rabbitVRFDKGTransportKeySetPairV1,
		len(members),
	)
	for index := range members {
		pairs[index] = rabbitVRFDKGTransportKeySetPairV1{
			member:   members[index],
			binding:  bindings[index],
			envelope: envelopes[index],
		}
	}

	sort.Slice(pairs, func(i, j int) bool {
		return pairs[i].member.ShareID < pairs[j].member.ShareID
	})

	seenShares := make(map[uint64]struct{}, len(pairs))
	seenParticipants := make(map[common.Address]struct{}, len(pairs))

	entries := make(
		[]rabbitVRFDKGTransportKeySetEntryV1,
		0,
		len(pairs),
	)

	for _, pair := range pairs {
		member := pair.member

		if member.ShareID == 0 ||
			member.ShareID > context.CommitteeSize ||
			member.TicketHash == (common.Hash{}) ||
			member.Participant == (common.Address{}) {
			return common.Hash{}, ErrInvalidRabbitVRFDKGTransportKeySetV1
		}

		if _, exists := seenShares[member.ShareID]; exists {
			return common.Hash{}, ErrInvalidRabbitVRFDKGTransportKeySetV1
		}
		seenShares[member.ShareID] = struct{}{}

		if _, exists := seenParticipants[member.Participant]; exists {
			return common.Hash{}, ErrInvalidRabbitVRFDKGTransportKeySetV1
		}
		seenParticipants[member.Participant] = struct{}{}

		if err := VerifyRabbitVRFDKGTransportKeyEnvelopeV1(
			context,
			member,
			pair.binding,
			pair.envelope,
		); err != nil {
			return common.Hash{}, ErrInvalidRabbitVRFDKGTransportKeySetV1
		}

		root, err := VerifyRabbitVRFDKGTransportKeyBindingV1(
			context,
			member,
			pair.binding,
		)
		if err != nil {
			return common.Hash{}, ErrInvalidRabbitVRFDKGTransportKeySetV1
		}

		entries = append(
			entries,
			rabbitVRFDKGTransportKeySetEntryV1{
				ShareID:     member.ShareID,
				Participant: member.Participant,
				KeyRoot:     root,
			},
		)
	}

	encoded, err := rlp.EncodeToBytes(
		rabbitVRFDKGTransportKeySetPayloadV1{
			Domain:    rabbitVRFDKGTransportKeySetDomainV1,
			Version:   RabbitVRFDKGTransportKeySetVersionV1,
			SessionID: sessionID,
			Entries:   entries,
		},
	)
	if err != nil {
		return common.Hash{}, ErrInvalidRabbitVRFDKGTransportKeySetV1
	}

	root := crypto.Keccak256Hash(encoded)
	if root == (common.Hash{}) {
		return common.Hash{}, ErrInvalidRabbitVRFDKGTransportKeySetV1
	}

	return root, nil
}
