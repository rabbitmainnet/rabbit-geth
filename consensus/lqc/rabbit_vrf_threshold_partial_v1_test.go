package lqc

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	gethcrypto "github.com/ethereum/go-ethereum/crypto"
	rabbitvrf "github.com/ethereum/go-ethereum/crypto/rabbitvrf"
)

func TestRabbitVRFThresholdMessageAndPartialV1(t *testing.T) {
	context, err := NewRabbitVRFDKGSessionContextV1(
		big.NewInt(9280),
		11,
		common.HexToHash("0x1234"),
		32,
	)
	if err != nil {
		t.Fatal(err)
	}

	keysetRoot := common.HexToHash("0xaaaa")
	requestID := common.HexToHash("0xbbbb")

	messageA, hashA, err := RabbitVRFThresholdMessageV1(
		context,
		keysetRoot,
		requestID,
	)
	if err != nil {
		t.Fatal(err)
	}
	messageB, hashB, err := RabbitVRFThresholdMessageV1(
		context,
		keysetRoot,
		requestID,
	)
	if err != nil {
		t.Fatal(err)
	}

	if string(messageA) != string(messageB) || hashA != hashB {
		t.Fatal("threshold message is not deterministic")
	}

	_, changedRequestHash, err := RabbitVRFThresholdMessageV1(
		context,
		keysetRoot,
		common.HexToHash("0xbbbc"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if changedRequestHash == hashA {
		t.Fatal("request id did not bind threshold message")
	}

	_, changedKeysetHash, err := RabbitVRFThresholdMessageV1(
		context,
		common.HexToHash("0xaaab"),
		requestID,
	)
	if err != nil {
		t.Fatal(err)
	}
	if changedKeysetHash == hashA {
		t.Fatal("keyset root did not bind threshold message")
	}

	partial := rabbitvrf.PartialSignature{ShareID: 1}
	partial.Signature[0] = 1

	packet, err := NewRabbitVRFThresholdPartialV1(
		context,
		keysetRoot,
		requestID,
		partial,
	)
	if err != nil {
		t.Fatal(err)
	}

	if packet.MessageHash != hashA ||
		packet.ShareID != partial.ShareID ||
		packet.SessionID == (common.Hash{}) {
		t.Fatal("threshold partial packet binding mismatch")
	}

	idA, err := RabbitVRFThresholdPartialMessageIDV1(packet)
	if err != nil {
		t.Fatal(err)
	}
	idB, err := RabbitVRFThresholdPartialMessageIDV1(packet)
	if err != nil {
		t.Fatal(err)
	}
	if idA != idB {
		t.Fatal("threshold partial message id is not deterministic")
	}

	changed := packet
	changed.Signature[0] ^= 0xff
	idChanged, err := RabbitVRFThresholdPartialMessageIDV1(changed)
	if err != nil {
		t.Fatal(err)
	}
	if idChanged == idA {
		t.Fatal("threshold partial signature did not bind message id")
	}

	participantKey, err := gethcrypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	transportKey, err := gethcrypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	member := RabbitVRFCommitteeMemberV1{
		ShareID:     1,
		TicketHash:  gethcrypto.Keccak256Hash([]byte("rabbit-vrf-threshold-participation-ticket")),
		Participant: gethcrypto.PubkeyToAddress(participantKey.PublicKey),
	}
	transportPublic, err := RabbitVRFDKGTransportPublicKeyV1FromBytes(
		gethcrypto.CompressPubkey(&transportKey.PublicKey),
	)
	if err != nil {
		t.Fatal(err)
	}
	binding, _, err := NewRabbitVRFDKGTransportKeyBindingV1(context, member, transportPublic)
	if err != nil {
		t.Fatal(err)
	}
	transportRoot := gethcrypto.Keccak256Hash([]byte("rabbit-vrf-threshold-participation-transport-root"))
	signingHash, err := RabbitVRFParticipationSigningHashV1(
		context,
		transportRoot,
		keysetRoot,
		requestID,
		packet.MessageHash,
		idA,
		member,
	)
	if err != nil {
		t.Fatal(err)
	}
	participationSignature, err := gethcrypto.Sign(signingHash[:], transportKey)
	if err != nil {
		t.Fatal(err)
	}
	copy(packet.ParticipationSignature[:], participationSignature[:RabbitVRFParticipationSignatureSizeV1])
	if err := ValidateRabbitVRFThresholdPartialParticipationV1(
		context,
		transportRoot,
		member,
		binding,
		packet,
	); err != nil {
		t.Fatalf("valid threshold participation rejected: %v", err)
	}
	idAfterAttestation, err := RabbitVRFThresholdPartialMessageIDV1(packet)
	if err != nil {
		t.Fatal(err)
	}
	if idAfterAttestation != idA {
		t.Fatal("participation signature created a circular threshold partial message id")
	}
	badParticipation := packet
	badParticipation.ParticipationSignature[0] ^= 0x01
	if err := ValidateRabbitVRFThresholdPartialParticipationV1(
		context,
		transportRoot,
		member,
		binding,
		badParticipation,
	); err == nil {
		t.Fatal("invalid threshold participation signature accepted")
	}

	if _, _, err := RabbitVRFThresholdMessageV1(
		context,
		common.Hash{},
		requestID,
	); err == nil {
		t.Fatal("zero keyset root accepted")
	}

	if _, _, err := RabbitVRFThresholdMessageV1(
		context,
		keysetRoot,
		common.Hash{},
	); err == nil {
		t.Fatal("zero request id accepted")
	}
}
