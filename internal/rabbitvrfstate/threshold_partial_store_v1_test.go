package rabbitvrfstate

import (
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/lqc"
	rabbitvrf "github.com/ethereum/go-ethereum/crypto/rabbitvrf"
)

func TestThresholdPartialStoreV1RestartDuplicateConflict(t *testing.T) {
	context, err := lqc.NewRabbitVRFDKGSessionContextV1(
		big.NewInt(9280),
		11,
		common.HexToHash("0x1234"),
		3,
	)
	if err != nil {
		t.Fatal(err)
	}

	keysetRoot := common.HexToHash("0xaaaa")
	requestID := common.HexToHash("0xbbbb")

	partial := rabbitvrf.PartialSignature{ShareID: 1}
	partial.Signature[0] = 7

	packet, err := lqc.NewRabbitVRFThresholdPartialV1(
		context,
		keysetRoot,
		requestID,
		partial,
	)
	if err != nil {
		t.Fatal(err)
	}
	packet.ParticipationSignature[0] = 9

	dir := t.TempDir()

	store, err := NewThresholdPartialStoreV1(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Store(context, packet); err != nil {
		t.Fatal(err)
	}
	if err := store.Store(context, packet); err != nil {
		t.Fatalf("idempotent duplicate failed: %v", err)
	}

	restarted, err := NewThresholdPartialStoreV1(dir)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := restarted.Load(
		context,
		keysetRoot,
		requestID,
		1,
	)
	if err != nil {
		t.Fatal(err)
	}
	if loaded != packet {
		t.Fatal("threshold partial changed after restart")
	}

	conflicting := packet
	conflicting.Signature[0] ^= 0xff

	if err := restarted.Store(context, conflicting); !errors.Is(err, ErrThresholdPartialStoreConflictV1) {
		t.Fatalf("conflict error=%v want=%v", err, ErrThresholdPartialStoreConflictV1)
	}
}
