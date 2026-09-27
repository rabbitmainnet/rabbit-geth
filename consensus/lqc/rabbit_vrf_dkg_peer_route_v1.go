package lqc

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
)

var rabbitVRFDKGPeerRouteDomainV1 = []byte("RABBIT-VRF-DKG-PEER-ROUTE-V1")

type rabbitVRFDKGPeerRoutePayloadV1 struct {
	Domain      []byte
	SessionID   common.Hash
	ShareID     uint64
	Participant common.Address
	PeerID      common.Hash
}

func RabbitVRFDKGPeerRoutePayloadHashV1(context RabbitVRFDKGSessionContextV1, member RabbitVRFCommitteeMemberV1, peerID common.Hash) (common.Hash, error) {
	if peerID == (common.Hash{}) || member.ShareID == 0 || member.ShareID > context.CommitteeSize || member.Participant == (common.Address{}) || member.TicketHash == (common.Hash{}) {
		return common.Hash{}, ErrInvalidRabbitVRFDKGEnvelopeV1
	}
	sessionID, err := RabbitVRFDKGSessionIDV1(context)
	if err != nil {
		return common.Hash{}, ErrInvalidRabbitVRFDKGEnvelopeV1
	}
	encoded, err := rlp.EncodeToBytes(rabbitVRFDKGPeerRoutePayloadV1{Domain: append([]byte(nil), rabbitVRFDKGPeerRouteDomainV1...), SessionID: sessionID, ShareID: member.ShareID, Participant: member.Participant, PeerID: peerID})
	if err != nil {
		return common.Hash{}, ErrInvalidRabbitVRFDKGEnvelopeV1
	}
	root := crypto.Keccak256Hash(encoded)
	if root == (common.Hash{}) {
		return common.Hash{}, ErrInvalidRabbitVRFDKGEnvelopeV1
	}
	return root, nil
}
