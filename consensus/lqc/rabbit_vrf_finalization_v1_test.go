package lqc

import (
	"errors"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	rabbitvrf "github.com/ethereum/go-ethereum/crypto/rabbitvrf"
)

func TestRabbitVRFFinalizationV1CanonicalAndProofHash(t *testing.T) {
	a := RabbitVRFFinalizationV1{
		Version:                         RabbitVRFFinalizationVersionV1,
		RequestID:                       common.HexToHash("0x01"),
		KeysetRoot:                      common.HexToHash("0x1001"),
		Epoch:                           11,
		Round:                           2,
		Randomness:                      common.HexToHash("0x11"),
		Signature:                       rabbitvrf.Signature{1},
		ProofHash:                       common.HexToHash("0x21"),
		ParticipationBitmap:             [RabbitVRFCompactParticipationBitmapBytesV1]byte{1},
		ParticipationAggregateSignature: rabbitvrf.Signature{2},
	}
	b := RabbitVRFFinalizationV1{
		Version:                         RabbitVRFFinalizationVersionV1,
		RequestID:                       common.HexToHash("0x02"),
		KeysetRoot:                      common.HexToHash("0x1001"),
		Epoch:                           11,
		Round:                           3,
		Randomness:                      common.HexToHash("0x12"),
		Signature:                       rabbitvrf.Signature{1},
		ProofHash:                       common.HexToHash("0x22"),
		ParticipationBitmap:             [RabbitVRFCompactParticipationBitmapBytesV1]byte{1},
		ParticipationAggregateSignature: rabbitvrf.Signature{2},
	}

	canonical, err := CanonicalRabbitVRFFinalizationsV1([]RabbitVRFFinalizationV1{b, a})
	if err != nil {
		t.Fatal(err)
	}
	if len(canonical) != 2 || canonical[0] != a || canonical[1] != b {
		t.Fatal("rabbit vrf finalizations were not canonicalized by request ID")
	}
	if err := ValidateCanonicalRabbitVRFFinalizationsV1([]RabbitVRFFinalizationV1{b, a}); !errors.Is(err, ErrNonCanonicalRabbitVRFFinalizationsV1) {
		t.Fatalf("non-canonical order error=%v", err)
	}
	if _, err := CanonicalRabbitVRFFinalizationsV1([]RabbitVRFFinalizationV1{a, a}); !errors.Is(err, ErrDuplicateRabbitVRFFinalizationV1) {
		t.Fatalf("duplicate error=%v", err)
	}

	invalid := a
	invalid.Randomness = common.Hash{}
	if err := invalid.Validate(); !errors.Is(err, ErrInvalidRabbitVRFFinalizationV1) {
		t.Fatalf("invalid finalization error=%v", err)
	}

	signature := []byte{1, 2, 3, 4}
	first, err := RabbitVRFFinalizationProofHashV1(a.RequestID, a.Epoch, a.Round, signature)
	if err != nil {
		t.Fatal(err)
	}
	second, err := RabbitVRFFinalizationProofHashV1(a.RequestID, a.Epoch, a.Round, signature)
	if err != nil {
		t.Fatal(err)
	}
	if first == (common.Hash{}) || first != second {
		t.Fatal("rabbit vrf finalization proof hash is not deterministic")
	}
}
