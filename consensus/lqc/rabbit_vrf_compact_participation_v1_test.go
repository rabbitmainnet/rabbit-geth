package lqc

import (
	"bytes"
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

func TestRabbitVRFCompactParticipationMessageV1(t *testing.T) {
	context, err := NewRabbitVRFDKGSessionContextV1(
		big.NewInt(9280),
		11,
		crypto.Keccak256Hash([]byte("compact-participation-committee")),
		4,
	)
	if err != nil {
		t.Fatal(err)
	}
	if context.Threshold != 3 {
		t.Fatalf("threshold=%d want=3", context.Threshold)
	}

	member1 := RabbitVRFCommitteeMemberV1{
		ShareID:     1,
		TicketHash:  crypto.Keccak256Hash([]byte("ticket-1")),
		Participant: common.HexToAddress("0x0000000000000000000000000000000000000011"),
	}
	member2 := RabbitVRFCommitteeMemberV1{
		ShareID:     2,
		TicketHash:  crypto.Keccak256Hash([]byte("ticket-2")),
		Participant: common.HexToAddress("0x0000000000000000000000000000000000000022"),
	}

	keysetRoot := crypto.Keccak256Hash([]byte("compact-participation-keyset"))
	requestID := crypto.Keccak256Hash([]byte("compact-participation-request"))
	messageHash := crypto.Keccak256Hash([]byte("compact-participation-threshold-message"))

	first, firstHash, err := RabbitVRFCompactParticipationMessageV1(
		context, keysetRoot, requestID, messageHash, member1,
	)
	if err != nil {
		t.Fatal(err)
	}
	second, secondHash, err := RabbitVRFCompactParticipationMessageV1(
		context, keysetRoot, requestID, messageHash, member1,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) == 0 || firstHash == (common.Hash{}) {
		t.Fatal("empty compact participation message")
	}
	if !bytes.Equal(first, second) || firstHash != secondHash {
		t.Fatal("compact participation message is not deterministic")
	}

	_, member2Hash, err := RabbitVRFCompactParticipationMessageV1(
		context, keysetRoot, requestID, messageHash, member2,
	)
	if err != nil {
		t.Fatal(err)
	}
	if member2Hash == firstHash {
		t.Fatal("different ShareIDs produced the same compact participation message")
	}

	mutated := member1
	mutated.Participant = common.HexToAddress("0x0000000000000000000000000000000000000033")
	_, mutatedHash, err := RabbitVRFCompactParticipationMessageV1(
		context, keysetRoot, requestID, messageHash, mutated,
	)
	if err != nil {
		t.Fatal(err)
	}
	if mutatedHash == firstHash {
		t.Fatal("different participant produced the same compact participation message")
	}

	if _, _, err := RabbitVRFCompactParticipationMessageV1(
		context, common.Hash{}, requestID, messageHash, member1,
	); !errors.Is(err, ErrInvalidRabbitVRFCompactParticipationV1) {
		t.Fatalf("zero keyset error=%v", err)
	}
	if _, _, err := RabbitVRFCompactParticipationMessageV1(
		context, keysetRoot, common.Hash{}, messageHash, member1,
	); !errors.Is(err, ErrInvalidRabbitVRFCompactParticipationV1) {
		t.Fatalf("zero request error=%v", err)
	}
	if _, _, err := RabbitVRFCompactParticipationMessageV1(
		context, keysetRoot, requestID, common.Hash{}, member1,
	); !errors.Is(err, ErrInvalidRabbitVRFCompactParticipationV1) {
		t.Fatalf("zero message hash error=%v", err)
	}
	badMember := member1
	badMember.ShareID = 0
	if _, _, err := RabbitVRFCompactParticipationMessageV1(
		context, keysetRoot, requestID, messageHash, badMember,
	); !errors.Is(err, ErrInvalidRabbitVRFCompactParticipationV1) {
		t.Fatalf("zero ShareID error=%v", err)
	}
}

func TestRabbitVRFCompactParticipationBitmapV1(t *testing.T) {
	context, err := NewRabbitVRFDKGSessionContextV1(
		big.NewInt(9280),
		11,
		crypto.Keccak256Hash([]byte("compact-participation-bitmap-committee")),
		4,
	)
	if err != nil {
		t.Fatal(err)
	}
	if context.Threshold != 3 {
		t.Fatalf("threshold=%d want=3", context.Threshold)
	}

	first, err := RabbitVRFCompactParticipationBitmapV1(context, []uint64{3, 1, 2})
	if err != nil {
		t.Fatal(err)
	}
	second, err := RabbitVRFCompactParticipationBitmapV1(context, []uint64{1, 2, 3})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("bitmap depends on input order")
	}
	if len(first) != 1 || first[0] != 0x07 {
		t.Fatalf("bitmap=%x want=07", first)
	}

	shareIDs, err := RabbitVRFCompactParticipationShareIDsV1(context, first)
	if err != nil {
		t.Fatal(err)
	}
	if len(shareIDs) != 3 || shareIDs[0] != 1 || shareIDs[1] != 2 || shareIDs[2] != 3 {
		t.Fatalf("shareIDs=%v want=[1 2 3]", shareIDs)
	}

	cases := map[string][]uint64{
		"insufficient": {1, 2},
		"duplicate":    {1, 1, 2},
		"zero":         {0, 1, 2},
		"out_of_range": {1, 2, 5},
	}
	for name, shareIDs := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := RabbitVRFCompactParticipationBitmapV1(context, shareIDs); !errors.Is(err, ErrInvalidRabbitVRFCompactParticipationV1) {
				t.Fatalf("error=%v", err)
			}
		})
	}

	if _, err := RabbitVRFCompactParticipationShareIDsV1(context, []byte{0x0f}); !errors.Is(err, ErrInvalidRabbitVRFCompactParticipationV1) {
		t.Fatalf("too many bits error=%v", err)
	}
	if _, err := RabbitVRFCompactParticipationShareIDsV1(context, []byte{0x87}); !errors.Is(err, ErrInvalidRabbitVRFCompactParticipationV1) {
		t.Fatalf("out-of-range bit error=%v", err)
	}
}

func TestRabbitVRFThresholdPartialMessageIDBindsCompactParticipationV1(t *testing.T) {
	packet := RabbitVRFThresholdPartialV1{
		SessionID:   crypto.Keccak256Hash([]byte("compact-message-id-session")),
		KeysetRoot:  crypto.Keccak256Hash([]byte("compact-message-id-keyset")),
		RequestID:   crypto.Keccak256Hash([]byte("compact-message-id-request")),
		MessageHash: crypto.Keccak256Hash([]byte("compact-message-id-message")),
		ShareID:     1,
	}
	packet.Signature[0] = 0x80
	packet.CompactParticipationSignature[0] = 0x81

	first, err := RabbitVRFThresholdPartialMessageIDV1(packet)
	if err != nil {
		t.Fatal(err)
	}

	changedCompact := packet
	changedCompact.CompactParticipationSignature[1] = 0x42
	second, err := RabbitVRFThresholdPartialMessageIDV1(changedCompact)
	if err != nil {
		t.Fatal(err)
	}
	if second == first {
		t.Fatal("compact BLS participation signature is not bound by threshold partial message ID")
	}

	changedTransport := packet
	changedTransport.ParticipationSignature[0] ^= 0x01
	third, err := RabbitVRFThresholdPartialMessageIDV1(changedTransport)
	if err != nil {
		t.Fatal(err)
	}
	if third != first {
		t.Fatal("outer transport participation signature created a circular threshold partial message ID")
	}
}
