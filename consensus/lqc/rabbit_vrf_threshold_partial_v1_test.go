package lqc

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
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
